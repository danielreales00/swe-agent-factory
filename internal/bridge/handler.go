package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/agent"
)

// PiSpawner builds and starts a pi `--mode rpc` subprocess for a worktree.
// The returned Session is "live": OnEvent/OnUI can be set on it, and
// SendPrompt is ready to accept input.
type PiSpawner func(ctx context.Context, worktree string, chatID int64) (*agent.Session, error)

// Handler is the bridge's per-message logic. Holds dependencies shared by
// every chat; Run is wired into the Router as the SessionHandler.
type Handler struct {
	Worktrees *WorktreeManager
	Client    *Client
	SpawnPi   PiSpawner
	Logger    *log.Logger
}

// Run is called by the Router under the session's lock. The state machine:
//   - sess.Pi nil    → create worktree, spawn pi, prompt
//   - sess.Pi alive  → forward as another prompt
//   - sess.Pi dead   → clean up inline, then start fresh
func (h *Handler) Run(ctx context.Context, sess *Session, msg Message) {
	// If pi has exited but the watcher hasn't reset state yet, do it now.
	if sess.Pi != nil {
		select {
		case <-sess.Pi.Done():
			h.cleanupPi(ctx, sess)
		default:
		}
	}

	if sess.Pi == nil {
		if err := h.startSession(ctx, sess, msg.ChatID); err != nil {
			h.Logger.Printf("start session for chat=%d: %v", msg.ChatID, err)
			return
		}
	}

	if err := sess.Pi.SendPrompt(msg.Text); err != nil {
		h.Logger.Printf("send prompt: %v", err)
		h.notify(msg.ChatID, "❌ send to pi: "+err.Error())
	}
}

func (h *Handler) startSession(ctx context.Context, sess *Session, chatID int64) error {
	path, branch, err := h.Worktrees.Create(ctx, chatID)
	if err != nil {
		h.notify(chatID, "❌ create worktree: "+err.Error())
		return fmt.Errorf("create worktree: %w", err)
	}

	piSess, err := h.SpawnPi(ctx, path, chatID)
	if err != nil {
		h.notify(chatID, "❌ spawn pi: "+err.Error())
		_ = h.Worktrees.Remove(context.Background(), path, branch)
		return fmt.Errorf("spawn pi: %w", err)
	}

	sess.Pi = piSess
	sess.Worktree = path
	sess.Branch = branch

	piSess.OnEvent = func(e agent.Event) {
		h.handlePiEvent(chatID, e)
	}
	piSess.OnUI = agent.DenyAllUI

	go h.watchPiExit(sess, chatID, piSess)

	h.notify(chatID, fmt.Sprintf("🌱 worktree: %s\nbranch: %s\npi ready.", path, branch))
	return nil
}

func (h *Handler) handlePiEvent(chatID int64, e agent.Event) {
	// 2.4 surfaces only assistant message_end. Tool-call rendering arrives
	// in 2.5; permission prompts in 2.6.
	text, ok := assistantTextFromEvent(e)
	if !ok || text == "" {
		return
	}
	if _, err := h.Client.Send(chatID, text); err != nil {
		h.Logger.Printf("send assistant text to chat=%d: %v", chatID, err)
	}
}

func (h *Handler) watchPiExit(sess *Session, chatID int64, piSess *agent.Session) {
	<-piSess.Done()

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Another spawn (or inline cleanup) already took over — nothing to do.
	if sess.Pi != piSess {
		return
	}

	worktree, branch := sess.Worktree, sess.Branch
	sess.Pi = nil
	sess.Worktree = ""
	sess.Branch = ""

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := h.Worktrees.Remove(ctx, worktree, branch); err != nil {
		h.Logger.Printf("post-exit worktree cleanup: %v", err)
	}
	h.notify(chatID, "💀 pi exited. Worktree cleaned. Next message starts fresh.")
}

// cleanupPi is the inline counterpart to watchPiExit. Used when Run sees pi
// is already done but the watcher hasn't acquired the lock yet. Called
// under sess.mu.
func (h *Handler) cleanupPi(ctx context.Context, sess *Session) {
	worktree, branch := sess.Worktree, sess.Branch
	sess.Pi = nil
	sess.Worktree = ""
	sess.Branch = ""
	if worktree != "" {
		if err := h.Worktrees.Remove(ctx, worktree, branch); err != nil {
			h.Logger.Printf("inline worktree cleanup: %v", err)
		}
	}
}

func (h *Handler) notify(chatID int64, text string) {
	if _, err := h.Client.Send(chatID, text); err != nil {
		h.Logger.Printf("notify chat=%d: %v", chatID, err)
	}
}

// assistantTextFromEvent extracts the plaintext assistant body from a
// message_end event. Returns ("", false) for non-assistant or non-text
// events.
func assistantTextFromEvent(e agent.Event) (string, bool) {
	if e.Type != "message_end" {
		return "", false
	}
	var me struct {
		Message struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(e.Raw, &me); err != nil {
		return "", false
	}
	if me.Message.Role != "assistant" {
		return "", false
	}
	return extractContentText(me.Message.Content), true
}

func extractContentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, blk := range v {
			m, ok := blk.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t == "text" {
				if s, ok := m["text"].(string); ok {
					b.WriteString(s)
				}
			}
		}
		return b.String()
	}
	return ""
}
