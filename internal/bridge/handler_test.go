package bridge

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"

	"github.com/danielreales00/swe-agent-factory/internal/agent"
)

func TestAssistantTextFromEvent_StringContent(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "message_end",
		"message": {"role": "assistant", "content": "hello world"}
	}`)
	got, ok := assistantTextFromEvent(agent.Event{Type: "message_end", Raw: raw})
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got != "hello world" {
		t.Errorf("got = %q, want %q", got, "hello world")
	}
}

func TestAssistantTextFromEvent_BlockContent(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "message_end",
		"message": {
			"role": "assistant",
			"content": [
				{"type": "text", "text": "part one. "},
				{"type": "tool_use", "id": "x"},
				{"type": "text", "text": "part two."}
			]
		}
	}`)
	got, ok := assistantTextFromEvent(agent.Event{Type: "message_end", Raw: raw})
	if !ok {
		t.Fatal("ok = false, want true")
	}
	want := "part one. part two."
	if got != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

func TestAssistantTextFromEvent_NonAssistantRoleIgnored(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "message_end",
		"message": {"role": "user", "content": "ignore me"}
	}`)
	got, ok := assistantTextFromEvent(agent.Event{Type: "message_end", Raw: raw})
	if ok {
		t.Errorf("ok = true, want false (role=user)")
	}
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

func TestAssistantTextFromEvent_WrongEventTypeIgnored(t *testing.T) {
	raw := json.RawMessage(`{"type": "tool_call"}`)
	_, ok := assistantTextFromEvent(agent.Event{Type: "tool_call", Raw: raw})
	if ok {
		t.Errorf("ok = true for tool_call event, want false")
	}
}

func TestAssistantTextFromEvent_MalformedJSON(t *testing.T) {
	raw := json.RawMessage(`{not json`)
	_, ok := assistantTextFromEvent(agent.Event{Type: "message_end", Raw: raw})
	if ok {
		t.Errorf("ok = true for malformed json, want false")
	}
}

func TestExtractContentText_EmptyArray(t *testing.T) {
	got := extractContentText([]any{})
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

func TestExtractContentText_AllToolBlocks(t *testing.T) {
	content := []any{
		map[string]any{"type": "tool_use", "id": "x"},
		map[string]any{"type": "tool_result", "id": "y"},
	}
	got := extractContentText(content)
	if got != "" {
		t.Errorf("got = %q, want empty (no text blocks)", got)
	}
}

func TestTriggerShip_NoShipperConfigured_LogsAndReturns(t *testing.T) {
	var buf bytes.Buffer
	h := &Handler{Logger: log.New(&buf, "", 0)}
	sess := &Session{Worktree: "/some/path"}
	h.triggerShip(sess, 42, ShipRequest{Branch: "swe-agent/x", Title: "t", Body: "b"})
	if !strings.Contains(buf.String(), "no Shipper configured") {
		t.Errorf("log = %q, want 'no Shipper configured'", buf.String())
	}
}

func TestTriggerShip_NoWorktree_LogsAndReturns(t *testing.T) {
	var buf bytes.Buffer
	h := &Handler{
		Ship:   &DefaultShipper{},
		Logger: log.New(&buf, "", 0),
	}
	sess := &Session{} // Worktree empty
	h.triggerShip(sess, 42, ShipRequest{Branch: "swe-agent/x", Title: "t", Body: "b"})
	if !strings.Contains(buf.String(), "no worktree") {
		t.Errorf("log = %q, want 'no worktree'", buf.String())
	}
}

func TestExtractContentText_UnknownTypeIgnored(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "kept"},
		map[string]any{"type": "future_block", "text": "skipped"},
		map[string]any{"type": "text", "text": " and kept"},
	}
	got := extractContentText(content)
	if got != "kept and kept" {
		t.Errorf("got = %q, want %q", got, "kept and kept")
	}
}
