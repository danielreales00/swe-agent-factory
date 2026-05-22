# How it works

The factory has two layers. Understanding the boundary between them explains everything else.

## The outer layer: swe-agent-factory (Go)

The factory is the orchestrator. It watches for signals, decides what work needs doing, and hands each task to an agent. It doesn't write any code itself.

```
Signals ──→ WorkItems ──→ Triage ──→ Pipeline stages ──→ Agent
(Axiom /    (what's       (what        (draft / file /     (pi)
 Linear /    broken or     stages       impl / ship)
 GitHub /    missing)      does this
 inline)                   type need?)
```

Signal sources are pluggable — any source that can produce a list of work items implements the same two-method interface. Today: Axiom production logs and inline CLI tasks. Next: Linear, GitHub issues, Slack.

WorkItems have stable IDs derived from their source and key, so the same gap from the same source never gets double-implemented. The state store tracks which items are open, in-progress, or shipped.

## The inner layer: pi (the agent)

`pi` is the coding agent. The factory spawns it as a subprocess for each work item and communicates over a JSONL protocol on stdin/stdout — one JSON object per line. The factory sends a prompt; pi sends back a stream of events (tool calls, messages, errors); the factory reads until `agent_end`.

```
Factory                              pi
──────────────────────────────────────────────────────────────
{"type":"prompt","message":"..."}  →
                                   ← {"type":"tool_execution_end",...}
                                   ← {"type":"tool_execution_end",...}
                                   ← {"type":"message_end",...}
                                   ← {"type":"agent_end","messages":[...]}
```

The agent never knows it's being driven programmatically. It uses the same tools (read, write, edit, bash), the same extensions (permission-gate, run_quality, request_ship), and the same quality gate as a human running `pi` interactively in a terminal. One `Pi` instance per work item. Sessions are written to JSONL files for full audit and replay.

## Extensions

Three extensions run inside every agent session:

**`permission-gate`** — blocks destructive shell patterns and prevents writes to protected paths (`.env`, credentials, anything outside the repo root). Fully headless — it blocks and returns a reason; the agent receives the block and can take a different approach.

**`run_quality`** — exposes the target repo's `quality_cmd` as a tool the agent can call. The agent calls this before declaring done. If it fails, the agent reads the output and fixes the issue.

**`request_ship`** — the agent's "I'm done" signal. Validates branch name, PR title, and PR body (which must include a `## Decisions I made` section explaining every unilateral choice). On success, appends a `swe-agent.ship-request` entry to the session JSONL. The factory reads this entry after `agent_end` to know where to open the PR.

## Session audit

Every pi session writes a `.jsonl` file — one event per line, timestamped. The factory stores these under `state/<target>/sessions/<work-item-id>/`. You can replay any session, audit every tool call, and see exactly why the agent made each decision.

```
state/
  personal_finance/
    workitems/
      axiom__capability_gap__rename-account.json
    sessions/
      axiom__capability_gap__rename-account/
        2026-05-21T19-22-33Z_<uuid>.jsonl
```
