package bridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/agent"
)

// PiConfig wraps the env-derived inputs for spawning a pi subprocess
// against any target worktree.
type PiConfig struct {
	Provider     string   // BRIDGE_PI_PROVIDER (default "anthropic")
	Model        string   // BRIDGE_PI_MODEL    (default: provider default)
	SessionsRoot string   // BRIDGE_PI_SESSIONS_ROOT (default ".bridge/sessions")
	Env          []string // passthrough env (ANTHROPIC_API_KEY, etc.)
	Thinking     string   // BRIDGE_PI_THINKING level (optional)
}

// PiConfigFromEnv loads PiConfig from BRIDGE_PI_* env vars + the host
// process's ANTHROPIC_API_KEY (which pi needs).
func PiConfigFromEnv() PiConfig {
	provider := os.Getenv("BRIDGE_PI_PROVIDER")
	if provider == "" {
		provider = "anthropic"
	}
	sessionsRoot := os.Getenv("BRIDGE_PI_SESSIONS_ROOT")
	if sessionsRoot == "" {
		sessionsRoot = ".bridge/sessions"
	}
	var env []string
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		env = append(env, "ANTHROPIC_API_KEY="+k)
	}
	return PiConfig{
		Provider:     provider,
		Model:        os.Getenv("BRIDGE_PI_MODEL"),
		SessionsRoot: sessionsRoot,
		Env:          env,
		Thinking:     os.Getenv("BRIDGE_PI_THINKING"),
	}
}

// NewPiSpawner returns a PiSpawner that:
//   - mkdirs a per-chat session dir under cfg.SessionsRoot
//   - locates the .pi/extensions/*.ts files inside the worktree (the S1
//     config that was checked in to personal_finance)
//   - hands all of that to agent.Pi.Start
func NewPiSpawner(cfg PiConfig) PiSpawner {
	return func(ctx context.Context, worktree string, chatID int64) (*agent.Session, error) {
		ts := time.Now().UTC().Format("20060102T150405Z")
		sessionDir := filepath.Join(cfg.SessionsRoot,
			fmt.Sprintf("chat-%d-%s", chatID, ts))
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir session dir: %w", err)
		}

		extDir := filepath.Join(worktree, ".pi", "extensions")
		extensions := []string{
			filepath.Join(extDir, "permission-gate.ts"),
			filepath.Join(extDir, "request-ship.ts"),
			filepath.Join(extDir, "plan-gate.ts"),
		}
		for _, e := range extensions {
			if _, err := os.Stat(e); err != nil {
				return nil, fmt.Errorf("extension missing: %s", e)
			}
		}

		pi := agent.Pi{
			Provider:   cfg.Provider,
			Model:      cfg.Model,
			Cwd:        worktree,
			SessionDir: sessionDir,
			Extensions: extensions,
			Env:        cfg.Env,
			Thinking:   cfg.Thinking,
		}
		return pi.Start(ctx)
	}
}
