// Package agent spawns and drives a `pi --mode rpc` subprocess.
//
// Pi (github.com/earendil-works/pi) is the inner agent loop the factory
// hands work to. This package owns the JSONL protocol over stdin/stdout:
// it writes commands, parses events, manages the extension UI sub-protocol,
// and returns a structured result when the agent emits `agent_end`.
//
// One Pi instance drives one work item end to end. Pi sessions are not
// reused across work items — pin them via SessionDir for audit/replay.
package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Pi configures how the factory invokes `pi --mode rpc` for one work item.
type Pi struct {
	BinPath    string   // pi binary, defaults to "pi" on PATH
	Provider   string   // --provider, e.g. "anthropic"
	Model      string   // --model, e.g. "claude-sonnet-4-5"
	Cwd        string   // working dir for the subprocess (the target repo)
	SessionDir string   // --session-dir; one dir per WorkItem keeps audit trail
	Extensions []string // absolute paths to .ts extension files (--extension repeated)
	Tools      []string // optional allowlist (--tools); empty = pi defaults
	System     string   // optional --append-system-prompt text
	Env        []string // extra env vars (e.g. ANTHROPIC_API_KEY=…)
	Thinking   string   // optional --thinking level: off|minimal|low|medium|high|xhigh
}

// RunInput is one prompt handed to pi. Phase 1 is single-turn; multi-turn
// (steer/follow_up) is intentionally out of scope.
type RunInput struct {
	Prompt string
}

// RunResult is the structured outcome the orchestrator can act on.
type RunResult struct {
	AssistantText string            // text of the final assistant message
	Messages      []json.RawMessage // raw messages array from agent_end
	ToolCalls     int               // count of tool_execution_end events seen
	ToolErrors    int               // tool_execution_end with isError=true
	StopReason    string            // last seen assistant stop reason
	Stderr        string            // tail of pi's stderr (for diagnostics)
}

