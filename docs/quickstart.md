# Quickstart

## Prerequisites

- Go 1.22+
- [pi](https://github.com/earendil-works/pi-coding-agent) on your PATH
- An Anthropic API key (or any OpenAI-compatible provider)
- Node.js 18+ (for the TypeScript extensions)

## Build

```bash
go build ./cmd/swe-agent
```

## Scan a target

Collect work items from a target's signal sources and print a digest. No agent is spawned — this is read-only.

```bash
AXIOM_TOKEN=your-token ./swe-agent scan \
  --target personal_finance \
  --manifest-path /path/to/personal_finance/.swe-agent.yml \
  --days 7
```

Sample output:
```
──────────────────────────────────────────────────────────────────────
scan  target=personal_finance  window=2026-05-15T... → 2026-05-22T...
      items=3  new=2  known=1
──────────────────────────────────────────────────────────────────────

# axiom (3)
  · [capability_gap] :rename-account not handled
      id=axiom://capability_gap/rename-account  evidence=7
  · [capability_gap] :set-budget-note not handled
      id=axiom://capability_gap/set-budget-note  evidence=3
  ✓ [capability_gap] :tag-merchant not handled
      id=axiom://capability_gap/tag-merchant  (shipped)
```

## Run an inline task

Skip signal collection and give the agent a task directly:

```bash
./swe-agent scan \
  --target personal_finance \
  --manifest-path /path/to/personal_finance/.swe-agent.yml \
  --task "add :set-budget-note action"
```

## Drive the agent directly (spike / debug)

Bypass the factory and talk to pi directly. Useful for testing new repos or debugging the agent loop:

```bash
go run ./cmd/spike \
  --cwd /path/to/personal_finance \
  --provider anthropic \
  --model claude-sonnet-4-5 \
  --prompt "Add the :rename-account action following the pattern in agent_actions.clj"
```

## Run the S1 workflow in a target repo

If you want to run the agent interactively in the target repo (S1 mode, without the Go factory):

```bash
cd /path/to/personal_finance
pi --prompt-file .pi/prompts/work.md "add :rename-account action"
```

The agent will read the codebase, show you its plan, wait for approval, implement, run `bin/quality`, and call `request_ship` when done.

## Wiring a new target repo

1. Create `targets/myrepo.yml`:
   ```yaml
   repo: https://github.com/you/myrepo.git
   manifest_path: .swe-agent.yml
   ```

2. Commit `.swe-agent.yml` in the target repo (see [manifest reference](manifest.md))

3. Commit `.pi/` in the target repo — copy from `personal_finance/.pi/` as a starting point

4. Scan: `./swe-agent scan --target myrepo --manifest-path /path/to/myrepo/.swe-agent.yml`
