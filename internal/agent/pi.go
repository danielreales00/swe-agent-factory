// Package agent spawns and drives a `pi --mode rpc` subprocess.
//
// Pi (github.com/earendil-works/pi) is the inner agent loop the factory
// hands work to. This package owns the JSONL protocol over stdin/stdout:
// it writes commands, parses events, manages the extension UI sub-protocol,
// and returns a structured result when the agent emits `agent_end`.
//
// Two APIs live here:
//
//   - Session — long-lived, streaming, multi-turn. Caller wires OnEvent and
//     OnUI, drives the session with SendPrompt, ends it with Close (graceful)
//     or Cancel (forced). This is what the Telegram bridge uses.
//
//   - Pi.Run — one-shot wrapper. Sends one prompt, waits for the first
//     agent_end, closes stdin, returns the accumulated RunResult. Preserved
//     for cmd/spike and any other unattended caller.
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
	"sync/atomic"
	"time"
)

// Pi configures how the factory invokes `pi --mode rpc`.
type Pi struct {
	BinPath    string   // pi binary, defaults to "pi" on PATH
	Provider   string   // --provider, e.g. "anthropic"
	Model      string   // --model, e.g. "claude-sonnet-4-5"
	Cwd        string   // working dir for the subprocess (the target repo)
	SessionDir string   // --session-dir; one dir per session keeps audit trail
	Extensions []string // absolute paths to .ts extension files (--extension repeated)
	Tools      []string // optional allowlist (--tools); empty = pi defaults
	System     string   // optional --append-system-prompt text
	Env        []string // extra env vars (e.g. ANTHROPIC_API_KEY=…)
	Thinking   string   // optional --thinking level: off|minimal|low|medium|high|xhigh
}

// RunInput is one prompt handed to pi via the legacy Pi.Run API.
type RunInput struct {
	Prompt string
}

// RunResult is the structured outcome accumulated over a session's lifetime.
type RunResult struct {
	AssistantText string            // text of the most-recent assistant message
	Messages      []json.RawMessage // raw messages array from the last agent_end
	ToolCalls     int               // count of tool_execution_end events seen
	ToolErrors    int               // tool_execution_end with isError=true
	StopReason    string            // last seen assistant stop reason
	Stderr        string            // tail of pi's stderr (for diagnostics)
}

// Event is one parsed RPC event from pi's stdout. Streamed via Session.OnEvent.
type Event struct {
	Type string
	Raw  json.RawMessage
}

// UIRequest is an extension_ui_request that needs a reply. Pi blocks waiting
// for the response, so handlers should return quickly (or buffer the request
// for asynchronous resolution and reply later via Session.RespondUI).
type UIRequest struct {
	ID     string
	Method string
	Raw    json.RawMessage
}

// UIResponse describes the reply for a UIRequest. The Method on the request
// dictates which fields matter:
//
//   - "confirm" → Confirmed
//   - "select"  → SelectedID (or Cancelled)
//   - "input", "editor" → Value (or Cancelled)
//
// Fire-and-forget methods (notify, setStatus, setWidget, setTitle,
// setEditorText) never reach a UIHandler; nothing is sent back for them.
type UIResponse struct {
	Cancelled  bool
	Confirmed  bool
	Value      string
	SelectedID string
}

// UIHandler maps a UIRequest to its reply. Installed on Session.OnUI; if nil,
// all interactive methods auto-cancel (matches the unattended default).
type UIHandler func(UIRequest) UIResponse

// DenyAllUI cancels every interactive request. Matches the original Pi.Run
// behavior before the streaming refactor.
func DenyAllUI(req UIRequest) UIResponse {
	if req.Method == "confirm" {
		return UIResponse{Confirmed: false}
	}
	return UIResponse{Cancelled: true}
}

// Session is a live `pi --mode rpc` subprocess.
//
//	sess, err := cfg.Start(ctx)
//	sess.OnEvent = func(e Event) { … }
//	sess.OnUI    = func(req UIRequest) UIResponse { … }
//	sess.SendPrompt("first task")
//	<wait for an agent_end via OnEvent, or whatever signal matches your flow>
//	sess.SendPrompt("follow-up")
//	sess.Close()                 // graceful: close stdin, let pi finish
//	res, err := sess.Wait()      // blocks until subprocess exits
type Session struct {
	cmd   *exec.Cmd
	enc   *jsonlEncoder
	stdin io.WriteCloser
	ctx   context.Context

	// OnEvent and OnUI are set by the caller after Start returns. Both are
	// invoked from the reader goroutine, so handlers should be quick or
	// dispatch their own work asynchronously.
	OnEvent func(Event)
	OnUI    UIHandler

	closed    atomic.Bool
	cancelled atomic.Bool
	pending   atomic.Int32 // outstanding prompts: increments on SendPrompt, decrements on agent_end
	closeOne  sync.Once
	killOne   sync.Once

	stderrBuf strings.Builder
	stderrMu  sync.Mutex
	stderrWG  sync.WaitGroup

	resMu   sync.Mutex
	res     RunResult
	loopErr error

	done chan struct{}
}

