# The manifest: `.swe-agent.yml`

A repo opts into the factory by committing a single YAML file at its root. The factory reads this at scan time — no factory-side configuration per repo. The repo describes itself.

## Full example (personal_finance)

```yaml
version: 1

project:
  name: personal_finance
  language: clojure
  description: |
    Telegram-based finance bot. LLM emits Clojure via SCI sandbox;
    writes flow through (propose ...) and a typed executor.

conventions:
  voice_file: CLAUDE.md   # agent reads this before touching any code

build:
  fmt_cmd:         bin/fmt
  quality_cmd:     bin/quality   # fmt + lint + arch-check + all tests
  integration_cmd: bin/kaocha :integration
  pre_commit:      [bin/fmt, bin/quality]

source_layout:
  # Edit anchors the agent uses to find the right files fast
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
    cluster_by: action        # group errors by the action keyword
    triage_as: capability_gap # → draft + file + impl + ship stages

  # Reserved — uncomment when ready:
  # - kind: linear
  #   team: ENG
  #   label: swe-agent
  #   triage_as: feature_request   # → impl + ship only

git:
  default_branch: main
  branch_prefix: swe-agent/
  commit_signing: false

approvers: [danielreales00]

testing:
  unit_only:          bin/kaocha :unit
  coverage_threshold: 70
  mocks_file:         test/finance/support/mocks.clj
```

## Field reference

| Field | What it does |
|---|---|
| `conventions.voice_file` | The agent reads this file before touching any code — your house style, idioms, and hard rules |
| `build.quality_cmd` | The blocking gate. Agent calls this before `request_ship`. Must exit 0 for the PR to open. |
| `source_layout` | Named shortcuts to important files. Agent uses these as starting points instead of searching from scratch. |
| `signals[].triage_as` | Controls which pipeline stages run. `capability_gap` gets a spec drafted first; `bug_report` and `feature_request` go straight to implementation. |
| `signals[].cluster_by` | For Axiom signals: which field to group errors by when creating work items. Prevents one noisy error from flooding the queue. |
| `approvers` | GitHub usernames who can merge agent PRs. Used in future PR automation. |

## Adding a new repo

1. Create `targets/myrepo.yml` pointing at the git URL
2. Commit `.swe-agent.yml` in the target repo
3. Commit `.pi/` (extensions + prompts + system-prompt) in the target repo
4. Run `./swe-agent scan --target myrepo`
