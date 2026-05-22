# Status — SHELVED (active work moved elsewhere)

**Shelved:** 2026-05-21
**Active location:** `~/personal/personal_finance/.pi/` (Increment 1, scenario S1)

## What's here

Phase-1 scaffold for a multi-target SWE Agent Factory:

- `internal/workitem/` — WorkItem model with stable IDs
- `internal/signal/{axiom,inline}/` — pluggable signal sources
- `internal/state/filestore.go` — JSON-backed dedup store
- `internal/pipeline/run.go` — Scan→Triage→Stage skeleton
- `internal/agent/pi.go` — Go RPC client for `pi --mode rpc` (validated by spike)
- `internal/config/target.go` — `.swe-agent.yml` parser
- `extensions/{permission-gate,run-bin-quality}.ts` — pi extensions (now superseded by `personal_finance/.pi/extensions/`)
- `cmd/spike/main.go` — throwaway driver that proved end-to-end pi RPC works
- `state/spike/20260521T192233Z/*.jsonl` — captured spike session (audit)

The spike succeeded on first try: 8 tool calls, 0 errors, ~50s wall clock, clean tree.
See `/tmp/pi-research-report.md` for the deep architecture audit that led to this layout.

## Why shelved

After the spike, the architecture pivoted: pi is a *harness*, not a thing to wrap.
The MVP (S1, terminal-driven) doesn't need a Go orchestrator — pi runs in the target
repo directly, with prompts/extensions/safety committed under the target's `.pi/`.

Building a Phase-3 product (multi-target, multi-source, draft-then-impl stages) before
the Phase-1 product (one repo, human-triggered) is exactly the kind of overdesign
CLAUDE.md warns against. We backed off.

## Will be revived for

**S2 (Telegram bridge):** server-hosted worker that spawns `pi --mode rpc` per task,
routes `extension_ui_request{method:"input"}` through the personal_finance bot's
Telegram DM, captures `request_ship` events, opens PRs.

When S2 starts, this dir comes back largely unchanged:

- `internal/agent/pi.go` — the worker's heart
- `internal/workitem/`, `internal/signal/`, `internal/state/`, `internal/pipeline/` — the orchestrator's plumbing
- `extensions/permission-gate.ts` — universal safety (now in `personal_finance/.pi/`; copy/sync at S2)

**S3 (Axiom cron + draft-then-impl):** uses everything above plus a scheduler entrypoint.

**S4 (GitHub issue triggered):** uses everything above plus a webhook handler.

## Do not delete

Cannibalizing the working code here costs more than keeping it. The scaffold is
the design document we'll need when revival starts; deleting it means re-deriving
the same shape in three months.

For the full plan, see the conversation log on 2026-05-21 (search "S1 → S4").
