// Tests for the agent package use a fake pi binary implemented in this same
// test binary via the well-known TestMain dispatch trick: when FAKE_PI is
// set in the environment, the binary runs as fake pi instead of running the
// test suite. Each test sets a behavior via FAKE_PI=<name>; the fake reads
// JSONL commands from stdin and emits scripted events.

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv("FAKE_PI"); mode != "" {
		os.Exit(fakePiMain(mode))
	}
	os.Exit(m.Run())
}

// ---------------------------------------------------------------------------
// Fake pi
// ---------------------------------------------------------------------------

func fakePiMain(mode string) int {
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	emit := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, _ = out.Write(b)
		_, _ = out.WriteString("\n")
		return out.Flush()
	}

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	promptIdx := 0
	for sc.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			_ = emit(map[string]any{"type": "response", "success": false, "error": "bad json"})
			continue
		}
		msgType, _ := msg["type"].(string)
		msgID, _ := msg["id"].(string)

		if msgType == "abort" {
			return 2
		}

		// Acknowledge non-abort commands so the parent can see protocol works.
		if msgID != "" && msgType != "extension_ui_response" {
			_ = emit(map[string]any{
				"type":    "response",
				"id":      msgID,
				"command": msgType,
				"success": true,
			})
		}

		if msgType != "prompt" && msgType != "extension_ui_response" {
			continue
		}

		switch mode {
		case "echo":
			fakeEchoTurn(emit, msg, promptIdx)
			promptIdx++

		case "tool_call":
			if msgType == "prompt" {
				_ = emit(map[string]any{"type": "tool_execution_end", "isError": false})
				fakeEchoTurn(emit, msg, promptIdx)
				promptIdx++
			}

		case "confirm":
			if msgType == "prompt" {
				_ = emit(map[string]any{
					"type":   "extension_ui_request",
					"id":     "ui-1",
					"method": "confirm",
					"prompt": "approve?",
				})
				// Stay in loop; wait for extension_ui_response.
			} else { // extension_ui_response
				confirmed, _ := msg["confirmed"].(bool)
				_ = emit(map[string]any{
					"type": "message_end",
					"message": map[string]any{
						"role":       "assistant",
						"stopReason": "end_turn",
						"content":    fmt.Sprintf("confirmed=%v", confirmed),
					},
				})
				_ = emit(map[string]any{"type": "agent_end", "messages": []any{}})
			}

		case "input":
			if msgType == "prompt" {
				_ = emit(map[string]any{
					"type":   "extension_ui_request",
					"id":     "ui-input-1",
					"method": "input",
					"prompt": "name?",
				})
			} else {
				value, _ := msg["value"].(string)
				_ = emit(map[string]any{
					"type": "message_end",
					"message": map[string]any{
						"role":       "assistant",
						"stopReason": "end_turn",
						"content":    "got=" + value,
					},
				})
				_ = emit(map[string]any{"type": "agent_end", "messages": []any{}})
			}

		case "eof_after_prompt":
			// Exit without emitting agent_end → simulates pi crash.
			return 0

		case "bad_response":
			_ = emit(map[string]any{
				"type":    "response",
				"id":      "x",
				"command": "prompt",
				"success": false,
				"error":   "synthetic failure",
			})
			return 0

		case "ui_async":
			// Emit a ui_request but don't reply with anything when the user
			// responds — used to verify async RespondUI flow.
			if msgType == "prompt" {
				_ = emit(map[string]any{
					"type":   "extension_ui_request",
					"id":     "ui-async-1",
					"method": "confirm",
					"prompt": "wait?",
				})
			} else {
				confirmed, _ := msg["confirmed"].(bool)
				_ = emit(map[string]any{
					"type": "message_end",
					"message": map[string]any{
						"role":       "assistant",
						"stopReason": "end_turn",
						"content":    fmt.Sprintf("async-confirmed=%v", confirmed),
					},
				})
				_ = emit(map[string]any{"type": "agent_end", "messages": []any{}})
			}

		default:
			fmt.Fprintf(os.Stderr, "fakepi: unknown mode %q\n", mode)
			return 3
		}
	}
	return 0
}