// Run spawns pi, drives one prompt to agent_end, returns the result.
// Cancelling ctx sends an `abort` command and kills the process.
func (p *Pi) Run(ctx context.Context, in RunInput) (*RunResult, error) {
	bin := p.BinPath
	if bin == "" {
		bin = "pi"
	}

	args := []string{"--mode", "rpc"}
	if p.Provider != "" {
		args = append(args, "--provider", p.Provider)
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.SessionDir != "" {
		args = append(args, "--session-dir", p.SessionDir)
	} else {
		args = append(args, "--no-session")
	}
	for _, ext := range p.Extensions {
		args = append(args, "--extension", ext)
	}
	if len(p.Tools) > 0 {
		args = append(args, "--tools", strings.Join(p.Tools, ","))
	}
	if p.System != "" {
		args = append(args, "--append-system-prompt", p.System)
	}
	if p.Thinking != "" {
		args = append(args, "--thinking", p.Thinking)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = p.Cwd
	if len(p.Env) > 0 {
		cmd.Env = append(cmd.Environ(), p.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start pi: %w", err)
	}

	// Drain stderr concurrently; keep only the tail for diagnostics.
	var stderrBuf strings.Builder
	var stderrMu sync.Mutex
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		const maxBytes = 16 * 1024
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				stderrMu.Lock()
				if stderrBuf.Len()+n > maxBytes {
					// Drop oldest by collapsing to last maxBytes/2.
					cur := stderrBuf.String()
					if len(cur) > maxBytes/2 {
						cur = cur[len(cur)-maxBytes/2:]
					}
					stderrBuf.Reset()
					stderrBuf.WriteString(cur)
				}
				stderrBuf.Write(buf[:n])
				stderrMu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	res := &RunResult{}
	encoder := newJSONLEncoder(stdin)

	// Send the initial prompt. id is informational; we don't correlate beyond
	// noting the matching response.
	if err := encoder.Encode(map[string]any{
		"id":      "wi-prompt",
		"type":    "prompt",
		"message": in.Prompt,
	}); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	// Read events until agent_end, process exit, or ctx cancel.
	reader := bufio.NewReaderSize(stdout, 64*1024)
	loopErr := readEventLoop(ctx, reader, encoder, res)

	// Close stdin to let pi exit cleanly; then wait with a short grace period.
	_ = stdin.Close()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-waitDone
	}
	stderrWG.Wait()

	stderrMu.Lock()
	res.Stderr = strings.TrimSpace(stderrBuf.String())
	stderrMu.Unlock()

	if loopErr != nil && !errors.Is(loopErr, errAgentEnd) {
		return res, loopErr
	}
	return res, nil
}

var errAgentEnd = errors.New("agent_end reached")

func readEventLoop(ctx context.Context, r *bufio.Reader, enc *jsonlEncoder, res *RunResult) error {
	for {
		select {
		case <-ctx.Done():
			_ = enc.Encode(map[string]any{"type": "abort"})
			return ctx.Err()
		default:
		}

		line, err := readJSONLLine(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return fmt.Errorf("read event: %w", err)
		}
		if len(line) == 0 {
			continue
		}

		// Peek the type without unmarshalling the whole payload twice.
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			return fmt.Errorf("decode event head: %w (raw=%q)", err, truncate(line, 200))
		}

		switch head.Type {
		case "response":
			// We could correlate by id; for Phase 1 we just log non-success.
			var resp struct {
				Command string          `json:"command"`
				Success bool            `json:"success"`
				Err     string          `json:"error"`
				Data    json.RawMessage `json:"data"`
			}
			_ = json.Unmarshal(line, &resp)
			if !resp.Success {
				return fmt.Errorf("rpc command %q failed: %s", resp.Command, resp.Err)
			}

		case "tool_execution_end":
			var tee struct {
				IsError bool `json:"isError"`
			}
			_ = json.Unmarshal(line, &tee)
			res.ToolCalls++
			if tee.IsError {
				res.ToolErrors++
			}

		case "message_end":
			var me struct {
				Message struct {
					Role       string `json:"role"`
					StopReason string `json:"stopReason"`
					Content    any    `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal(line, &me); err == nil && me.Message.Role == "assistant" {
				res.StopReason = me.Message.StopReason
				if txt := extractAssistantText(me.Message.Content); txt != "" {
					res.AssistantText = txt
				}
			}

		case "agent_end":
			var ae struct {
				Messages []json.RawMessage `json:"messages"`
			}
			_ = json.Unmarshal(line, &ae)
			res.Messages = ae.Messages
			return errAgentEnd

		case "extension_ui_request":
			if err := handleExtensionUI(line, enc); err != nil {
				return err
			}

		case "extension_error":
			// Surface but don't abort; pi keeps running.
			// Caller can read res.Stderr for the upstream message too.
			// (No-op for now; future: aggregate into res.ExtensionErrors.)
		}
	}
}

// handleExtensionUI replies to dialog requests (select/confirm/input/editor)
// with a conservative default and silently accepts fire-and-forget ones.
// The factory runs unattended, so we never auto-allow risky prompts.
func handleExtensionUI(line []byte, enc *jsonlEncoder) error {
	var req struct {
		ID     string `json:"id"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(line, &req); err != nil {
		return fmt.Errorf("decode extension_ui_request: %w", err)
	}
	switch req.Method {
	case "select", "input", "editor":
		return enc.Encode(map[string]any{
			"type": "extension_ui_response", "id": req.ID, "cancelled": true,
		})
	case "confirm":
		return enc.Encode(map[string]any{
			"type": "extension_ui_response", "id": req.ID, "confirmed": false,
		})
	default:
		// notify, setStatus, setWidget, setTitle, set_editor_text are
		// fire-and-forget. Nothing to do.
		return nil
	}
}

func extractAssistantText(content any) string {
	// Content can be a string OR an array of blocks ({type:"text", text:"..."}, …).
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

// jsonlEncoder writes one JSON value per line, LF-terminated, thread-safely.
type jsonlEncoder struct {
	w  io.Writer
	mu sync.Mutex
}

func newJSONLEncoder(w io.Writer) *jsonlEncoder { return &jsonlEncoder{w: w} }

func (e *jsonlEncoder) Encode(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err = e.w.Write(b)
	return err
}

// readJSONLLine respects the LF-only framing rule from pi's RPC docs:
// split on \n only, strip a trailing \r, never on U+2028/U+2029.
func readJSONLLine(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if len(line) > 0 {
		if line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
	}
	if err != nil && len(line) == 0 {
		return nil, err
	}
	return line, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
