<div align="center">

# Forge (swe-agent-factory)

### Production tells you what to build.<br>Agents build it. You just review.

<br>

[![Go 1.22](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](go.mod)&nbsp;
[![S1 live](https://img.shields.io/badge/status-S1%20live-brightgreen)](#where-things-stand)&nbsp;
[![Axiom](https://img.shields.io/badge/signals-Axiom-6366f1?logo=data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCI+PHBhdGggZmlsbD0id2hpdGUiIGQ9Ik0xMiAyTDIgN2wxMCA1IDEwLTV6TTIgMTdsIDEwIDUgMTAtNW0tMTAtNWwxMCA1IDEwLTUiLz48L3N2Zz4=)](#the-loop)&nbsp;
[![pi powered](https://img.shields.io/badge/agent-pi%20powered-0f172a)](#how-it-works)&nbsp;
[![PRs by agents](https://img.shields.io/badge/PRs-by%20agents-9ece6a)](#live-proof)

</div>

<img width="1184" height="689" alt="Screenshot 2026-05-22 at 5 20 08 PM" src="https://github.com/user-attachments/assets/f38a7d06-2221-418b-98aa-8f0a7d974f6c" />

<br>

Every codebase accumulates a backlog of things everyone knows should be fixed but nobody has time to fix. The factory clears that backlog. Not by generating slop. It actually reads the code, understands the conventions, runs the tests, and produces diffs a senior engineer would be proud to review.

One target repo today. The architecture is already multi-target. The signal sources are already pluggable. The agent runs in any codebase that has a `.swe-agent.yml`.

**You stop being an engineer on your own projects. You become the engineering manager.**

<br>

---

## The loop

It starts with an error in your production logs. A user typed something your app couldn't handle. That error flows into Axiom, the factory notices it, classifies it as a capability gap, and puts it in a queue.

Then the agent gets to work. It clones your repo and reads the codebase the way a new engineer onboards: finds the analogous pattern, understands the conventions, writes the implementation, adds the tests. When the full quality pipeline passes, it opens a branch and signals that it's done.

You get a pull request. You review. You merge. The gap is closed.

No tickets filed. No sprint planned. No one context-switched.

<br>

---

## Live proof

The first target is [`personal_finance`](https://github.com/danielreales00/personal_finance), a Telegram finance bot running in production. Real codebase: 35 features shipped, 572 tests, strict architecture rules enforced by a linter that fails the build on style violations.

These are the branches the factory has opened so far:

| Branch | What the agent did |
|---|---|
| `swe-agent/rename-account-action` | Added a new user command (dispatch, schema, executor, tests) after production showed users trying to rename accounts |
| `swe-agent/unify-format-money-fix-mocks` | Found a money-formatting helper duplicated across 9 files, unified them, and fixed a concurrency smell in the test mocks |
| `swe-agent/format-money-negative-test` | Noticed a missing edge case in the test suite and added it |
| `swe-agent/rename-account-agent-action` | Rewired the account-rename flow through the agent action pipeline |

No human wrote a line of any of these. Each one: agent reads, implements, quality gate passes, branch ready.

<br>

> **End-to-end integration test**
>
> `elapsed=49.2s &nbsp; tool_calls=8 &nbsp; tool_errors=0`
>
> `bin/quality: 572 tests, 0 failures. Clean tree.`
>
> Eight tool calls. Zero errors. Fifty seconds.

<br>

---

## What using it looks like today

You give it a task, it handles the rest:

```
> add :rename-account action

  Reading codebase...
  Found pattern in agent_actions.clj (line 47)
  Plan: add dispatch arm + schema + executor + 2 tests
  [approve? y]

  Implementing...
  Running bin/quality... 572 tests, 0 failures
  Branch: swe-agent/rename-account-action
  Done. Ready for review.
```

<br>

---

## What it becomes

### The Telegram bridge

The factory runs as a server. A gap appears in production and instead of silently queuing it, it asks:

```
forge

:rename-account not handled (7 affected users this week)
Evidence: "renombra mi cuenta a nequi" x3, "cambia el nombre" x4

  [Yes, implement]   [No, skip]   [Show me the gap]
```

You tap **Yes** from your phone. The agent implements it. When it's done:

```
swe-agent/rename-account-action ready

+87 lines  572 tests  0 failures  49s

  [View diff]   [Merge]   [Request changes]
```

You stay in the loop without being in the loop.

<br>

### The factory floor

`cmd/demo` ships a cosmetic TUI showing what the full factory dashboard looks like. Run it with `go run ./cmd/demo` to see the factory floor in your terminal.

```
╔══════════════════════════════════════════════════════════════════════════════════════════════════════════╗
║  ⚙  forge                                                   3 active · 12 queued · scan in 47m · S1   ║
╠══════════════════════════╦════════════════════════════════════════════════════╦══════════════════════════╣
║  SIGNALS                 ║  ACTIVE AGENTS                                     ║  REVIEW QUEUE            ║
╠══════════════════════════╬════════════════════════════════════════════════════╬══════════════════════════╣
║  ● capability gap        ║  personal_finance                                  ║  ✓ rename-account        ║
║    :rename-account       ║  › rename-account-action                           ║    +87 lines  0 fails    ║
║    7 users · 2d          ║  ██████████████░░░░░  impl  47 calls  2m 14s       ║    49s  [ Review ]       ║
║                          ║  ↳ Reading agent_actions.clj                       ║                          ║
║  ● capability gap        ║                                                    ║  ✓ unify-format-money    ║
║    :set-budget-note      ║  my_api                                            ║    +36 / -83  73s        ║
║    3 users · 6h          ║  › fix-null-ptr                                    ║    [ Review ]            ║
║                          ║  ████░░░░░░░░░░░░░░  draft  12 calls  38s          ║                          ║
║  ◑ LIN-4821              ║  ↳ Writing spec...                                 ║  ✓ fix-date-parsing      ║
║    null dereference      ║                                                    ║    +12 / -3  31s         ║
║    medium                ║  company_api                                       ║    Merged ✓              ║
║                          ║  › add-csv-export                                  ║                          ║
║  ○ "add CSV export"      ║  ██░░░░░░░░░░░░░░░░  impl   9 calls  21s           ║                          ║
║                          ║  ↳ Scanning export patterns...                     ║                          ║
║  [ + new task ]          ║                                                    ║                          ║
╠══════════════════════════╩════════════════════════════════════════════════════╩══════════════════════════╣
║  personal_finance   572 tests ✓   last gap 2h ago    4 PRs merged this week                            ║
║  my_api             214 tests ✓   last gap 14m ago   1 PR open · 0 failures                            ║
║  company_api         89 tests ✓   last gap 3d ago    12 PRs merged this month                          ║
╠══════════════════════════════════════════════════════════════════════════════════════════════════════════╣
║  ⚙ forge  ·  3 agents running  ·  12 queued  ·  next scan in 47m  ·  uptime 3h 12m                    ║
╚══════════════════════════════════════════════════════════════════════════════════════════════════════════╝
```

<br>

---

## Where things stand

| Stage | What it does | Status |
|---|---|---|
| **S1: Terminal** | Human gives agent a task; agent implements end-to-end | ✅ live |
| **S2: Telegram bridge** | Factory DMs you when a gap is found; you approve from your phone | 🔜 next |
| **S3: Autonomous cron** | Factory scans production logs on a schedule; high-confidence gaps auto-implement | 🔜 |
| **S4: Multi-source** | Linear tickets, GitHub issues, Slack messages all become work items | 🔜 |
| **S5: Multi-repo** | Any codebase opts in with a single YAML file; factory manages them all | 🔜 |
| **S6: Factory floor UI** | Web dashboard: signal feed, live agent sessions, review queue, repo health | 🔜 |

<br>

---

## Dive deeper

<table>
<tr>
<td width="50%" valign="top">

**[How it works](docs/how-it-works.md)**<br>
Two-layer architecture, the agent protocol, session audit

</td>
<td width="50%" valign="top">

**[The manifest](docs/manifest.md)**<br>
How a repo opts in, full `.swe-agent.yml` reference

</td>
</tr>
<tr>
<td width="50%" valign="top">

**[Roadmap](docs/roadmap.md)**<br>
S1 to S6 in detail, what is built and what is next

</td>
<td width="50%" valign="top">

**[Quickstart](docs/quickstart.md)**<br>
Build, scan, run your first task

</td>
</tr>
</table>
