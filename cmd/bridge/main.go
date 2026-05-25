// bridge — Telegram-side entrypoint for the SWE-Agent Factory.
//
// 2.7 scope: each Telegram chat maps to a long-lived pi --mode rpc session
// running in a fresh git worktree of the target repo. User messages become
// pi prompts; pi assistant messages stream back to the chat. Tool calls
// render as compact lines (2.5). Permission-gated calls surface as inline
// buttons (2.6). `request_ship` triggers `git push` + `gh pr create --draft`
// and posts the PR URL back to the chat (2.7). No /repo command yet (2.8).
//
// Required env (loadable from .env in cwd):
//
//	BRIDGE_TG_TOKEN              Telegram bot token from @BotFather
//	BRIDGE_ALLOWED_USER_IDS      comma-separated Telegram user IDs
//	BRIDGE_TARGET_NAME           human-readable target tag
//	BRIDGE_TARGET_REPO_PATH      absolute path to a git repo
//	ANTHROPIC_API_KEY            pi's LLM credential
//
// Optional env:
//
//	BRIDGE_TARGET_DEFAULT_BRANCH (default "main")
//	BRIDGE_TARGET_WORKTREE_ROOT  (default: parent dir of repo)
//	BRIDGE_TARGET_BRANCH_PREFIX  (default "bridge/")
//	BRIDGE_PI_PROVIDER           (default "anthropic")
//	BRIDGE_PI_MODEL              (default: provider default)
//	BRIDGE_PI_SESSIONS_ROOT      (default ".bridge/sessions")
//	BRIDGE_PI_THINKING           (off|minimal|low|medium|high|xhigh)
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/danielreales00/swe-agent-factory/internal/bridge"
)

const (
	startupGCTimeout    = 30 * time.Second
	staleWorktreeMaxAge = 24 * time.Hour
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

	piCfg := bridge.PiConfigFromEnv()
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		logger.Println("warning: ANTHROPIC_API_KEY is not set; pi will fail on first prompt")
	}

	client, err := bridge.NewClient(token, allow, logger)
	if err != nil {
		die("init telegram: %v", err)
	}
	name, botID := client.Self()
	logger.Printf("connected as @%s (id=%d), target=%s repo=%s pi.provider=%s, allowed_users=%v",
		name, botID, target.Name, target.RepoPath, piCfg.Provider, ids)

	handler := &bridge.Handler{
		Worktrees: wt,
		Client:    client,
		SpawnPi:   bridge.NewPiSpawner(piCfg),
		Ship:      bridge.NewDefaultShipper(),
		Logger:    logger,
	}
	router := bridge.NewRouter(handler.Run)
	router.OnCallback = handler.HandleCallback

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handlers := bridge.Handlers{
		OnMessage:  router.Dispatch,
		OnCallback: router.DispatchCallback,
	}
	if err := client.Run(ctx, handlers); err != nil && !errors.Is(err, context.Canceled) {
		die("run: %v", err)
	}
	logger.Println("shutting down")
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "bridge: "+format+"\n", a...)
	os.Exit(1)
}
