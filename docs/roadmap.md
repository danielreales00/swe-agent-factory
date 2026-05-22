# Roadmap

## S1 — Terminal-driven ✅

**What it is:** A human gives the agent a task in plain English. The agent reads the codebase, plans, implements, passes the quality gate, and opens a branch. The human reviews and merges.

**What's live:**
- `pi` runs with `.pi/prompts/work.md` as the task template
- `plan-gate` extension blocks any edits until the agent has shown its plan
- `permission-gate` blocks destructive operations
- `request_ship` validates and records the completion signal
- `./swe-agent scan` collects work items from Axiom and prints a digest
- The Go `Pi` client drives `pi --mode rpc` headlessly (validated by spike)

**What the human does:** runs `pi`, reviews the PR.

---

## S2 — Telegram bridge 🔜

**What it is:** The factory runs as a server. When a gap is detected, it asks you on Telegram before implementing. You approve or skip from your phone. When the agent is done, it sends you the PR link.

**New pieces needed:**
- HTTP server with a Telegram webhook handler
- Worker that spawns `pi --mode rpc` per approved work item
- Telegram DM routing for `extension_ui_request` events from the agent
- `request_ship` event captured server-side → GitHub PR opened automatically

**What the human does:** taps [Yes] or [No] on their phone.

---

## S3 — Autonomous cron 🔜

**What it is:** The factory scans on a schedule. High-confidence items (clear error, analogous implementation exists) get implemented automatically. Low-confidence items still get a Telegram nudge.

**New pieces needed:**
- Scheduler entrypoint (cron or ticker)
- Confidence scoring in the triage step
- `draft` stage: agent writes a spec before coding (for `capability_gap` type)

**What the human does:** reviews PRs. Occasionally overrides a triage decision.

---

## S4 — Multi-source 🔜

**What it is:** Linear tickets, GitHub issues, Slack `:robot:` reactions — all become work items in the same queue. The factory is source-agnostic because every source implements the same interface.

**New pieces needed:**
- `internal/signal/linear` — Linear GraphQL client
- `internal/signal/github` — GitHub Issues REST client
- `internal/signal/slack` — Slack Events API handler
- `file` stage: agent creates a GitHub issue or Linear ticket before coding (for spec-first types)

---

## S5 — Multi-repo 🔜

**What it is:** Any codebase opts in with a `.swe-agent.yml` file. The factory manages them all from a single process with a shared work-item store and a concurrent agent pool.

**New pieces needed:**
- `targets/` directory scanning (today: one file per repo, loaded manually)
- Git cloning at scan time (today: operator points at local checkout)
- Agent pool with configurable concurrency limit
- Per-repo session isolation

---

## S6 — Factory floor UI 🔜

**What it is:** A web dashboard showing every agent, every work item, every repo in real time. Signal feed, live agent sessions, review queue, repo health metrics, full audit log.

**Key views:**
- **Signal timeline** — incoming items across all repos, triage decisions, manual overrides
- **Active agents** — live progress, current tool call, elapsed time, abort button
- **Review queue** — open branches with diff stats, one-click merge
- **Health board** — test count trend, mean time to fix, agent error rate per repo
- **Session viewer** — replay any agent session, see every tool call and reasoning step
