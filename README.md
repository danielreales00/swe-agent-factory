# swe-agent-factory

> **Production tells you what to build. Agents build it. You just review.**

Every codebase accumulates a backlog of things everyone knows should be fixed but nobody has time to fix. The factory clears that backlog. Not by generating slop — by actually reading the code, understanding the conventions, running the tests, and producing diffs a senior engineer would be proud to review.

One target repo today. The architecture is already multi-target. The signal sources are already pluggable. The agent runs in any codebase that has a `.swe-agent.yml`.

You stop being an engineer on your own projects. You become the engineering manager.

---

A software factory where AI agents are the engineering workforce. A gap appears in your production logs. A ticket lands in your issue tracker. A user complains on Slack. The factory turns that signal into a pull request — reading your codebase, following your conventions, running your quality gates — and puts it in your review queue. You approve or reject. That's your entire job.

This isn't a code-autocomplete tool. It's an autonomous engineering loop.

---

## The loop

```
Production                Factory                    Agent                     You
─────────────────────────────────────────────────────────────────────────────────────

User types something   →  Axiom sees               →  Reads the codebase    →  PR lands
the bot can't handle      error: outcome=error         Follows CLAUDE.md        in your
                          action=:rename-account       Writes the impl          queue
                          (capability gap)             Runs bin/quality
                          WorkItem created             572 tests pass
                                                       Calls request_ship
                                                       Branch: swe-agent/
                                                       rename-account-action
```

The agent doesn't ask for help. It reads your code the way a senior engineer onboards — finds analogous patterns, follows the conventions, tests the edge cases. You see a clean diff when it's done.

---

## Live right now: personal_finance

