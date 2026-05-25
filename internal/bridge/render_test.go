package bridge

import (
	"strings"
	"testing"
)

func TestRenderToolEnd_KnownTools(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]any
		isError bool
		want    string
	}{
		{
			name: "read",
			args: map[string]any{"path": "src/finance/x.clj"},
			want: "📖 read src/finance/x.clj",
		},
		{
			name: "grep",
			args: map[string]any{"pattern": "format-money"},
			want: "🔍 grep \"format-money\"",
		},
		{
			name: "ls",
			args: map[string]any{"path": "src/finance/"},
			want: "📂 ls src/finance/",
		},
		{
			name: "ls",
			args: map[string]any{},
			want: "📂 ls .",
		},
		{
			name: "edit",
			args: map[string]any{
				"path":  "src/finance/x.clj",
				"edits": []any{1, 2, 3},
			},
			want: "✏️ edit src/finance/x.clj (3 changes)",
		},
		{
			name: "edit",
			args: map[string]any{"path": "src/x.clj"},
			want: "✏️ edit src/x.clj",
		},
		{
			name: "write",
			args: map[string]any{
				"path":    "src/finance/x.clj",
				"content": "line1\nline2\nline3",
			},
			want: "📝 write src/finance/x.clj (+3 lines)",
		},
		{
			name: "write",
			args: map[string]any{"path": "src/finance/x.clj", "content": ""},
			want: "📝 write src/finance/x.clj (+0 lines)",
		},
		{
			name: "bash",
			args: map[string]any{"command": "git log --oneline -20"},
			want: "⚙️ bash: git log --oneline -20",
		},
		{
			name:    "bash",
			args:    map[string]any{"command": "rm -rf x"},
			isError: true,
			want:    "❌ bash failed: rm -rf x",
		},
		{
			name: "run_quality",
			args: map[string]any{},
			want: "✅ run_quality passed",
		},
		{
			name:    "run_quality",
			args:    map[string]any{},
			isError: true,
			want:    "❌ run_quality failed",
		},
		{
			name: "propose_plan",
			args: map[string]any{
				"summary":   "Refactor format-money to handle negatives\nMore context here.",
				"questions": []any{"q1", "q2"},
			},
			want: "📋 plan proposed: Refactor format-money to handle negatives (2 questions)",
		},
		{
			name: "request_ship",
			args: map[string]any{"branch": "swe-agent/format-money-fix"},
			want: "🚢 request_ship → swe-agent/format-money-fix",
		},
		{
			name: "unknown_tool",
			args: map[string]any{},
			want: "🔧 unknown_tool",
		},
		{
			name:    "unknown_tool",
			args:    map[string]any{},
			isError: true,
			want:    "❌ unknown_tool failed",
		},
	}
	for _, tc := range cases {
		got := RenderToolEnd(tc.name, tc.args, tc.isError)
		if got != tc.want {
			t.Errorf("RenderToolEnd(%q, %v, isError=%v) = %q, want %q",
				tc.name, tc.args, tc.isError, got, tc.want)
		}
	}
}

func TestRenderToolEnd_TruncatesLongPaths(t *testing.T) {
	longPath := strings.Repeat("a/", 60) + "file.clj"
	got := RenderToolEnd("read", map[string]any{"path": longPath}, false)
	if !strings.HasPrefix(got, "📖 read ") {
		t.Errorf("got = %q, want prefix", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("got = %q, want trailing ellipsis (truncation)", got)
	}
}

func TestRenderToolEnd_TruncatesLongBashCommand(t *testing.T) {
	longCmd := strings.Repeat("x", 200)
	got := RenderToolEnd("bash", map[string]any{"command": longCmd}, false)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("got = %q, want trailing ellipsis", got)
	}
}

func TestRenderToolEnd_NilArgsFallsBack(t *testing.T) {
	got := RenderToolEnd("read", nil, false)
	if got != "📖 read " {
		t.Errorf("got = %q, want %q (nil args means empty path)", got, "📖 read ")
	}
}

func TestParseToolStart(t *testing.T) {
	raw := []byte(`{
		"type": "tool_execution_start",
		"toolCallId": "call_xyz",
		"toolName": "bash",
		"args": {"command": "ls"}
	}`)
	ev, ok := ParseToolStart(raw)
	if !ok {
		t.Fatal("ParseToolStart returned false")
	}
	if ev.ToolCallID != "call_xyz" || ev.ToolName != "bash" {
		t.Errorf("ev = %+v", ev)
	}
	if cmd, _ := ev.Args["command"].(string); cmd != "ls" {
		t.Errorf("args[command] = %q, want ls", cmd)
	}
}

func TestParseToolEnd_Success(t *testing.T) {
	raw := []byte(`{
		"type": "tool_execution_end",
		"toolCallId": "call_xyz",
		"toolName": "bash",
		"result": {"content": [{"type": "text", "text": "total 48\nfoo"}]},
		"isError": false
	}`)
	ev, ok := ParseToolEnd(raw)
	if !ok {
		t.Fatal("ParseToolEnd returned false")
	}
	if ev.IsError {
		t.Error("IsError = true, want false")
	}
	if ev.ToolResultText() != "total 48\nfoo" {
		t.Errorf("ToolResultText = %q", ev.ToolResultText())
	}
}

func TestParseToolEnd_Error(t *testing.T) {
	raw := []byte(`{
		"type": "tool_execution_end",
		"toolCallId": "x",
		"toolName": "bash",
		"result": {"content": [{"type": "text", "text": "permission denied"}]},
		"isError": true
	}`)
	ev, ok := ParseToolEnd(raw)
	if !ok {
		t.Fatal("ParseToolEnd returned false")
	}
	if !ev.IsError {
		t.Error("IsError = false, want true")
	}
	if ev.ToolResultText() != "permission denied" {
		t.Errorf("ToolResultText = %q", ev.ToolResultText())
	}
}

func TestParseToolEnd_Malformed(t *testing.T) {
	_, ok := ParseToolEnd([]byte("{not json"))
	if ok {
		t.Error("ParseToolEnd returned true for malformed JSON")
	}
}

func TestFormatErrorBody(t *testing.T) {
	if got := FormatErrorBody(""); got != "" {
		t.Errorf("empty → %q, want empty", got)
	}
	if got := FormatErrorBody("   \n  "); got != "" {
		t.Errorf("whitespace-only → %q, want empty", got)
	}
	if got := FormatErrorBody("oops"); got != "↳ oops" {
		t.Errorf("got = %q, want %q", got, "↳ oops")
	}
}

func TestFormatErrorBody_Truncates(t *testing.T) {
	long := strings.Repeat("x", 5000)
	got := FormatErrorBody(long)
	if !strings.HasSuffix(got, "(truncated)") {
		t.Errorf("got suffix = %q, want (truncated)", got[len(got)-12:])
	}
	if len(got) > errorBodyMaxLen+50 {
		t.Errorf("len(got) = %d, want ~%d", len(got), errorBodyMaxLen)
	}
}
