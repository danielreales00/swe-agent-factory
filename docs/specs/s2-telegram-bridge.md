# Spec S2 — Telegram bridge: pi in Telegram

**Status:** Draft (2026-05-22)
**Phase:** S2 of the SWE-Agent Factory roadmap
**Depends on:** S1 (shipped 2026-05-22 — `.pi/` config + `bin/agent` wrapper in `personal_finance/main`)
**Unblocks:** S3 (autonomous cron), S4 (multi-source signals)

## 1. Vision

**Telegram becomes pi's chat surface.** The bot is not a launchpad with verbs around pi; it *is* pi, with Telegram as the message transport.

Every message you send → pi sees it as user input.
Every pi assistant message → arrives in your chat.
Every tool call → renders as a compact one-liner in chat.
Plan-gate's questions, request_ship's PR draft, the whole S1 workflow → all happen over Telegram instead of a TTY.

The bridge is **a protocol translator + a worktree manager**. Nothing more.

## 2. Scope

### In
- A Go service in `swe-agent-factory/` that:
  - Listens for Telegram messages on a dedicated bot
  - Maintains per-chat session state (active pi process, worktree, target repo)
  - Spawns `pi --mode rpc` per task in the right worktree
  - Translates pi's RPC event stream → Telegram messages
  - Translates Telegram user messages → pi's stdin `prompt` commands
  - Surfaces permission-gated tool calls as Telegram inline buttons
  - Captures `request_ship` events and opens draft PRs via `gh`
- Multi-repo support via `/repo <name>` command (registry under `targets/`)
- New BotFather bot dedicated to the factory (separate from finance bot)
- Allowlist by Telegram user_id (one user for S2: yours)

### Out
- Always-on hosting (S2 runs on your laptop; VM migration is post-S2)
- Linear / GitHub / Axiom signal sources (S3, S4)
- Web UI (S6)
- Multi-user / multi-tenancy beyond your own user_id (S5+)
- Spec-first stages (`draft`, `file`) — those are S3 work
- Reviving the full S1 `WorkItem` pipeline plumbing (re-enters at S3)

## 3. Decisions (the four picks)

| # | Question | Choice | Why |
|---|---|---|---|
| 1 | Session lifecycle | **Chat persists, pi-per-task** | Chat is the durable session; bridge spawns a fresh pi + worktree per task. When pi exits (request_ship or otherwise), the chat stays alive. User just keeps typing tasks. |
| 2 | Chat rendering | **Assistant + compact tool lines** | Forward pi's assistant text verbatim. Render tool calls as one-line summaries (`📖 read X`, `✏️ edit Y`, `✅ quality passed`). Collapse tool results unless they error. |
| 3 | Repo scope | **Multi-repo via `/repo` command** | Even though only `personal_finance` is configured at S2, architecture goes multi-repo from day one. `/repo personal_finance` selects target. New chats default to first target. |
| 4 | Permission model | **Auto-approve safe, prompt rest** | Anything passing `permission-gate.ts` denylist auto-approves silently. Anything the gate would block surfaces as inline Telegram buttons (`approve` / `deny`). |

## 4. Architecture

```
   ┌──────────────────────────────────────────────────────────────┐
   │  Telegram Bot API (long-polling or webhook)                  │
   └────────────────────────┬─────────────────────────────────────┘
                            │ user_id allowlist
                            ▼
   ┌──────────────────────────────────────────────────────────────┐
   │  bridge (Go, swe-agent-factory/cmd/bridge)                   │
   │                                                              │
   │   ┌──────────────┐    ┌────────────────────────────────────┐ │
   │   │ TelegramIn   │───▶│ Session router                     │ │
   │   │   handlers   │    │  (chat_id → session)               │ │
   │   └──────────────┘    └────────────┬───────────────────────┘ │
   │                                    │                         │
   │                       ┌────────────▼───────────────────────┐ │
   │                       │ Session                            │ │
   │                       │  - target repo (TargetID)          │ │
   │                       │  - worktree path                   │ │
   │                       │  - active pi process (or nil)      │ │
   │                       │  - pending tool approvals          │ │
   │                       └────────────┬───────────────────────┘ │
   │                                    │                         │
   │   ┌────────────────────────────────▼───────────────────────┐ │
   │   │ pi driver (extends internal/agent/pi.go)               │ │
   │   │   - streaming callbacks (not just final result)        │ │
   │   │   - extension_ui_request → Telegram button             │ │
   │   │   - request_ship → ship flow                           │ │
   │   └────────────────────────────────┬───────────────────────┘ │
   │                                    │                         │
   │   ┌────────────────────────────────▼───────────────────────┐ │
   │   │ Tool renderer                                          │ │
   │   │   pi event → compact Telegram line                     │ │
   │   └────────────────────────────────┬───────────────────────┘ │
   │                                    │                         │
   │                       ┌────────────▼───────────────────────┐ │
   │                       │ TelegramOut sender                 │ │
   │                       │  (rate-limited, edits-in-place     │ │
   │                       │   for streaming assistant text)    │ │
   │                       └────────────────────────────────────┘ │
   └──────────────────────────────────────────────────────────────┘
                            │
                            ▼
   ┌──────────────────────────────────────────────────────────────┐
   │  pi --mode rpc                                               │
   │   cwd = ~/personal/personal_finance-agent-<ts>/  (worktree)  │
   │   --session-dir ~/personal/personal_finance/.pi/sessions/... │
   │   --extension permission-gate.ts                             │
   │   --extension plan-gate.ts                                   │
   │   --extension request-ship.ts                                │
   └──────────────────────────────────────────────────────────────┘
                            │
                            ▼
                   git worktree (personal_finance)
```

