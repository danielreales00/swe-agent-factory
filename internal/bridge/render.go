package bridge

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolStartEvent is the subset of a tool_execution_start event the renderer
// cares about.
type ToolStartEvent struct {
	ToolCallID string         `json:"toolCallId"`
	ToolName   string         `json:"toolName"`
	Args       map[string]any `json:"args"`
}

// ToolEndEvent is the subset of a tool_execution_end event the renderer
// cares about.
type ToolEndEvent struct {
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	IsError    bool   `json:"isError"`
	Result     struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
}

// ParseToolStart decodes a tool_execution_start event. Returns false on
// malformed JSON.
func ParseToolStart(raw []byte) (ToolStartEvent, bool) {
	var ev ToolStartEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return ToolStartEvent{}, false
	}
	return ev, true
}

// ParseToolEnd decodes a tool_execution_end event. Returns false on
// malformed JSON.
func ParseToolEnd(raw []byte) (ToolEndEvent, bool) {
	var ev ToolEndEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return ToolEndEvent{}, false
	}
	return ev, true
}

// ToolResultText concatenates the text blocks in a tool result.
func (e ToolEndEvent) ToolResultText() string {
	var b strings.Builder
	for _, c := range e.Result.Content {
		if c.Type == "text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

const (
	bashCmdMaxLen   = 80
	pathMaxLen      = 80
	patternMaxLen   = 80
	errorBodyMaxLen = 2000
)

// RenderToolEnd produces a one-line chat-friendly summary of a completed
// tool call. Combines the args captured from the start event with the
// end event's name + isError flag. Falls back to a generic
// `🔧 <name>` / `❌ <name> failed` for unknown tools.
func RenderToolEnd(name string, args map[string]any, isError bool) string {
	switch name {
	case "read":
		return "📖 read " + pathArg(args, "path")
	case "grep":
		return "🔍 grep " + quote(stringArg(args, "pattern"))
	case "find":
		return "🔎 find " + stringArg(args, "pattern")
	case "ls":
		path := pathArg(args, "path")
		if path == "" {
			path = "."
		}
		return "📂 ls " + path
	case "edit":
		path := pathArg(args, "path")
		edits, _ := args["edits"].([]any)
		if len(edits) > 0 {
			return fmt.Sprintf("✏️ edit %s (%d changes)", path, len(edits))
		}
		return "✏️ edit " + path
	case "write":
		path := pathArg(args, "path")
		content, _ := args["content"].(string)
		lines := strings.Count(content, "\n") + 1
		if content == "" {
			lines = 0
		}
		return fmt.Sprintf("📝 write %s (+%d lines)", path, lines)
	case "bash":
		cmd := truncate(stringArg(args, "command"), bashCmdMaxLen)
		if isError {
			return "❌ bash failed: " + cmd
		}
		return "⚙️ bash: " + cmd
	case "run_quality":
		if isError {
			return "❌ run_quality failed"
		}
		return "✅ run_quality passed"
	case "propose_plan":
		summary := stringArg(args, "summary")
		firstLine, _, _ := strings.Cut(summary, "\n")
		questions, _ := args["questions"].([]any)
		return fmt.Sprintf("📋 plan proposed: %s (%d questions)",
			truncate(firstLine, 80), len(questions))
	case "request_ship":
		return "🚢 request_ship → " + stringArg(args, "branch")
	default:
		if isError {
			return fmt.Sprintf("❌ %s failed", name)
		}
		return "🔧 " + name
	}
}

// FormatErrorBody clamps a tool result body to a reasonable Telegram-friendly
// length and prefixes it. Returns "" if the body is empty/whitespace-only.
func FormatErrorBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if len(body) > errorBodyMaxLen {
		body = body[:errorBodyMaxLen] + "\n...(truncated)"
	}
	return "↳ " + body
}

func stringArg(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}

func pathArg(args map[string]any, key string) string {
	return truncate(stringArg(args, key), pathMaxLen)
}

func quote(s string) string {
	return "\"" + s + "\""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
