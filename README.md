<div align="center">

# 🏭 swe-agent-factory

<p>
  <strong>Production tells you what to build.</strong><br>
  Agents build it. You just review.
</p>

<br>

[![Go 1.22](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go&logoColor=white)](go.mod)&nbsp;
[![S1 live](https://img.shields.io/badge/S1-live-brightgreen)](#where-things-stand)&nbsp;
[![target: personal_finance](https://img.shields.io/badge/target-personal__finance-blue)](https://github.com/danielreales00/personal_finance)

</div>

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
  Running bin/quality... 572 tests, 0 failures ✓
  Branch: swe-agent/rename-account-action
  Done. Ready for review.
```

<br>

---

## What it becomes

### The Telegram bridge

The factory runs as a server. A gap appears in production and instead of silently queuing it, it asks:

```
🏭 Factory

:rename-account not handled (7 affected users this week)
Evidence: "renombra mi cuenta a nequi" x3, "cambia el nombre" x4

  [Yes, implement]   [No, skip]   [Show me the gap]
```

You tap **Yes** from your phone. The agent implements it. When it's done:

```
✅ swe-agent/rename-account-action ready

+87 lines · 572 tests · 0 failures · 49s

  [View diff]   [Merge]   [Request changes]
```

You stay in the loop without being in the loop.

<br>

### The factory floor

When you're managing multiple repos, every agent, every work item, every repo visible in one place:

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
│   [ new task ]   │   ✅ rename-account        +87 / -0   49s   [ Review ]   │
│                  │   ✅ unify-format-money     +36 / -83  73s   [ Review ]   │
│                  │   ✅ fix-date-parsing       +12 / -3   31s   [ Merged ✓ ] │
│                  │                                                            │
├──────────────────┴───────────────────────────────────────────────────────────┤
│  personal_finance  572 tests ✓  last gap 2h ago  4 PRs merged this week     │
│  my_api            214 tests ✓  last gap 14m ago  1 PR open · 0 failures    │
└──────────────────────────────────────────────────────────────────────────────┘
```

Click any active agent and watch it work in real time: tool calls streaming in, the moment it writes the test, the moment it calls `request_ship`. Every session is logged and replayable. Every triage decision is overridable. The factory is a suggestion engine; you control what ships.

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
S1 to S6 in detail, what's built and what's next

</td>
<td width="50%" valign="top">

**[Quickstart](docs/quickstart.md)**<br>
Build, scan, run your first task

</td>
</tr>
</table>