// Start spawns pi with the configured args. The reader goroutine begins
// immediately; the returned Session is ready for SendPrompt.
func (p *Pi) Start(ctx context.Context) (*Session, error) {
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

	sess := &Session{
		cmd:   cmd,
		enc:   newJSONLEncoder(stdin),
		stdin: stdin,
		ctx:   ctx,
		done:  make(chan struct{}),
	}

	sess.stderrWG.Add(1)
	go sess.drainStderr(stderr)

	reader := bufio.NewReaderSize(stdout, 64*1024)
	go sess.runReader(reader)

	return sess, nil
}

// SendPrompt forwards a prompt to pi's stdin. Safe to call mid-session for
// multi-turn flows. Returns an error if Close or Cancel has been called.
func (s *Session) SendPrompt(message string) error {
	if s.closed.Load() {
		return errSessionClosed
	}
	s.pending.Add(1)
	err := s.enc.Encode(map[string]any{
		"id":      "wi-prompt-" + time.Now().UTC().Format("150405.000000000"),
		"type":    "prompt",
		"message": message,
	})
	if err != nil {
		s.pending.Add(-1)
	}
	return err
}

// RespondUI replies to a previously-received UIRequest. Useful when OnUI
// can't decide synchronously (e.g. the Telegram bridge waiting for a button
// tap). The OnUI handler should return UIResponse{}-zero and call
// RespondUI later from a different goroutine.
//
// Note: if OnUI returns a non-zero response synchronously, the response is
// sent automatically and RespondUI is not needed.
func (s *Session) RespondUI(req UIRequest, resp UIResponse) error {
	if s.closed.Load() {
		return errSessionClosed
	}
	return s.enc.Encode(buildUIReply(req.Method, req.ID, resp))
}

// Close shuts stdin so pi exits cleanly when it finishes. Idempotent.
func (s *Session) Close() error {
	var err error
	s.closeOne.Do(func() {
		s.closed.Store(true)
		err = s.stdin.Close()
	})
	return err
}

// Cancel sends abort + kills the subprocess. Use for forced termination.
func (s *Session) Cancel() {
	s.killOne.Do(func() {
		_ = s.enc.Encode(map[string]any{"type": "abort"})
		s.cancelled.Store(true)
		s.closed.Store(true)
		_ = s.stdin.Close()
		_ = s.cmd.Process.Kill()
	})
}

// Done is closed when the subprocess has exited.
func (s *Session) Done() <-chan struct{} { return s.done }

// Wait blocks until the subprocess has exited and returns the accumulated
// result. Idempotent. If neither Close nor Cancel has been called, Wait
// closes stdin first so pi exits cleanly.
func (s *Session) Wait() (*RunResult, error) {
	_ = s.Close()
	<-s.done
	s.resMu.Lock()
	defer s.resMu.Unlock()
	s.stderrMu.Lock()
	s.res.Stderr = strings.TrimSpace(s.stderrBuf.String())
	s.stderrMu.Unlock()
	if s.loopErr != nil {
		return &s.res, s.loopErr
	}
	return &s.res, nil
}

func (s *Session) runReader(r *bufio.Reader) {
	defer func() {
		waitDone := make(chan error, 1)
		go func() { waitDone <- s.cmd.Wait() }()
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
			_ = s.cmd.Process.Kill()
			<-waitDone
		}
		s.stderrWG.Wait()
		close(s.done)
	}()

	for {
		line, err := readJSONLLine(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if ctxErr := s.ctx.Err(); ctxErr != nil {
					s.setLoopErr(ctxErr)
					return
				}
				if s.cancelled.Load() {
					return
				}
				if s.pending.Load() > 0 {
					s.setLoopErr(io.ErrUnexpectedEOF)
					return
				}
				return
			}
			s.setLoopErr(fmt.Errorf("read event: %w", err))
			return
		}
		if len(line) == 0 {
			continue
		}

		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			s.setLoopErr(fmt.Errorf("decode event head: %w (raw=%q)", err, truncate(line, 200)))
			return
		}

		if s.dispatch(head.Type, line) {
			return // fatal — loopErr already set
		}
	}
}