The only target wired up today is [`personal_finance`](https://github.com/danielreales00/personal_finance) — a Telegram-based finance bot running in production on fly.io. It's a real codebase: 35 shipped features, 572 tests, strict hexagonal architecture, a linter suite that fails the build on style violations.

Here's what the factory has shipped so far:

| Branch | What the agent did | Lines changed |
|---|---|---|
| `swe-agent/rename-account-action` | Added `:rename-account` dispatch arm, schema, executor, and tests to `agent_actions.clj` | +87 |
| `swe-agent/unify-format-money-fix-mocks` | Unified duplicated `format-money` helpers across 9 files; fixed atom-while-mutating smell in mocks | −47 net |
| `swe-agent/format-money-negative-test` | Added missing negative-amount edge case to formatting test suite | +9 |
| `swe-agent/rename-account-agent-action` | Rewired account-rename through the agent action pipeline | +52 |

Each one: agent reads codebase → implements → `bin/quality` passes → `request_ship` fires → branch ready for review. No human wrote a line.

**The spike that validated the plumbing:**
```
go run ./cmd/spike \
  --cwd /path/to/personal_finance \
  --provider anthropic --model claude-sonnet-4-5 \
  --prompt "Open agent_actions.clj and tell me which actions are dispatched."

elapsed=49.2s  tool_calls=8  tool_errors=0  stop="end_turn"
bin/quality: 572 tests, 0 failures. Clean tree.
```

Eight tool calls. Zero errors. Fifty seconds. The Go orchestrator drove `pi --mode rpc` over a JSONL pipe and the agent ran inside the live codebase without any human in the loop until the answer came back.

---

## How it works

Two layers. The factory is the outer orchestrator. `pi` is the inner agent.

```
┌─────────────────────────────────────────────────────────────────────┐
│  swe-agent-factory  (Go)                                            │
│                                                                     │
│  Signals ──→ WorkItems ──→ Triage ──→ Pipeline stages              │
│  (Axiom /                  (what      (draft / file /              │
│   Linear /                  stages     impl / ship)                │
│   GitHub /                  does this                              │
│   inline)                   type need?)                            │
│                                          │                         │
│                                    spawn pi --mode rpc             │
│                                    over JSONL stdin/stdout         │
└─────────────────────────────────────────┬───────────────────────────┘
                                          │
                    ┌─────────────────────▼──────────────────────────┐
                    │  pi  (inner agent loop)                         │
                    │                                                  │
                    │  Prompt ──→ read/edit/bash tools                │
                    │             permission-gate extension           │
                    │             run_quality extension               │
                    │             request_ship extension              │
                    │                        │                        │
                    │              passes quality gate               │
                    │              calls request_ship                │
                    │              branch: swe-agent/<slug>          │
                    └──────────────────────────────────────────────── ┘
                                          │
                                          ▼
                              Factory reads ship-request
                              from session JSONL → opens PR
```

The factory drives `pi` headlessly over a JSONL RPC protocol. It sends a prompt, reads events (tool calls, messages, errors), handles UI requests from extensions with conservative defaults (no auto-confirm for risky prompts), and returns a structured `RunResult` when `agent_end` fires. One `Pi` instance per work item. Sessions are written to disk for full audit and replay.

The inner agent never knows it's being driven programmatically. It uses the same tools, the same extensions, the same quality gate as a human running `pi` in a terminal.

---

## The contract: `.swe-agent.yml`

Any repo opts into the factory by committing one YAML file. This is the full contract — project context, quality gate, signal sources, source layout, branching convention.

```yaml
# .swe-agent.yml
version: 1

project:
  name: personal_finance
  language: clojure
  description: |
    Telegram-based finance bot. LLM emits Clojure via SCI sandbox;
    writes flow through (propose ...) and a typed executor.

conventions:
  voice_file: CLAUDE.md        # project conventions; agent reads this first

build:
  fmt_cmd:     bin/fmt
  quality_cmd: bin/quality     # blocking: fmt + kondo + splint + arch-check + tests
  pre_commit:  [bin/fmt, bin/quality]

source_layout:
  executor_dispatch: src/finance/app/agent_actions.clj
  executor_schemas:  src/finance/app/agent_actions.clj
  prompt_vocabulary: src/finance/app/advise.clj
  port_layer:        src/finance/ports/
  store_postgres:    src/finance/adapters/

signals:
  - kind: axiom
    name: capability-gaps
    dataset: finance-prod
    query: |
      ['finance-prod']
      | extend p = parse_json(['message'])
      | where tostring(p['event']) == 'agent-action'
      | where tostring(p['outcome']) == 'error'
      | project _time, p | sort by _time desc
    cluster_by: action
    triage_as: capability_gap   # → draft + file + impl + ship

git:
  default_branch: main
  branch_prefix: swe-agent/

approvers: [danielreales00]

testing:
  coverage_threshold: 70
  mocks_file: test/finance/support/mocks.clj
```

That's it. The factory reads this at scan time, knows which Axiom dataset to query, how to run the quality gate, where the relevant source files are, and whose approval is needed before merge. No factory-side configuration per repo. The repo describes itself.

---

## Current state: S1 — terminal-driven

What works right now:

```bash
# Human-triggered: give the agent a task
pi --prompt-file .pi/prompts/work.md "add :rename-account action"

# The agent:
# 1. Reads CLAUDE.md and the system prompt
# 2. Reads agent_actions.clj to find analogous patterns
# 3. Calls propose_plan — plan-gate extension blocks edits until plan is shown
# 4. Implements the action, schema, executor, tests
# 5. Runs bin/quality (572 tests, 0 failures)
# 6. Creates branch swe-agent/rename-account-action
# 7. Calls request_ship → entry appended to session JSONL
# 8. Done. Branch is ready for review.
```

The Go factory can also drive this headlessly:

```bash
./swe-agent scan \
  --target personal_finance \
  --manifest-path /path/to/personal_finance/.swe-agent.yml \
  --days 7

# ──────────────────────────────────────────────────────────────────────
# scan  target=personal_finance  window=2026-05-15T... → 2026-05-22T...
#       items=3  new=2  known=1
# ──────────────────────────────────────────────────────────────────────
#
# # axiom (3)
#   · [capability_gap] :rename-account not handled
#       id=axiom://capability_gap/rename-account  evidence=7
#   · [capability_gap] :set-budget-note not handled
#       id=axiom://capability_gap/set-budget-note  evidence=3
#   ✓ [capability_gap] :tag-merchant not handled
#       id=axiom://capability_gap/tag-merchant  (shipped)
```

The factory knows which items are new and which are already shipped. Dedup is stable — the same gap from the same source always gets the same ID, so the same problem never gets double-implemented.

---

## The vision: S2 → S∞

### S2 — Telegram bridge

The factory runs as a server. When a new capability gap lands, instead of spawning the agent immediately, it sends a Telegram DM:

```
🏭 Factory

:rename-account not handled (3 users affected in the last 7 days)
Evidence: "renombra mi cuenta bancolombia a nequi" (×3), "cambia el nombre" (×4)

Should I implement it?

  [Yes, implement]   [No, skip]   [Show me the gap]
```

Tap **Yes**. The factory spawns `pi --mode rpc`, routes any UI confirmation requests through the DM, and when `request_ship` fires:

```
✅ swe-agent/rename-account-action ready for review
+87 lines  572 tests  0 failures  49s

  [View diff]  [Merge]  [Request changes]
```

You stay in the loop without being in the loop.

### S3 — Autonomous cron

The factory runs on a schedule. Every hour it queries Axiom for new gaps, deduplicates against the work-item store, triages, and enqueues. High-confidence items (clear error pattern, analogous implementation exists) get implemented automatically. Ambiguous items get a Telegram nudge first.

The `triage_as` field in `.swe-agent.yml` controls this:
- `capability_gap` → `draft + file + impl + ship` (agent writes a spec before coding)
- `bug_report` → `impl + ship` (agent goes straight to code)
- `inline_task` → `impl + ship` (operator-initiated, fully trusted)

### S4 — Multi-source, multi-repo

Linear tickets, GitHub issues, Slack messages with a `:robot:` reaction — anything can become a WorkItem. Every signal source implements the same two-method interface, and every repo opts in with one YAML file. The factory is agnostic about both.

```
targets/
  personal_finance.yml   → github.com/me/personal_finance
  my_api.yml             → github.com/me/my_api
  company_monorepo.yml   → github.com/org/monorepo

Each repo has its .swe-agent.yml that declares its own signals, build
commands, source layout, and approval requirements. The factory manages
all of them from a single process with a shared work-item store.
```

### S5 — The factory floor

This is the endgame. A dashboard where every agent, every work item, every repo is visible at once.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│  🏭  Software Factory                              3 active · 12 queued      │
├──────────────────┬───────────────────────────────────────────────────────────┤
│                  │                                                            │
│   Signal feed    │   Active agents                                            │
│   ────────────   │   ─────────────────────────────────────────────────────   │
│                  │                                                            │
│   🔴 gap         │   personal_finance / rename-account                       │
│      :set-note   │   ████████████░░░░  impl · 47 tool calls · 2m 14s        │
│      7 users     │   Reading agent_actions.clj...                            │
│                  │   [ view session ]  [ abort ]                             │
│   🟡 ticket      │                                                            │
│      LIN-4821    │   my_api / fix-null-ptr                                   │
│      medium      │   ███░░░░░░░░░░░░░  draft · 12 tool calls · 38s          │
│                  │   Writing spec...                                          │
│   🔵 inline      │   [ view session ]  [ abort ]                             │
│      "add CSV    │                                                            │
│       export"    │   Review queue                                             │
│                  │   ─────────────────────────────────────────────────────   │
│   [ new task ]   │   ✅ rename-account        +87 / −0   49s   [ Review ]   │
│                  │   ✅ unify-format-money     +36 / −83  73s   [ Review ]   │
│                  │   ✅ fix-date-parsing       +12 / −3   31s   [ Merged ✓ ] │
│                  │                                                            │
├──────────────────┴───────────────────────────────────────────────────────────┤
│  personal_finance  572 tests ✓  last gap 2h ago  4 PRs merged this week     │
│  my_api            214 tests ✓  last gap 14m ago  1 PR open · 0 failures    │
└──────────────────────────────────────────────────────────────────────────────┘
```

**The agent session view.** Click any active agent and watch it work in real time. Tool calls streaming in. The bash output from `bin/quality`. The moment it writes the test. The moment it calls `request_ship`. Every decision logged to JSONL — you can replay any session and see exactly why the agent made each choice.

**The signal timeline.** A scrollable feed of every incoming signal across all repos, with the factory's triage decision visible. Override any item: demote a `capability_gap` to `closed`, or promote an `inline_task` to urgent. The factory is a suggestion engine; you control what ships.

**The health board.** Per-repo metrics over time — test count trend, mean time from signal to merged PR, agent error rate, coverage percentage. You see the health of your entire software portfolio at a glance.

**The diff viewer.** Every PR the factory opens shows inline diffs with the agent's reasoning in the margin. "Why did the agent add this test?" — click in, read the chain of thought, see which analogous function it modeled it on.

---

## Architecture (full vision)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  Factory floor UI  (web / Telegram)                      │
│   signal feed · live agent sessions · review queue · repo health        │
└──────────────────────────────┬──────────────────────────────────────────┘
                               │ REST / WebSocket / Telegram Bot API
┌──────────────────────────────▼──────────────────────────────────────────┐
│                     swe-agent-factory  (Go server)                       │
│                                                                          │
│  ┌───────────┐  ┌─────────┐  ┌──────────────────┐  ┌────────────────┐  │
│  │  Signals  │  │  State  │  │    Pipeline       │  │  Agent pool    │  │
│  │           │→ │  Store  │→ │                   │→ │                │  │
│  │  axiom    │  │  (JSON) │  │  draft            │  │  4 concurrent  │  │
│  │  linear   │  └─────────┘  │  file             │  │  pi --mode rpc │  │
│  │  github   │               │  impl             │  │  one per item  │  │
│  │  slack    │               │  ship             │  │                │  │
│  │  inline   │               └──────────────────┘  └────────────────┘  │
│  └───────────┘                                                           │
└──────────────────────────────────────────────────────────────────────────┘
                               │ JSONL over stdin/stdout (pi RPC protocol)
┌──────────────────────────────▼──────────────────────────────────────────┐
│              pi  (inner agent, one instance per work item)               │
│                                                                          │
│  reads target repo · follows .swe-agent.yml conventions                 │
│  runs quality_cmd · calls request_ship · writes session JSONL           │
└──────────────────────────────┬──────────────────────────────────────────┘
                               │ git checkout, read, write, bash
┌──────────────────────────────▼──────────────────────────────────────────┐
│              Target repos  (each has .swe-agent.yml)                     │
│   personal_finance · my_api · company_monorepo · ...                    │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## What's built today

| Component | Status |
|---|---|
| `internal/workitem` — WorkItem model, stable IDs, triage logic | ✅ |
| `internal/signal/axiom` — pulls clustered errors from Axiom | ✅ |
| `internal/signal/inline` — `--task` flag → ad-hoc WorkItem | ✅ |
| `internal/state/filestore` — JSON-backed dedup store | ✅ |
| `internal/pipeline/scan` — collect → dedup → digest | ✅ |
| `internal/agent/pi.go` — Go RPC client for `pi --mode rpc` | ✅ |
| `internal/config/target.go` — `.swe-agent.yml` parser | ✅ |
| `extensions/permission-gate.ts` — headless safety layer | ✅ |
| `extensions/run-bin-quality.ts` — quality gate extension | ✅ |
| `cmd/spike` — end-to-end integration test (proved the loop works) | ✅ |
| `cmd/swe-agent scan` — scan a target, print work-item digest | ✅ |
| `personal_finance/.pi/` — full S1 workflow on a live production target | ✅ |
| S2: Telegram bridge (server + async human-in-the-loop) | 🔜 |
| S3: Autonomous cron + draft stage | 🔜 |
| S4: Linear / GitHub / Slack sources | 🔜 |
| S5: Multi-repo concurrent agent pool | 🔜 |
| S6: Factory floor UI | 🔜 |

---

## Try it

```bash
# Build
go build ./cmd/swe-agent

# Scan a target (needs AXIOM_TOKEN and a local checkout)
AXIOM_TOKEN=... ./swe-agent scan \
  --target personal_finance \
  --manifest-path /path/to/personal_finance/.swe-agent.yml \
  --days 7

# Inline task (no Axiom needed)
./swe-agent scan \
  --target personal_finance \
  --manifest-path /path/to/personal_finance/.swe-agent.yml \
  --task "add :set-budget-note action"

# Drive the agent directly (spike / debug mode)
go run ./cmd/spike \
  --cwd /path/to/personal_finance \
  --provider anthropic \
  --model claude-sonnet-4-5 \
  --prompt "Add the :rename-account action following the pattern in agent_actions.clj"
```

