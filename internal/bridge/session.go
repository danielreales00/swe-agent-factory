package bridge

import (
	"context"
	"sync"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/agent"
)

// Session holds per-chat state. The mutex serializes Router.Dispatch calls
// for the same chat; different chats run concurrently.
//
// Pi/Worktree/Branch are nil/empty between tasks. They are populated when
// the handler starts a new pi subprocess and cleared by the watcher
// goroutine after pi exits.
type Session struct {
	ChatID    int64
	UserID    int64
	CreatedAt time.Time

	Pi       *agent.Session // active pi subprocess; nil when idle
	Worktree string         // path to the active worktree; "" when idle
	Branch   string         // placeholder branch for the active worktree

	mu sync.Mutex

	// inFlight tracks tool_execution_start events keyed by toolCallId so the
	// matching tool_execution_end can be rendered with the original args.
	// Guarded by toolMu; mutated from pi's reader goroutine via OnEvent.
	toolMu   sync.Mutex
	inFlight map[string]ToolStart

	// pendingUIs tracks extension_ui_request{confirm} events awaiting a
	// user button tap. Keyed by req.ID. Guarded by uiMu.
	uiMu       sync.Mutex
	pendingUIs map[string]*PendingUI
}

// PendingUI is one extension_ui_request awaiting a user button tap.
type PendingUI struct {
	Request       UIRequestSnapshot
	TelegramMsgID int         // bot message hosting the buttons
	Timer         *time.Timer // auto-deny timer; Stop() on resolve
}

// UIRequestSnapshot is the subset of agent.UIRequest the bridge needs to
// reply later via Pi.RespondUI. Kept separate from agent.UIRequest so the
// session.go doesn't have to grow the import set at every field touch.
type UIRequestSnapshot struct {
	ID     string
	Method string
}

// ToolStart is the subset of a tool_execution_start event needed to render
// the matching tool_execution_end.
type ToolStart struct {
	Name string
	Args map[string]any
}

// SessionHandler handles one message under a session's lock. The Router
// guarantees no two handler invocations run concurrently for the same
// session, so the handler is free to mutate Session fields without locking.
type SessionHandler func(ctx context.Context, sess *Session, msg Message)

// CallbackSessionHandler handles one Telegram callback (button tap) under
// the session's lock. Set on Router after construction (optional).
type CallbackSessionHandler func(ctx context.Context, sess *Session, cb Callback)

// Router maps Telegram chats to Sessions and serializes per-chat work.
// Different chats run concurrently; same-chat messages queue.
type Router struct {
	mu       sync.Mutex
	sessions map[int64]*Session
	handler  SessionHandler

	// OnCallback handles Telegram callback queries (button taps). Optional;
	// nil means callbacks are dropped.
	OnCallback CallbackSessionHandler
}

func NewRouter(handler SessionHandler) *Router {
	return &Router{
		sessions: map[int64]*Session{},
		handler:  handler,
	}
}

// Dispatch is the Telegram MessageHandler entrypoint. Looks up or creates
// the per-chat session, locks it, runs the handler.
func (r *Router) Dispatch(ctx context.Context, msg Message) {
	sess := r.session(msg.ChatID, msg.UserID)
	sess.mu.Lock()
	defer sess.mu.Unlock()
	r.handler(ctx, sess, msg)
}

// DispatchCallback is the Telegram CallbackHandler entrypoint. Looks up
// or creates the per-chat session, locks it, runs OnCallback.
func (r *Router) DispatchCallback(ctx context.Context, cb Callback) {
	if r.OnCallback == nil {
		return
	}
	sess := r.session(cb.ChatID, cb.UserID)
	sess.mu.Lock()
	defer sess.mu.Unlock()
	r.OnCallback(ctx, sess, cb)
}

// Sessions returns a snapshot of all live sessions, for diagnostics.
func (r *Router) Sessions() []*Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}

func (r *Router) session(chatID, userID int64) *Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[chatID]; ok {
		return s
	}
	s := &Session{
		ChatID:     chatID,
		UserID:     userID,
		CreatedAt:  time.Now(),
		inFlight:   map[string]ToolStart{},
		pendingUIs: map[string]*PendingUI{},
	}
	r.sessions[chatID] = s
	return s
}
