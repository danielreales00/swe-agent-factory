# swe-agent-factory

Multi-tenant engineer-as-a-service. Reads `.swe-agent.yml` from a target repo,
collects WorkItems from pluggable signal sources (production logs, Linear
tickets, GitHub issues, inline CLI tasks), and routes each through the
pipeline stages it needs (draft → file → impl → ship).

Phase 1 (this scaffold) ships:

- `axiom` source — pulls clustered `:agent-action` errors from a target's
  Axiom dataset (port of `audit.clj/post-apl`).
- `inline` source — `--task "..."` flag → ad-hoc WorkItem.
- `scan` stage — dedup against the state store, print a digest.

Phases 2+ add the rest of the sources (linear, github, manifest) and the
draft/file/impl/ship stages. See `~/.claude/plans/glittery-chasing-hoare.md`
in the operator's home for the full plan.

## Run

```bash
go build ./cmd/swe-agent

AXIOM_TOKEN=...              # required for axiom source
AXIOM_DATASET=finance-prod   # optional override; target manifest wins

./swe-agent scan --target personal_finance --days 30
./swe-agent scan --target personal_finance --task "add :create-account action"
```

## Targets

`targets/<name>.yml` points at a target repo's git URL and manifest path:

```yaml
repo:          https://github.com/danielreales00/personal_finance.git
manifest_path: .swe-agent.yml
```

At scan time the factory clones the repo into a temp dir, reads its
`.swe-agent.yml`, and uses that as the source-of-truth for what signals to
collect and how to operate on the codebase.

## State

`state/<target>/workitems/<id>.json` — one file per known WorkItem. The
filename's ID is stable (`source://kind/key`) so the same gap from the
same source doesn't get re-ticketed.

## Not LiteLLM

CVE-2026-33634 (March supply chain) + CVE-2026-42208 (April SQLi RCE) ruled
it out. Each provider is a thin wrapper over its official Go SDK behind a
`Provider` interface. LLM wiring lands in Phase 2 with the `draft` stage.
