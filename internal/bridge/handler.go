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
	Registry  *Registry
	Worktrees *WorktreeManager
	Client    *Client
	SpawnPi   PiSpawner
	Ship      Shipper
	Logger    *log.Logger

	// UIPromptTimeout is how long the bridge waits for a button tap before
	// auto-denying an extension_ui_request{confirm}. Zero means no timeout
	// (pi blocks until the user replies). Default 5 minutes.
	UIPromptTimeout time.Duration

	// ShipTimeout caps the git push + gh pr create combo. Default 60s.
	ShipTimeout time.Duration
}

const (
	defaultUIPromptTimeout = 5 * time.Minute
	defaultShipTimeout     = 60 * time.Second
)

// Run is called by the Router under the session's lock. The state machine:
//   - msg is /repo or /cancel → command branch (never spawns pi)
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

	if cmd := ParseCommand(msg.Text); cmd.Kind != CmdNone {
		h.runCommand(sess, msg.ChatID, cmd)
		return
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

// runCommand applies the pure ExecuteCommand result: sends the reply, then
// applies any state effect (cancel pi, switch target). Called under sess.mu
// via Router.Dispatch — safe to read/write sess.Target / sess.Pi.
func (h *Handler) runCommand(sess *Session, chatID int64, cmd Command) {
	reply := ExecuteCommand(cmd, sess, h.Registry)
	if reply.Reply != "" {
		h.notify(chatID, reply.Reply)
	}
	if reply.SwitchTarget != nil {
		sess.Target = *reply.SwitchTarget
	}
	if reply.Cancel && sess.Pi != nil {
		sess.Pi.Cancel()
	}
}

func (h *Handler) startSession(ctx context.Context, sess *Session, chatID int64) error {
	if sess.Target.Name == "" {
		h.notify(chatID, "❌ no target selected. /repo to pick one.")
		return fmt.Errorf("no target on session")
	}
	target := sess.Target
	path, branch, err := h.Worktrees.Create(ctx, target, chatID)
	if err != nil {
		h.notify(chatID, "❌ create worktree: "+err.Error())
		return fmt.Errorf("create worktree: %w", err)
	}

	piSess, err := h.SpawnPi(ctx, path, chatID)
	if err != nil {
		h.notify(chatID, "❌ spawn pi: "+err.Error())
		_ = h.Worktrees.Remove(context.Background(), target, path, branch)
		return fmt.Errorf("spawn pi: %w", err)
	}

	sess.Pi = piSess
	sess.Worktree = path
	sess.Branch = branch

	piSess.OnEvent = func(e agent.Event) {
		h.handlePiEvent(sess, chatID, e)
	}
	piSess.OnUI = func(req agent.UIRequest) agent.UIResponse {
		return h.handleUIRequest(sess, chatID, req)
	}

	go h.watchPiExit(sess, chatID, piSess)

	h.notify(chatID, fmt.Sprintf("🌱 worktree: %s\nbranch: %s\ntarget: %s\npi ready.",
		path, branch, target.Name))
	return nil
}

func (h *Handler) handlePiEvent(sess *Session, chatID int64, e agent.Event) {
	switch e.Type {
	case "message_end":
		text, ok := assistantTextFromEvent(e)
		if !ok || text == "" {
			return
		}
		if _, err := h.Client.Send(chatID, text); err != nil {
			h.Logger.Printf("send assistant text to chat=%d: %v", chatID, err)
		}

	case "tool_execution_start":
		ev, ok := ParseToolStart(e.Raw)
		if !ok || ev.ToolCallID == "" {
			return
		}
		sess.toolMu.Lock()
		sess.inFlight[ev.ToolCallID] = ToolStart{Name: ev.ToolName, Args: ev.Args}
		sess.toolMu.Unlock()

	case "tool_execution_end":
		ev, ok := ParseToolEnd(e.Raw)
		if !ok {
			return
		}
		sess.toolMu.Lock()
		start, hadStart := sess.inFlight[ev.ToolCallID]
		delete(sess.inFlight, ev.ToolCallID)
		sess.toolMu.Unlock()

		name := ev.ToolName
		var args map[string]any
		if hadStart {
			if start.Name != "" {
				name = start.Name
			}
			args = start.Args
		}

		line := RenderToolEnd(name, args, ev.IsError)
		if _, err := h.Client.Send(chatID, line); err != nil {
			h.Logger.Printf("send tool line to chat=%d: %v", chatID, err)
		}
		if ev.IsError {
			if body := FormatErrorBody(ev.ToolResultText()); body != "" {
				if _, err := h.Client.Send(chatID, body); err != nil {
					h.Logger.Printf("send tool error body to chat=%d: %v", chatID, err)
				}
			}
		}

		if name == "request_ship" && !ev.IsError {
			if req, ok := ParseShipRequestArgs(args); ok {
				h.triggerShip(sess, chatID, req)
			}
		}

	case "extension_error":
		var ee struct {
			ExtensionPath string `json:"extensionPath"`
			Event         string `json:"event"`
			Error         string `json:"error"`
		}
		if err := json.Unmarshal(e.Raw, &ee); err != nil {
			return
		}
		h.notify(chatID, fmt.Sprintf("⚠️ extension error in %s (%s): %s",
			filepathBase(ee.ExtensionPath), ee.Event, ee.Error))
	}
}

// filepathBase returns the last path component without importing path/filepath
// just for one helper. Splits on / which is fine for the extension paths we
// see (pi resolves to absolute POSIX-style paths even on Windows).
func filepathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (h *Handler) watchPiExit(sess *Session, chatID int64, piSess *agent.Session) {
	<-piSess.Done()

	sess.mu.Lock()
	defer sess.mu.Unlock()

	// Another spawn (or inline cleanup) already took over — nothing to do.
	if sess.Pi != piSess {
		return
	}

	worktree, branch, target := sess.Worktree, sess.Branch, sess.Target
	sess.Pi = nil
	sess.Worktree = ""
	sess.Branch = ""
	sess.toolMu.Lock()
	clear(sess.inFlight)
	sess.toolMu.Unlock()
	h.cancelAllPendingUIs(sess, chatID, "(pi exited before reply)")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := h.Worktrees.Remove(ctx, target, worktree, branch); err != nil {
		h.Logger.Printf("post-exit worktree cleanup: %v", err)
	}
	h.notify(chatID, "💀 pi exited. Worktree cleaned. Next message starts fresh.")
}

// cleanupPi is the inline counterpart to watchPiExit. Used when Run sees pi
// is already done but the watcher hasn't acquired the lock yet. Called
// under sess.mu.
func (h *Handler) cleanupPi(ctx context.Context, sess *Session) {
	worktree, branch, target := sess.Worktree, sess.Branch, sess.Target
	sess.Pi = nil
	sess.Worktree = ""
	sess.Branch = ""
	sess.toolMu.Lock()
	clear(sess.inFlight)
	sess.toolMu.Unlock()
	h.cancelAllPendingUIs(sess, sess.ChatID, "(pi exited before reply)")
	if worktree != "" {
		if err := h.Worktrees.Remove(ctx, target, worktree, branch); err != nil {
			h.Logger.Printf("inline worktree cleanup: %v", err)
		}
	}
}

// handleUIRequest is wired into pi's OnUI. The bridge supports the
// "confirm" method via inline Telegram buttons; all other methods fall
// through to DenyAllUI for now. Returns a zero UIResponse to defer; the
// real reply ships from HandleCallback or the auto-deny timer.
func (h *Handler) handleUIRequest(sess *Session, chatID int64, req agent.UIRequest) agent.UIResponse {
	if req.Method != "confirm" {
		return agent.DenyAllUI(req)
	}

	confirm, _ := ParseConfirmRequest(req.Raw)
	timeout := h.UIPromptTimeout
	if timeout == 0 {
		timeout = defaultUIPromptTimeout
	}
	prompt := RenderConfirmPrompt(confirm, int(timeout/time.Minute))

	msgID, err := h.Client.SendWithButtons(chatID, prompt, []Button{
		{Label: "✅ approve", Data: EncodeCallbackData("a", req.ID)},
		{Label: "❌ deny", Data: EncodeCallbackData("d", req.ID)},
	})
	if err != nil {
		h.Logger.Printf("send confirm prompt: %v", err)
		// Synchronous deny so pi isn't left hanging.
		return agent.UIResponse{Confirmed: false}
	}

	pending := &PendingUI{
		Request:       UIRequestSnapshot{ID: req.ID, Method: req.Method},
		TelegramMsgID: msgID,
	}
	pending.Timer = time.AfterFunc(timeout, func() {
		h.timeoutPendingUI(sess, chatID, req.ID)
	})

	sess.uiMu.Lock()
	sess.pendingUIs[req.ID] = pending
	sess.uiMu.Unlock()

	return agent.UIResponse{} // deferred — RespondUI later
}

// HandleCallback is wired into the Router as the CallbackSessionHandler.
// Called under sess.mu by Router.DispatchCallback.
func (h *Handler) HandleCallback(ctx context.Context, sess *Session, cb Callback) {
	action, reqID, ok := DecodeCallbackData(cb.Data)
	if !ok {
		_ = h.Client.AnswerCallback(cb.QueryID, "unknown action")
		return
	}

	sess.uiMu.Lock()
	pending, exists := sess.pendingUIs[reqID]
	if exists {
		if pending.Timer != nil {
			pending.Timer.Stop()
		}
		delete(sess.pendingUIs, reqID)
	}
	sess.uiMu.Unlock()

	if !exists {
		_ = h.Client.AnswerCallback(cb.QueryID, "expired")
		_ = h.Client.EditMessage(cb.ChatID, cb.MsgID, "(expired)")
		return
	}

	var resp agent.UIResponse
	var result string
	switch action {
	case "a":
		resp = agent.UIResponse{Confirmed: true}
		result = "✅ approved"
	case "d":
		resp = agent.UIResponse{Confirmed: false}
		result = "❌ denied"
	}

	_ = h.Client.AnswerCallback(cb.QueryID, result)
	if err := h.Client.EditMessage(cb.ChatID, cb.MsgID, result); err != nil {
		h.Logger.Printf("edit confirm message: %v", err)
	}

	if sess.Pi == nil {
		return // pi already gone
	}
	if err := sess.Pi.RespondUI(agent.UIRequest{ID: pending.Request.ID, Method: pending.Request.Method}, resp); err != nil {
		h.Logger.Printf("respond UI to pi: %v", err)
	}
}

// timeoutPendingUI fires when the user doesn't tap within UIPromptTimeout.
// Auto-denies and edits the bot message to mark it timed-out.
func (h *Handler) timeoutPendingUI(sess *Session, chatID int64, reqID string) {
	sess.uiMu.Lock()
	pending, exists := sess.pendingUIs[reqID]
	if exists {
		delete(sess.pendingUIs, reqID)
	}
	sess.uiMu.Unlock()
	if !exists {
		return
	}

	if err := h.Client.EditMessage(chatID, pending.TelegramMsgID, "⏱ timed out (auto-denied)"); err != nil {
		h.Logger.Printf("edit timed-out message: %v", err)
	}
	if sess.Pi != nil {
		_ = sess.Pi.RespondUI(
			agent.UIRequest{ID: pending.Request.ID, Method: pending.Request.Method},
			agent.UIResponse{Confirmed: false},
		)
	}
}

// cancelAllPendingUIs is called under sess.mu when pi has exited. Edits
// each pending bot-message to mark it stale, stops timers, and clears the
// map. Does NOT call RespondUI (pi is gone).
func (h *Handler) cancelAllPendingUIs(sess *Session, chatID int64, reason string) {
	sess.uiMu.Lock()
	defer sess.uiMu.Unlock()
	for id, p := range sess.pendingUIs {
		if p.Timer != nil {
			p.Timer.Stop()
		}
		if err := h.Client.EditMessage(chatID, p.TelegramMsgID, reason); err != nil {
			h.Logger.Printf("cancel pending UI %s: %v", id, err)
		}
	}
	clear(sess.pendingUIs)
}

// triggerShip captures the worktree path off sess and launches the ship
// flow in a goroutine. Called from handlePiEvent (pi reader goroutine).
//
// Safe to read sess.Worktree here: watchPiExit only mutates it after pi
// has fully exited and the reader has closed, which is strictly after the
// reader emits its last tool_execution_end event.
func (h *Handler) triggerShip(sess *Session, chatID int64, req ShipRequest) {
	if h.Ship == nil {
		h.Logger.Printf("ship request received but no Shipper configured (chat=%d branch=%s)",
			chatID, req.Branch)
		return
	}
	worktree := sess.Worktree
	if worktree == "" {
		h.Logger.Printf("ship request with no worktree (chat=%d branch=%s)", chatID, req.Branch)
		return
	}
	go h.runShip(chatID, worktree, req)
}

func (h *Handler) runShip(chatID int64, worktree string, req ShipRequest) {
	timeout := h.ShipTimeout
	if timeout == 0 {
		timeout = defaultShipTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	h.notify(chatID, "📦 pushing "+req.Branch+"…")
	url, err := h.Ship.Ship(ctx, worktree, req)
	if err != nil {
		h.Logger.Printf("ship failed (chat=%d branch=%s): %v", chatID, req.Branch, err)
		h.notify(chatID, "❌ ship failed: "+err.Error())
		return
	}
	h.notify(chatID, "✅ draft PR opened\n"+url)
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