// dispatch processes one event. Returns true if the loop should exit.
func (s *Session) dispatch(t string, raw []byte) bool {
	s.resMu.Lock()
	switch t {
	case "response":
		var resp struct {
			Command string `json:"command"`
			Success bool   `json:"success"`
			Err     string `json:"error"`
		}
		_ = json.Unmarshal(raw, &resp)
		if !resp.Success {
			s.loopErr = fmt.Errorf("rpc command %q failed: %s", resp.Command, resp.Err)
			s.resMu.Unlock()
			return true
		}
	case "tool_execution_end":
		var tee struct {
			IsError bool `json:"isError"`
		}
		_ = json.Unmarshal(raw, &tee)
		s.res.ToolCalls++
		if tee.IsError {
			s.res.ToolErrors++
		}
	case "message_end":
		var me struct {
			Message struct {
				Role       string `json:"role"`
				StopReason string `json:"stopReason"`
				Content    any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(raw, &me); err == nil && me.Message.Role == "assistant" {
			s.res.StopReason = me.Message.StopReason
			if txt := extractAssistantText(me.Message.Content); txt != "" {
				s.res.AssistantText = txt
			}
		}
	case "agent_end":
		var ae struct {
			Messages []json.RawMessage `json:"messages"`
		}
		_ = json.Unmarshal(raw, &ae)
		s.res.Messages = ae.Messages
		s.pending.Add(-1)
	}
	s.resMu.Unlock()

	if s.OnEvent != nil {
		// Copy the slice so callers can hold it past this dispatch.
		dup := append([]byte(nil), raw...)
		s.OnEvent(Event{Type: t, Raw: dup})
	}

	if t == "extension_ui_request" {
		s.handleUIRequest(raw)
	}
	return false
}

func (s *Session) handleUIRequest(raw []byte) {
	var req struct {
		ID     string `json:"id"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return
	}

	switch req.Method {
	case "notify", "setStatus", "setWidget", "setTitle", "setEditorText", "set_editor_text":
		return
	}

	handler := s.OnUI
	if handler == nil {
		handler = DenyAllUI
	}
	resp := handler(UIRequest{ID: req.ID, Method: req.Method, Raw: append([]byte(nil), raw...)})

	// Zero-valued response means the handler will reply asynchronously via
	// Session.RespondUI. Distinguish that from a deliberate "deny" by
	// treating *only* an all-zero struct as deferred. Callers that want to
	// synchronously deny should return UIResponse{Cancelled: true}.
	if (resp == UIResponse{}) {
		return
	}

	_ = s.enc.Encode(buildUIReply(req.Method, req.ID, resp))
}

func buildUIReply(method, id string, resp UIResponse) map[string]any {
	reply := map[string]any{
		"type": "extension_ui_response",
		"id":   id,
	}
	switch method {
	case "confirm":
		reply["confirmed"] = resp.Confirmed
	case "select":
		if resp.Cancelled {
			reply["cancelled"] = true
		} else {
			reply["selectedId"] = resp.SelectedID
		}
	case "input", "editor":
		if resp.Cancelled {
			reply["cancelled"] = true
		} else {
			reply["value"] = resp.Value
		}
	default:
		// Unknown method — best effort acknowledgement.
		if resp.Cancelled {
			reply["cancelled"] = true
		}
	}
	return reply
}

func (s *Session) setLoopErr(err error) {
	s.resMu.Lock()
	if s.loopErr == nil {
		s.loopErr = err
	}
	s.resMu.Unlock()
}

func (s *Session) drainStderr(stderr io.Reader) {
	defer s.stderrWG.Done()
	const maxBytes = 16 * 1024
	buf := make([]byte, 4096)
	for {
		n, err := stderr.Read(buf)
		if n > 0 {
			s.stderrMu.Lock()
			if s.stderrBuf.Len()+n > maxBytes {
				cur := s.stderrBuf.String()
				if len(cur) > maxBytes/2 {
					cur = cur[len(cur)-maxBytes/2:]
				}
				s.stderrBuf.Reset()
				s.stderrBuf.WriteString(cur)
			}
			s.stderrBuf.Write(buf[:n])
			s.stderrMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// Run is the legacy one-shot API: send one prompt, wait for the first
// agent_end, close stdin, return the accumulated result. Backward-compatible
// with the pre-Session API used by cmd/spike.
func (p *Pi) Run(ctx context.Context, in RunInput) (*RunResult, error) {
	sess, err := p.Start(ctx)
	if err != nil {
		return nil, err
	}

	sess.OnUI = DenyAllUI

	agentEnd := make(chan struct{}, 1)
	sess.OnEvent = func(e Event) {
		if e.Type == "agent_end" {
			select {
			case agentEnd <- struct{}{}:
			default:
			}
		}
	}

	if err := sess.SendPrompt(in.Prompt); err != nil {
		sess.Cancel()
		_, _ = sess.Wait()
		return nil, fmt.Errorf("send prompt: %w", err)
	}

	select {
	case <-agentEnd:
	case <-sess.Done():
	case <-ctx.Done():
		sess.Cancel()
	}

	return sess.Wait()
}

var (
	errSessionClosed = errors.New("agent: session closed")
)

func extractAssistantText(content any) string {
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