func fakeEchoTurn(emit func(any) error, msg map[string]any, idx int) {
	text, _ := msg["message"].(string)
	_ = emit(map[string]any{
		"type": "message_end",
		"message": map[string]any{
			"role":       "assistant",
			"stopReason": "end_turn",
			"content":    fmt.Sprintf("echo[%d]: %s", idx, text),
		},
	})
	_ = emit(map[string]any{"type": "agent_end", "messages": []any{}})
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type eventCollector struct {
	mu     sync.Mutex
	events []Event
	signal map[string]chan struct{}
}

func newCollector() *eventCollector {
	return &eventCollector{signal: map[string]chan struct{}{}}
}

func (c *eventCollector) OnEvent(e Event) {
	c.mu.Lock()
	c.events = append(c.events, e)
	if ch, ok := c.signal[e.Type]; ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	c.mu.Unlock()
}

func (c *eventCollector) waitFor(t *testing.T, eventType string, timeout time.Duration) {
	t.Helper()
	c.mu.Lock()
	for _, e := range c.events {
		if e.Type == eventType {
			c.mu.Unlock()
			return
		}
	}
	ch := make(chan struct{}, 1)
	c.signal[eventType] = ch
	c.mu.Unlock()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for event %q", eventType)
	}
}

func (c *eventCollector) types() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.events))
	for i, e := range c.events {
		out[i] = e.Type
	}
	return out
}

func fakeCfg(t *testing.T, mode string) Pi {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	return Pi{
		BinPath: self,
		Env:     []string{"FAKE_PI=" + mode},
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSession_EchoSinglePrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "echo")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	c := newCollector()
	sess.OnEvent = c.OnEvent

	if err := sess.SendPrompt("hello"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	c.waitFor(t, "agent_end", 2*time.Second)

	res, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}

	if !strings.Contains(res.AssistantText, "echo[0]: hello") {
		t.Errorf("AssistantText = %q, want substring %q", res.AssistantText, "echo[0]: hello")
	}
	if res.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", res.StopReason, "end_turn")
	}
}

func TestSession_MultiTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "echo")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	c := newCollector()
	sess.OnEvent = c.OnEvent

	if err := sess.SendPrompt("first"); err != nil {
		t.Fatalf("SendPrompt 1: %v", err)
	}
	c.waitFor(t, "agent_end", 2*time.Second)

	if err := sess.SendPrompt("second"); err != nil {
		t.Fatalf("SendPrompt 2: %v", err)
	}
	// Wait for a second agent_end.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		count := 0
		for _, et := range c.types() {
			if et == "agent_end" {
				count++
			}
		}
		if count >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	res, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !strings.Contains(res.AssistantText, "echo[1]: second") {
		t.Errorf("AssistantText = %q, want last turn echoed", res.AssistantText)
	}

	endCount := 0
	for _, et := range c.types() {
		if et == "agent_end" {
			endCount++
		}
	}
	if endCount != 2 {
		t.Errorf("agent_end count = %d, want 2", endCount)
	}
}

func TestSession_ToolCallAccounting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "tool_call")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	c := newCollector()
	sess.OnEvent = c.OnEvent

	if err := sess.SendPrompt("do a thing"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	c.waitFor(t, "agent_end", 2*time.Second)

	res, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", res.ToolCalls)
	}
	if res.ToolErrors != 0 {
		t.Errorf("ToolErrors = %d, want 0", res.ToolErrors)
	}
}

func TestSession_OnUIConfirm(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "confirm")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	c := newCollector()
	sess.OnEvent = c.OnEvent
	uiCalls := 0
	sess.OnUI = func(req UIRequest) UIResponse {
		uiCalls++
		if req.Method != "confirm" {
			t.Errorf("UIRequest.Method = %q, want confirm", req.Method)
		}
		return UIResponse{Confirmed: true}
	}

	if err := sess.SendPrompt("go"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	c.waitFor(t, "agent_end", 2*time.Second)

	res, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if uiCalls != 1 {
		t.Errorf("OnUI calls = %d, want 1", uiCalls)
	}
	if !strings.Contains(res.AssistantText, "confirmed=true") {
		t.Errorf("AssistantText = %q, want confirmed=true", res.AssistantText)
	}
}

func TestSession_OnUIInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "input")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	c := newCollector()
	sess.OnEvent = c.OnEvent
	sess.OnUI = func(req UIRequest) UIResponse {
		return UIResponse{Value: "Daniel"}
	}

	if err := sess.SendPrompt("ask me"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	c.waitFor(t, "agent_end", 2*time.Second)

	res, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !strings.Contains(res.AssistantText, "got=Daniel") {
		t.Errorf("AssistantText = %q, want got=Daniel", res.AssistantText)
	}
}