## 5. Component design

### 5.1 Session model

```go
type Session struct {
    ChatID       int64
    UserID       int64
    Target       TargetID         // selected via /repo, defaults to first target
    Worktree     string           // path; "" when between tasks
    PiCancel     context.CancelFunc // nil when no pi is running
    PiDoneCh     chan struct{}    // closed when pi exits
    PendingTool  map[string]Pending // tool_call_id → awaiting approval
    LastBotMsgID int              // for stream-edit-in-place of assistant text
    CreatedAt    time.Time
    LastTaskEnd  time.Time
}
```

**Lifecycle:**

1. First message in a new chat → reject unless `user_id` is allowlisted.
2. `/repo <name>` → set `Session.Target`. Persists for the chat's lifetime.
3. First non-command message → spawn worktree, spawn pi, send prompt.
4. While pi is alive: route messages to pi's stdin as `prompt` commands (the same RPC verb pi accepts mid-session for follow-ups).
5. Pi exits (request_ship, error, or `/cancel`): teardown worktree-cleanup decision (see 5.4), session waits for next message.
6. Next non-command message: fresh pi + fresh worktree.

**State persistence:** in-memory only at S2 (laptop daemon, single process). If the bridge restarts, in-flight pi processes are abandoned but their worktrees remain on disk for manual cleanup. Acceptable trade-off for personal-use S2.

### 5.2 Pi RPC driver — extending `internal/agent/pi.go`

The existing driver (`internal/agent/pi.go`) was built for one-shot Phase-1 use: it spins up pi, sends one `prompt`, drains the stream, returns the final result at `agent_end`. **For S2 we need streaming + multi-turn.**

Changes:

- **Add an event callback** `OnEvent func(Event)` on `Pi`. Called for every parsed RPC event (assistant message, tool call, tool result, extension_ui_request, etc.) so the bridge can react in real time.
- **Add multi-turn support:** expose `SendPrompt(message string) error` on the running pi to forward additional Telegram messages as `prompt` events into pi's stdin. (Pi's RPC docs confirm `prompt` is accepted mid-session for follow-ups.)
- **Externalize `extension_ui_request` handling:** today `handleExtensionUI` always cancels/denies (unattended default). The bridge needs to route those requests to Telegram buttons and reply with the user's choice asynchronously.
- **Keep the existing JSONL framing logic** (LF-only, the `readJSONLLine` is already correct).

New shape (sketch):

```go
type Event struct {
    Type    string          // "assistant_delta" | "tool_call" | "tool_result" |
                            // "extension_ui_request" | "agent_end" | ...
    Raw     json.RawMessage
    // typed convenience fields filled per Type
}

type PiSession struct {
    cmd     *exec.Cmd
    enc     *jsonlEncoder
    OnEvent func(Event)
    OnUI    func(req UIRequest) UIResponse  // bridge implements (Telegram buttons)
    done    chan error
}

func StartPi(ctx context.Context, cfg Pi) (*PiSession, error) { ... }
func (s *PiSession) SendPrompt(msg string) error { ... }
func (s *PiSession) Cancel() { ... }
```

`Pi.Run` (current one-shot API) stays as a thin wrapper over `StartPi + drain` so existing call sites don't break.

