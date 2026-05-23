package bridge

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRouter_SessionReusedForSameChat(t *testing.T) {
	var seen []*Session
	var mu sync.Mutex
	r := NewRouter(func(ctx context.Context, sess *Session, msg Message) {
		mu.Lock()
		seen = append(seen, sess)
		mu.Unlock()
	})

	ctx := context.Background()
	r.Dispatch(ctx, Message{ChatID: 1, UserID: 100, Text: "a"})
	r.Dispatch(ctx, Message{ChatID: 1, UserID: 100, Text: "b"})
	r.Dispatch(ctx, Message{ChatID: 2, UserID: 200, Text: "c"})

	if len(seen) != 3 {
		t.Fatalf("len(seen) = %d, want 3", len(seen))
	}
	if seen[0] != seen[1] {
		t.Errorf("chat 1 got two different sessions")
	}
	if seen[0] == seen[2] {
		t.Errorf("chats 1 and 2 share a session")
	}
	if seen[0].ChatID != 1 || seen[2].ChatID != 2 {
		t.Errorf("session.ChatID mismatch")
	}
}

func TestRouter_HandlerSerializedPerChat(t *testing.T) {
	var concurrent int32
	var maxConcurrent int32
	r := NewRouter(func(ctx context.Context, sess *Session, msg Message) {
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&concurrent, -1)
	})

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Dispatch(context.Background(), Message{ChatID: 1, UserID: 100})
		}()
	}
	wg.Wait()

	if max := atomic.LoadInt32(&maxConcurrent); max != 1 {
		t.Errorf("maxConcurrent for same chat = %d, want 1", max)
	}
}

func TestRouter_DifferentChatsRunConcurrently(t *testing.T) {
	startCh := make(chan struct{})
	releaseCh := make(chan struct{})
	var inHandler sync.WaitGroup
	r := NewRouter(func(ctx context.Context, sess *Session, msg Message) {
		inHandler.Done()
		<-releaseCh
	})

	const n = 5
	inHandler.Add(n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startCh
			r.Dispatch(context.Background(), Message{ChatID: int64(i + 1), UserID: 100})
		}()
	}
	close(startCh)

	// All n handlers should reach their entry point concurrently.
	allEntered := make(chan struct{})
	go func() {
		inHandler.Wait()
		close(allEntered)
	}()
	select {
	case <-allEntered:
	case <-time.After(2 * time.Second):
		t.Fatalf("only some of %d different-chat handlers ran concurrently", n)
	}

	close(releaseCh)
	wg.Wait()
}

func TestRouter_SessionsSnapshot(t *testing.T) {
	r := NewRouter(func(ctx context.Context, sess *Session, msg Message) {})
	r.Dispatch(context.Background(), Message{ChatID: 10, UserID: 1})
	r.Dispatch(context.Background(), Message{ChatID: 20, UserID: 2})

	snap := r.Sessions()
	if len(snap) != 2 {
		t.Errorf("Sessions() len = %d, want 2", len(snap))
	}
}
