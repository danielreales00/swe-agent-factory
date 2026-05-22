// spike — throwaway driver that exercises the pi RPC integration end-to-end.
//
// Not part of the production pipeline. Run it by hand to validate that the
// agent loop, permission-gate, and run_bin_quality wiring all work against
// one real target repo before we bake pi into stage/impl.
//
//	go run ./cmd/spike \
//	  --cwd /Users/danielreales/personal/personal_finance \
//	  --provider anthropic \
//	  --model claude-sonnet-4-5 \
//	  --prompt "Open src/finance/app/agent_actions.clj and tell me which actions are currently dispatched. Do not edit anything."
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/agent"
)

func main() {
	var (
		cwd        = flag.String("cwd", "", "target repo absolute path (where pi runs)")
		provider   = flag.String("provider", "anthropic", "pi --provider")
		model      = flag.String("model", "", "pi --model (provider default if empty)")
		prompt     = flag.String("prompt", "", "user prompt for pi")
		sessionDir = flag.String("session-dir", "", "pi --session-dir (default: ./state/spike/<ts>)")
		extDir     = flag.String("extension-dir", "", "directory containing permission-gate.ts and run-bin-quality.ts (default: ./extensions next to this binary)")
		timeout    = flag.Duration("timeout", 3*time.Minute, "hard timeout on the pi run")
		thinking   = flag.String("thinking", "", "pi --thinking level (optional)")
	)
	flag.Parse()

	if *cwd == "" || *prompt == "" {
		fmt.Fprintln(os.Stderr, "spike: --cwd and --prompt are required")
		os.Exit(2)
	}
	if _, err := os.Stat(*cwd); err != nil {
		die("cwd not accessible: %v", err)
	}

	repoRoot := mustGuessRepoRoot()
	if *extDir == "" {
		*extDir = filepath.Join(repoRoot, "extensions")
	}
	extensions := []string{
		filepath.Join(*extDir, "permission-gate.ts"),
		filepath.Join(*extDir, "run-bin-quality.ts"),
	}
	for _, e := range extensions {
		if _, err := os.Stat(e); err != nil {
			die("extension missing: %s", e)
		}
	}
	if *sessionDir == "" {
		*sessionDir = filepath.Join(repoRoot, "state", "spike", time.Now().UTC().Format("20060102T150405Z"))
		if err := os.MkdirAll(*sessionDir, 0o755); err != nil {
			die("mkdir session dir: %v", err)
		}
	}

	pi := &agent.Pi{
		Provider:   *provider,
		Model:      *model,
		Cwd:        *cwd,
		SessionDir: *sessionDir,
		Extensions: extensions,
		Thinking:   *thinking,
	}

	fmt.Printf("--- spike ---\n")
	fmt.Printf("cwd          %s\n", *cwd)
	fmt.Printf("provider     %s\n", *provider)
	fmt.Printf("model        %s\n", orDefault(*model, "(provider default)"))
	fmt.Printf("session-dir  %s\n", *sessionDir)
	fmt.Printf("extensions   %s\n", strings.Join(shortNames(extensions), ", "))
	fmt.Printf("prompt       %s\n", *prompt)
	fmt.Println("-------------")

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	start := time.Now()
	res, err := pi.Run(ctx, agent.RunInput{Prompt: *prompt})
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "spike: pi.Run error after %s: %v\n", elapsed.Round(time.Millisecond), err)
		if res != nil && res.Stderr != "" {
			fmt.Fprintf(os.Stderr, "--- pi stderr tail ---\n%s\n", res.Stderr)
		}
		os.Exit(1)
	}

	fmt.Printf("\nelapsed=%s  tool_calls=%d  tool_errors=%d  stop=%q\n",
		elapsed.Round(time.Millisecond), res.ToolCalls, res.ToolErrors, res.StopReason)
	fmt.Println("--- assistant ---")
	fmt.Println(res.AssistantText)
	if res.Stderr != "" {
		fmt.Println("--- pi stderr tail ---")
		fmt.Println(res.Stderr)
	}
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "spike: "+format+"\n", a...)
	os.Exit(1)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func shortNames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

func mustGuessRepoRoot() string {
	// Spike is run via `go run ./cmd/spike` from the factory repo, so the
	// current working directory is the factory root.
	cwd, err := os.Getwd()
	if err != nil {
		die("getwd: %v", err)
	}
	return cwd
}