### 5.3 Telegram rendering

**Assistant text:** stream via Telegram message edits. First delta creates a message; subsequent deltas edit it in place. Avoids one-message-per-token spam. Rate-limited to ≤1 edit per ~500ms (Telegram's edit rate cap is 1/sec per chat, but bursts get throttled).

**Tool calls** render as bot messages with compact single-line summaries:

| Pi tool | Rendered as |
|---|---|
| `read` | `📖 read src/finance/x.clj` |
| `grep` | `🔍 grep "pattern"` |
| `find` / `ls` | `📂 ls src/finance/` |
| `edit` | `✏️ edit src/finance/x.clj (3 changes)` |
| `write` | `📝 write src/finance/x.clj (+42 lines)` |
| `bash` | `⚙️ bash: git log --oneline -20` |
| `run_quality` (custom) | `🧪 run_quality...` then update to `✅ run_quality passed` or `❌ run_quality failed` |
| `propose_plan` (custom) | `📋 plan proposed: <summary first line> (3 questions)` |
| `request_ship` (custom) | `🚢 request_ship → <branch>` |

**Tool results:** collapsed by default. If `isError: true` on `tool_execution_end`, the bridge sends the result body as a separate message so the user sees what broke. Otherwise silent.

**Errors / abnormal exits:** if pi dies without `request_ship`, bridge posts a final status: `❌ pi exited unexpectedly (stop_reason=...)`.

### 5.4 Permission model — auto-approve safe, prompt rest

Pi sends `extension_ui_request` events when its `permission-gate.ts` extension flags something. Today (S1) the gate uses a hard denylist (`rm -rf`, `sudo`, force-push, etc.). S2 keeps that gate but **promotes the human gate from "TTY prompt or auto-deny" to "Telegram inline button or auto-deny."**

Flow:

1. Pi tries to invoke `edit`/`write`/`bash`.
2. `permission-gate.ts` evaluates:
   - **Denylist match** → respond with deny, post `🚫 blocked: <reason>` to chat (no user interaction).
   - **Auto-approve allowlist** (read, grep, ls, find, edit/write inside cwd) → respond with approve silently.
   - **Anything else** → emit `extension_ui_request{method:"confirm"}` with a description.
3. Bridge sees the `extension_ui_request`, posts a Telegram message with inline buttons:
   ```
   pi wants to run:
     bash: <command>
   [✅ approve] [❌ deny]
   ```
4. User taps → bridge replies `extension_ui_response{confirmed:true/false, id:req.id}`.
5. Pi continues or skips.

**Cost ceiling:** If user is unresponsive for 5 minutes on a prompt, the bridge auto-denies and posts a timeout message. Prevents zombie pi sessions.

### 5.5 Worktree management

**Decision:** persistent worktrees, one per active task, parked across the task's lifetime, cleaned up on success.

```
~/personal/personal_finance                       ← main repo (live)
~/personal/personal_finance-tg-<chat_id>-<ts>     ← worktree per task
```

- New task in chat → `git worktree add <path> -b swe-agent/_pending-<ts>` from `main`.
- Pi runs in the worktree; commits land on the placeholder branch.
- `request_ship` arrives → bridge:
  1. Renames branch from `_pending-*` to the user-supplied `swe-agent/<slug>`.
  2. Pushes to origin.
  3. Runs `gh pr create --draft --fill` from the worktree.
  4. Posts PR URL to chat.
  5. Removes the worktree (keeps the branch ref).
- Pi exits without ship (error or cancel) → bridge keeps the worktree, posts the path for manual inspection (`💀 pi exited without ship. Worktree at <path>`).
- Stale worktrees (>24h, no active pi process): GC on bridge startup, identical to `bin/agent`'s logic.

### 5.6 Multi-repo via `/repo`

`targets/` dir already exists in the factory repo. Each `targets/<name>.yml`:

```yaml
name: personal_finance
repo_path: /Users/danielreales/personal/personal_finance
default_branch: main
worktree_prefix: /Users/danielreales/personal/personal_finance-tg-
manifest_path: .swe-agent.yml
allowed_chats: [<your_telegram_user_id>]   # optional per-target restriction
```

Bridge loads `targets/*.yml` at startup into a registry. Commands:

- `/repo` → list configured repos + current selection
- `/repo <name>` → select for this chat; errors if name unknown

A new chat defaults to the first registered target. S2 ships with `personal_finance` only; adding a second target is one YAML file (no code change).

### 5.7 The two commands at S2

Despite the "no command suite" framing, two verbs are unavoidable:

- **`/repo`** — repo selector (only purpose: pick target; can't be expressed as natural chat to pi since pi doesn't know about the registry)
- **`/cancel`** — graceful abort of the active pi session (sends `abort` to RPC, kills process, cleans worktree if empty)

Anything else is just messages to pi.

## 6. File layout in `swe-agent-factory/`

New / changed:

| Path | Action | Purpose |
|---|---|---|
| `cmd/bridge/main.go` | new | Bridge entrypoint: load config, start Telegram poller, run session router |
| `internal/bridge/session.go` | new | Session struct + state machine |
| `internal/bridge/router.go` | new | chat_id → session lookup + lifecycle |
| `internal/bridge/render.go` | new | Pi Event → Telegram message translator |
| `internal/bridge/telegram.go` | new | Telegram client wrapper (in/out) |
| `internal/bridge/worktree.go` | new | Worktree create/cleanup helpers |
| `internal/bridge/ship.go` | new | request_ship → branch rename + push + `gh pr create` |
| `internal/agent/pi.go` | extend | Add streaming callbacks + multi-turn `SendPrompt` + external UI handler |
| `internal/config/target.go` | extend (already exists) | Parse `targets/<name>.yml` registry |
| `targets/personal_finance.yml` | new | Single S2 target |
| `docs/specs/s2-telegram-bridge.md` | new (this file) | The spec |

Dependencies to add (`go.mod`):

- `github.com/go-telegram-bot-api/telegram-bot-api/v5` (or `github.com/go-telegram/bot` — pick at impl time; standard choices)

## 7. Increments within S2

All nine increments shipped to `main` between 2026-05-22 and 2026-05-25 as separate PRs (#1–#10, modulo the S1 carry-over). The smoke test in 2.9 is operator-driven; the runbook lives at `docs/running-the-bridge.md`.

| # | Deliverable | Validates | Status |
|---|---|---|---|
| 2.1 | Extend `pi.go` with streaming callbacks + `SendPrompt`. Unit test against a fake pi binary that echoes a scripted event sequence. | The RPC primitive multi-turn flow works headlessly. | ✅ |
| 2.2 | Telegram client + echo bot scaffold. `cmd/bridge` boots, joins your chat, echoes anything you send. Allowlist enforced. | Telegram plumbing + auth gate. | ✅ |
| 2.3 | Session model + worktree lifecycle. First message in chat spawns a worktree, runs `ls` (not pi) via `bash`, reports back. | Worktree management without pi entanglement. | ✅ |
| 2.4 | Wire pi into the session. First non-command message spawns pi with the same `.pi/` config S1 uses, streams assistant deltas to Telegram. No tools yet. | Pi-in-Telegram bare bones. | ✅ |
| 2.5 | Tool rendering. Tool calls + results render as compact lines. | The user can watch pi work. | ✅ |
| 2.6 | Permission prompts via inline buttons. Plan-gate's questions surface as bot messages and your replies route back to pi. | Multi-turn user input fully wired. | ✅ |
| 2.7 | `request_ship` capture → branch rename → push → `gh pr create --draft`. PR URL posted to chat. | End-to-end loop closed. | ✅ |
| 2.8 | `/repo` command + `targets/` registry loading. `/cancel` command. | Multi-repo entry point exists; abort works. | ✅ |
| 2.9 | Real-task validation: tell the bot "add a unit test for `format-money` with a negative input", review the diff, ship. | S2 done. | ⏳ code-complete; smoke test deferred |

Each increment is its own PR on `swe-agent-factory/main`. Each PR is reviewable in isolation, mirroring the S1 sequence.

## 8. Open questions / deferred decisions

1. **Long-polling or webhook?** Long-polling is trivial from a laptop (no TLS, no ngrok). Webhook is faster but needs a public endpoint. **S2 starts with long-polling.** Webhook can come when we move to a VM.

2. **Telegram client library.** Two viable Go options: `go-telegram-bot-api/v5` (mature, widely used) and `go-telegram/bot` (more modern, generics-friendly). Decide at increment 2.2 — both fit. Default: `go-telegram-bot-api/v5` unless an obvious issue surfaces.

3. **What if user sends a message while pi is mid-tool-call?** Two policies: (a) buffer until pi pauses for input, or (b) inject as a new `prompt` immediately (pi's RPC handles this — pi will see it as a user follow-up after the current tool returns). **Pick (b).** It matches "Telegram is the chat surface" — typing into a chat shouldn't stall.

4. **Markdown formatting.** Pi emits markdown; Telegram has its own `MarkdownV2` dialect with escape pitfalls. **Strategy:** use Telegram `HTML` parse mode and convert pi's markdown → HTML server-side (simpler than escaping MarkdownV2). Code blocks become `<pre>`, inline code becomes `<code>`, bold/italic preserved.

5. **Streaming-edit rate limiting.** Telegram throttles message edits. **Strategy:** debounce to 1 edit per 500ms; final edit on `message_end` always flushes. If the assistant message exceeds ~4000 chars (Telegram's per-message limit), split into a new bot message.

6. **Bridge process restart resilience.** S2 = in-memory state; bridge restart = abandoned worktrees + dead sessions. Acceptable for personal use, but a `/recover` command that scans for orphan worktrees and asks "resume or delete?" could be a quick win. **Deferred to post-2.9 polish.**

7. **Secrets at runtime.** Bridge needs: Telegram bot token, `ANTHROPIC_API_KEY` (for pi), GitHub auth (for `gh`). All loaded from a `.env` file at startup. No secrets in `targets/*.yml`.

## 9. Hard constraints (carried from S1)

- Bridge process never modifies `.env`, `secrets/`, `.swe-agent.yml` in the target.
- Bridge never pushes to `main`, only to `swe-agent/*` branches.
- Bridge never force-pushes.
- Pi's denylist (permission-gate.ts) remains canonical; bridge's auto-approve is a *narrower* allowlist on top, never wider.
- Cost ceiling: same 30-tool-call ceiling from S1's work.md remains in place (the prompt template is unchanged).

## 10. Verification

After all increments:

```bash
# Terminal 1
cd ~/personal/swe-agent-factory
make bridge   # or: go run ./cmd/bridge

# Phone: open @<your>_swe_agent_bot
> /repo
< Current: personal_finance. Available: [personal_finance]
> add a unit test for format-money with a negative input
< 📋 plan proposed: "Add a test asserting format-money handles negatives" (0 questions)
< 📖 read src/finance/domain/formatting.clj
< 📖 read test/finance/domain/formatting_test.clj
< ✏️ edit test/finance/domain/formatting_test.clj (1 change)
< 🧪 run_quality...
< ✅ run_quality passed
< 🚢 request_ship → swe-agent/format-money-negative-test
< https://github.com/.../pull/N  (draft)
```

Then:
- Open the PR in GitHub, review the diff, merge or reject.
- Bridge handles the next task in the same chat.
- All pi session JSONLs accumulate under `~/personal/personal_finance/.pi/sessions/` for audit (same path S1 uses).

## 11. Risks

1. **Telegram chat as ephemeral UI** — chat history doesn't survive bridge restarts mid-pi (session state is in-memory). Mitigation: keep S2 sessions short; persist via worktree on disk; explicit `/recover` later.
2. **API key in two places** — `.env` in `personal_finance` (used by `bin/agent` in S1) and `.env` in `swe-agent-factory` (S2 bridge). Mitigation: factory's `.env` symlinks or loads `personal_finance/.env` to keep one source of truth, decided at increment 2.2.
3. **Pi RPC API drift** — pi is pre-1.0; event shapes can change. Mitigation: pin pi version in factory's setup; spike already validated the contract at v0.75.4.
4. **Telegram rate limits on long sessions** — 30 messages/sec global, ~1 message/sec to the same chat. A chatty pi session could hit this. Mitigation: batch tool-call lines (multiple `read`s in a row → one message); aggressive debouncing on assistant deltas.
5. **Worktree disk usage** — each task is a full checkout. `personal_finance` is ~50MB so 10 worktrees = 500MB. Mitigation: GC on bridge startup + after-ship cleanup. Becomes a real issue only at S5 (many repos).

## 12. After S2

Once S2 is real:

- **S3 (cron):** the bridge stays; we add a scheduler that constructs synthetic prompts and runs them through the same pi-spawn path. The "Telegram surface" becomes one output channel; logs/Linear become others.
- **S4 (multi-source):** signals feed into the same per-chat session machinery — Linear ticket "appears" as a bot message saying *"new feature request: …"* and asks the user `[run] [skip]`.
- **S5 (multi-repo concurrent):** session router already supports multiple chats; adding a process pool is a natural extension.
