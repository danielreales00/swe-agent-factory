// bridge — Telegram-side entrypoint for the SWE-Agent Factory.
//
// Increment 2.3 scope: each allowlisted message spawns a fresh worktree
// against the configured target repo, runs `ls -la` inside it, reports the
// path + output to the chat, then cleans up. pi wiring lands in 2.4+.
//
// Required env (loadable from .env in cwd):
//
//	BRIDGE_TG_TOKEN              Telegram bot token from @BotFather
//	BRIDGE_ALLOWED_USER_IDS      comma-separated Telegram user IDs
//	BRIDGE_TARGET_NAME           human-readable target tag
//	BRIDGE_TARGET_REPO_PATH      absolute path to a git repo
//
// Optional env:
//
//	BRIDGE_TARGET_DEFAULT_BRANCH (default "main")
//	BRIDGE_TARGET_WORKTREE_ROOT  (default: parent dir of repo)
//	BRIDGE_TARGET_BRANCH_PREFIX  (default "bridge/")
//
// Run:
//
//	go run ./cmd/bridge
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/danielreales00/swe-agent-factory/internal/bridge"
)

const (
	worktreeTaskTimeout = 60 * time.Second
	startupGCTimeout    = 30 * time.Second
	staleWorktreeMaxAge = 24 * time.Hour
	lsMaxLines          = 20
)

func main() {
	logger := log.New(os.Stderr, "bridge: ", log.LstdFlags|log.Lmsgprefix)

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		logger.Printf(".env load warning: %v", err)
	}

	token := os.Getenv("BRIDGE_TG_TOKEN")
	if token == "" {
		die("BRIDGE_TG_TOKEN is required (set in .env or env)")
	}
	ids, err := bridge.ParseIDs(os.Getenv("BRIDGE_ALLOWED_USER_IDS"))
	if err != nil {
		die("BRIDGE_ALLOWED_USER_IDS: %v", err)
	}
	if len(ids) == 0 {
		die("BRIDGE_ALLOWED_USER_IDS must contain at least one user id")
	}
	allow := bridge.NewAllowlist(ids)

	target, err := bridge.TargetFromEnv()
	if err != nil {
		die("target: %v", err)
	}
	wt := bridge.NewWorktreeManager(target)

	gcCtx, cancelGC := context.WithTimeout(context.Background(), startupGCTimeout)
	if n, err := wt.GC(gcCtx, staleWorktreeMaxAge); err != nil {
		logger.Printf("worktree GC error: %v", err)
	} else if n > 0 {
		logger.Printf("worktree GC: cleaned %d stale", n)
	}
	cancelGC()

	client, err := bridge.NewClient(token, allow, logger)
	if err != nil {
		die("init telegram: %v", err)
	}
	name, botID := client.Self()
	logger.Printf("connected as @%s (id=%d), target=%s repo=%s, allowed_users=%v",
		name, botID, target.Name, target.RepoPath, ids)

	handler := func(ctx context.Context, sess *bridge.Session, msg bridge.Message) {
		runOneTask(ctx, logger, client, wt, sess, msg)
	}
	router := bridge.NewRouter(handler)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := client.Run(ctx, router.Dispatch); err != nil && !errors.Is(err, context.Canceled) {
		die("run: %v", err)
	}
	logger.Println("shutting down")
}

// runOneTask is a 2.3 stand-in for what 2.4+ will do with pi: spawn a
// worktree, run a canned command inside it, report results, clean up.
func runOneTask(
	parent context.Context,
	logger *log.Logger,
	client *bridge.Client,
	wt *bridge.WorktreeManager,
	sess *bridge.Session,
	msg bridge.Message,
) {
	ctx, cancel := context.WithTimeout(parent, worktreeTaskTimeout)
	defer cancel()

	logger.Printf("task chat=%d user=%d text=%q", msg.ChatID, msg.UserID, msg.Text)

	path, branch, err := wt.Create(ctx, sess.ChatID)
	if err != nil {
		logger.Printf("create worktree: %v", err)
		_, _ = client.Send(msg.ChatID, "❌ create worktree: "+err.Error())
		return
	}

	out, lsErr := runLS(ctx, path)
	reply := fmt.Sprintf("🌱 worktree: %s\nbranch: %s\n\nls -la:\n%s",
		path, branch, out)
	if lsErr != nil {
		reply += "\n\n(ls error: " + lsErr.Error() + ")"
	}
	if _, err := client.Send(msg.ChatID, reply); err != nil {
		logger.Printf("send: %v", err)
	}

	if err := wt.Remove(ctx, path, branch); err != nil {
		logger.Printf("remove worktree: %v", err)
		_, _ = client.Send(msg.ChatID, "⚠️ worktree cleanup failed: "+err.Error())
	}
}

func runLS(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "ls", "-la")
	cmd.Dir = dir
	out, err := cmd.Output()
	text := strings.TrimRight(string(out), "\n")
	lines := strings.Split(text, "\n")
	if len(lines) > lsMaxLines {
		lines = append(lines[:lsMaxLines], "...(truncated)")
		text = strings.Join(lines, "\n")
	}
	return text, err
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "bridge: "+format+"\n", a...)
	os.Exit(1)
}
