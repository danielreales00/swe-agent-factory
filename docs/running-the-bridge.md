# Running the Telegram bridge

Step-by-step for booting `cmd/bridge` and exercising the S2 end-to-end loop.

## 1. Prerequisites

- Go 1.22+
- [pi](https://github.com/earendil-works/pi-coding-agent) `0.75.4` on your PATH
- `gh` logged in to the GitHub account that owns the target repo
- An Anthropic API key
- A Telegram bot from [@BotFather](https://t.me/BotFather) (dedicated to the factory, separate from any other bots)
- Your Telegram numeric user id (DM [@userinfobot](https://t.me/userinfobot))
- A local clone of every target repo, each with `.pi/extensions/{permission-gate,plan-gate,request-ship}.ts` checked in (S1 layout)

## 2. Environment

Copy the template and fill it in:

```bash
cp .env.example .env
# edit .env
```

Three vars are required: `BRIDGE_TG_TOKEN`, `BRIDGE_ALLOWED_USER_IDS`, `ANTHROPIC_API_KEY`. The rest have defaults documented in the example.

## 3. Targets registry

The bridge loads `targets/*.yml` at boot. The committed `targets/personal_finance.yml` points at `local_path: /Users/danielreales/personal/personal_finance` — edit if your clone lives elsewhere.

To add a second target: drop a `targets/<name>.yml` with at least `local_path`. Optional fields: `default_branch` (defaults to `main`), `worktree_root`, `branch_prefix`.

## 4. Preflight

```bash
bin/preflight
```

Runs a red/green checklist: pi version, gh active account, required env vars, each `targets/*.yml` resolves to a real git repo with the pi extensions present, and `go build ./cmd/bridge` succeeds. Exits non-zero on any failure. Run this every time before starting the bridge — it catches misconfiguration in seconds, before pi spawns and burns tokens.

## 5. Start the bridge

```bash
go run ./cmd/bridge
```

You should see:

```
bridge: connected as @your_bot (id=…), targets=[personal_finance] default=personal_finance pi.provider=anthropic, allowed_users=[…]
```

Open the bot in Telegram. From an allowlisted user, send `/repo` — expect:

```
📁 current: personal_finance
available: [personal_finance]
```

## 6. The S2 validation (increment 2.9)

A real task, end-to-end. Watch the chat as it goes.

**You type:**

```
add a unit test for format-money with a negative input
```

**Expect (within a minute or so):**

```
🌱 worktree: /Users/.../personal_finance-tg-…
branch: bridge/_pending-tg-…
target: personal_finance
pi ready.
📋 plan proposed: …
📖 read src/finance/domain/formatting.clj
📖 read test/finance/domain/formatting_test.clj
✏️ edit test/finance/domain/formatting_test.clj (1 change)
⚙️ bash: bin/quality
✅ run_quality passed
🚢 request_ship → swe-agent/format-money-negative-test
📦 pushing swe-agent/format-money-negative-test…
✅ draft PR opened
https://github.com/danielreales00/personal_finance/pull/N
💀 pi exited. Worktree cleaned. Next message starts fresh.
```

Tap the PR URL, review the diff. If it looks right, mark the PR ready / merge. If it doesn't, close the PR and send a follow-up task to the bot — the chat persists, so the next message kicks off a fresh pi in a fresh worktree.

## 7. Useful commands while testing

| Command | Effect |
|---|---|
| `/repo` | List targets and the chat's current selection. |
| `/repo <name>` | Switch the chat to another target (only while pi is idle). |
| `/cancel` | Abort the active pi. Worktree is cleaned by the watcher. |
| (anything else) | Forwarded to pi as a prompt. |

If a permission-gate confirm surfaces in the chat as inline buttons, tap ✅ or ❌; auto-denies after 5 minutes.

## 8. Troubleshooting

- **`gh active account` wrong** → `gh auth switch --user danielreales00 && gh auth setup-git`. Preflight catches this.
- **Push 403** → same root cause as above; gh credentials are wired to the wrong account.
- **`extension missing` at session start** → the target's `.pi/extensions/<file>.ts` doesn't exist. Run preflight to confirm.
- **Pi spawn fails immediately** → check `BRIDGE_PI_SESSIONS_ROOT` is writable; check `ANTHROPIC_API_KEY` is exported.
- **Bot doesn't respond** → confirm your numeric user id is in `BRIDGE_ALLOWED_USER_IDS` (non-allowlisted users are silently ignored).
- **Worktree leftovers** → bridge GCs anything older than 24h at boot; otherwise `git worktree prune` in the target repo.
