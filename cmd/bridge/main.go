// bridge — Telegram-side entrypoint for the SWE-Agent Factory.
//
// Increment 2.2 scope: boot, connect to Telegram with a bot token, accept
// long-polled messages from allowlisted users, echo them back. No pi, no
// worktrees, no commands yet.
//
// Required env (loadable from .env in cwd):
//
//	BRIDGE_TG_TOKEN          - Telegram bot token from @BotFather
//	BRIDGE_ALLOWED_USER_IDS  - comma-separated Telegram user IDs
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
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/danielreales00/swe-agent-factory/internal/bridge"
)

func main() {
	logger := log.New(os.Stderr, "bridge: ", log.LstdFlags|log.Lmsgprefix)

	// Optional .env load. Missing file is fine; a malformed file is worth
	// surfacing.
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

	client, err := bridge.NewClient(token, allow, logger)
	if err != nil {
		die("init telegram: %v", err)
	}

	name, botID := client.Self()
	logger.Printf("connected as @%s (id=%d), allowed users: %v", name, botID, ids)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handler := func(ctx context.Context, msg bridge.Message) {
		logger.Printf("recv chat=%d user=%d text=%q", msg.ChatID, msg.UserID, msg.Text)
		if _, err := client.Send(msg.ChatID, "echo: "+msg.Text); err != nil {
			logger.Printf("send error: %v", err)
		}
	}

	if err := client.Run(ctx, handler); err != nil && !errors.Is(err, context.Canceled) {
		die("run: %v", err)
	}
	logger.Println("shutting down")
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "bridge: "+format+"\n", a...)
	os.Exit(1)
}
