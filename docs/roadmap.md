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

## S2 — Telegram bridge ✅ *(code-complete; awaiting smoke test)*

**What it is:** Telegram is pi's chat surface. You start a task by typing into the chat; pi runs in a fresh worktree per task, streams its work back as compact tool lines + assistant text, surfaces permission prompts as inline buttons, and finishes by pushing a branch and posting a draft-PR link.

**What landed (9 PRs, 2.1 → 2.9):**
- Pi RPC streaming + multi-turn `SendPrompt` (2.1)
- Telegram client + allowlist (2.2)
- Per-chat session + worktree lifecycle (2.3)
- Pi wired into the session, assistant text streamed (2.4)
- Compact tool-line rendering (2.5)
- Permission prompts via inline buttons (2.6)
- `request_ship` → `git push` + `gh pr create --draft` (2.7)
- `targets/*.yml` registry + `/repo` + `/cancel` (2.8)
- `.env.example` + `bin/preflight` + runbook (2.9)

**What the human does:** types tasks into the chat, taps approve/deny on permission prompts, reviews the draft PR.

**Open:** the actual end-to-end smoke test ("add a unit test for `format-money` with a negative input" → draft PR lands) is operator-driven; see `docs/running-the-bridge.md`. Polish ideas (`/recover` for orphan worktrees, etc.) are deferred to post-validation.

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