func TestSession_RespondUIAsync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "ui_async")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	c := newCollector()
	sess.OnEvent = c.OnEvent
	pendingReq := make(chan UIRequest, 1)
	sess.OnUI = func(req UIRequest) UIResponse {
		pendingReq <- req
		return UIResponse{} // deferred — will reply via RespondUI
	}

	if err := sess.SendPrompt("waitme"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}

	select {
	case req := <-pendingReq:
		if err := sess.RespondUI(req, UIResponse{Confirmed: true}); err != nil {
			t.Fatalf("RespondUI: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("never received UI request")
	}

	c.waitFor(t, "agent_end", 2*time.Second)
	res, err := sess.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !strings.Contains(res.AssistantText, "async-confirmed=true") {
		t.Errorf("AssistantText = %q, want async-confirmed=true", res.AssistantText)
	}
}

func TestSession_Cancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "ui_async") // will block waiting for a UI response
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	sess.OnUI = func(req UIRequest) UIResponse {
		return UIResponse{} // never reply
	}

	if err := sess.SendPrompt("hang"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}
	// Give pi a moment to emit the ui_request.
	time.Sleep(100 * time.Millisecond)

	sess.Cancel()

	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not fire after Cancel")
	}

	// SendPrompt after Cancel must error.
	if err := sess.SendPrompt("late"); !errors.Is(err, errSessionClosed) {
		t.Errorf("SendPrompt after Cancel = %v, want errSessionClosed", err)
	}
}

func TestSession_UnexpectedEOF(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "eof_after_prompt")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := sess.SendPrompt("die"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}

	_, err = sess.Wait()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Wait err = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestSession_BadResponseTerminates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "bad_response")
	sess, err := cfg.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := sess.SendPrompt("trigger"); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}

	_, err = sess.Wait()
	if err == nil || !strings.Contains(err.Error(), "synthetic failure") {
		t.Errorf("Wait err = %v, want failure containing 'synthetic failure'", err)
	}
}

// ---------------------------------------------------------------------------
// Pi.Run backward-compat
// ---------------------------------------------------------------------------

func TestPiRun_LegacyOneShot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "echo")
	res, err := cfg.Run(ctx, RunInput{Prompt: "ping"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.AssistantText, "echo[0]: ping") {
		t.Errorf("AssistantText = %q, want echo[0]: ping", res.AssistantText)
	}
	if res.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", res.StopReason)
	}
}

func TestPiRun_LegacyToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := fakeCfg(t, "tool_call")
	res, err := cfg.Run(ctx, RunInput{Prompt: "do"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", res.ToolCalls)
	}
}

// ---------------------------------------------------------------------------
// Helper sanity tests (catch regressions in framing helpers)
// ---------------------------------------------------------------------------

func TestReadJSONLLine_CRLF(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("{\"a\":1}\r\n{\"b\":2}\n"))
	for i, want := range []string{`{"a":1}`, `{"b":2}`} {
		got, err := readJSONLLine(r)
		if err != nil {
			t.Fatalf("line %d: %v", i, err)
		}
		if string(got) != want {
			t.Errorf("line %d = %q, want %q", i, got, want)
		}
	}
}

func TestBuildUIReply(t *testing.T) {
	cases := []struct {
		method  string
		resp    UIResponse
		wantKey string
		wantVal any
	}{
		{"confirm", UIResponse{Confirmed: true}, "confirmed", true},
		{"select", UIResponse{SelectedID: "x"}, "selectedId", "x"},
		{"select", UIResponse{Cancelled: true}, "cancelled", true},
		{"input", UIResponse{Value: "hi"}, "value", "hi"},
		{"input", UIResponse{Cancelled: true}, "cancelled", true},
		{"editor", UIResponse{Value: "body"}, "value", "body"},
	}
	for _, tc := range cases {
		got := buildUIReply(tc.method, "id", tc.resp)
		if got[tc.wantKey] != tc.wantVal {
			t.Errorf("buildUIReply(%q, %+v)[%q] = %v, want %v",
				tc.method, tc.resp, tc.wantKey, got[tc.wantKey], tc.wantVal)
		}
	}
}
