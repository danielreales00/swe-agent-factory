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
}

// SessionHandler handles one message under a session's lock. The Router
// guarantees no two handler invocations run concurrently for the same
// session, so the handler is free to mutate Session fields without locking.
type SessionHandler func(ctx context.Context, sess *Session, msg Message)

// Router maps Telegram chats to Sessions and serializes per-chat work.
// Different chats run concurrently; same-chat messages queue.
type Router struct {
	mu       sync.Mutex
	sessions map[int64]*Session
	handler  SessionHandler
}

func NewRouter(handler SessionHandler) *Router {
	return &Router{
		sessions: map[int64]*Session{},
		handler:  handler,
	}
}

// Dispatch is the Telegram Handler entrypoint. Looks up or creates the
// per-chat session, locks it, runs the handler.
func (r *Router) Dispatch(ctx context.Context, msg Message) {
	sess := r.session(msg.ChatID, msg.UserID)
	sess.mu.Lock()
	defer sess.mu.Unlock()
	r.handler(ctx, sess, msg)
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
		ChatID:    chatID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}
	r.sessions[chatID] = s
	return s
}
