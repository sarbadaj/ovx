# OVX

**Open, provider-independent coding agent for repository-aware software engineering.**

OVX is an Ompirit Software project for building a Codex-like developer experience without depending on a single proprietary model provider. OVX is designed to understand repositories, plan changes, use validated tools, edit code, execute tests inside isolated environments, present diffs, and require human approval before sensitive actions.

> **Project status:** Product architecture is defined. Core execution, security, recovery, data-model, and evaluation specifications are ready for MVP implementation.

## Core architecture rule

```text
Model        = proposes
Rails Agent  = orchestrates
Go Runtime   = validates and executes
Sandbox      = contains
PostgreSQL   = remembers
Human        = approves risky actions
Tests        = determine completion
```

## What OVX should do

A developer should be able to open a repository and ask OVX to perform a real engineering task, for example:

> Fix the failing specs in `UsersController`. Do not modify the public API. Run the relevant tests and show me the diff before committing.

OVX should:

1. create an isolated task workspace;
2. inspect the repository and retrieve only relevant context;
3. create a concise plan;
4. ask a model to propose the next action;
5. validate that action against OVX policy;
6. execute approved tools through the runtime service;
7. run targeted tests and quality checks;
8. stop for approval before risky or external operations;
9. show changed files, patches, test results, and remaining risks;
10. allow accept, reject, retry, resume, or manual edits;
11. optionally create a commit or pull request after approval.

## Architecture

```text
┌─────────────────────────────────────────────┐
│        OVX Desktop UI / Future CLI          │
│       Tauri + React + TypeScript            │
└──────────────────────┬──────────────────────┘
                       │
                       ▼
┌─────────────────────────────────────────────┐
│            Rails Control Plane              │
│ Tasks • Agent State • Context • Approvals   │
│ Events • Policies • Repository Metadata     │
└───────────────┬───────────────────┬─────────┘
                │                   │
                │ model requests    │ validated tool requests
                ▼                   ▼
┌──────────────────────────┐  ┌──────────────────────────────┐
│      Model Gateway       │  │      OVX Runtime Service     │
│ llama.cpp • vLLM •      │  │             Go               │
│ compatible providers     │  │ validation • limits • I/O   │
└──────────────────────────┘  └───────────────┬──────────────┘
                                              │
                                              ▼
                                  ┌───────────────────────────┐
                                  │      Docker Sandbox       │
                                  │ Repository • Tests • Git  │
                                  │ Shell • Build tools       │
                                  └───────────────────────────┘
```

The model never receives unrestricted host access. It proposes structured actions. Rails orchestrates the task and applies policy. The Go runtime validates execution constraints. The sandbox contains untrusted repository execution.

## MVP goals

- Open a local repository
- Create an isolated workspace
- Chat with repository context
- Browse, search, and read files
- Apply structured patches
- Run commands and tests in a sandbox
- Display Git status and diffs
- Plan and execute multi-step coding tasks
- Persist task state and tool execution
- Resume interrupted tasks safely
- Require approval for sensitive actions
- Use local or self-hosted coding models
- Persist audit events and outcomes

### Initial tool set

```text
list_files
read_file
search_code
apply_patch
run_command
run_tests
git_status
git_diff
git_log
```

## Technology stack

| Layer | Technology | Purpose |
|---|---|---|
| Desktop | Tauri + React + TypeScript | Cross-platform developer UI |
| Control plane | Ruby on Rails | Agent orchestration, API, approvals, persistence |
| Background jobs | Sidekiq + Redis | Indexing and long-running work |
| Database | PostgreSQL | Tasks, events, repository metadata, audit data |
| Agent engine | Explicit state machine | Predictable resumable execution |
| Runtime | Go | Safe tool execution and process control |
| Sandbox | Docker initially | Repository isolation and resource limits |
| Search | ripgrep + Tree-sitter | Lexical and symbol-aware retrieval |
| Local inference | llama.cpp | Local development and Apple Silicon |
| GPU inference | vLLM | Higher-throughput self-hosted inference |
| Model integration | ModelAdapter | Provider independence |

## Durable execution

Tool lifecycle:

```text
requested
authorized
started
completed
failed
cancelled
unknown
```

Non-idempotent actions such as `git push`, pull-request creation, deployment, or infrastructure changes must never be blindly replayed after an ambiguous crash.

## Security model

Generated code and commands are untrusted.

- models never receive unrestricted host access;
- side effects happen through validated tools;
- execution is sandboxed;
- CPU, memory, PIDs, disk, and time are limited;
- network is denied by default;
- secrets are not automatically mounted;
- unsafe path traversal and symlink escapes are rejected;
- risky capabilities require approval;
- tool activity is auditable.

### Capabilities

```text
filesystem.read
filesystem.write
filesystem.delete
process.execute
network.connect
git.read
git.write
git.remote
secrets.read
deployment.execute
```

## Repository intelligence

OVX combines:

- file tree;
- `.gitignore` filtering;
- language/framework detection;
- `ripgrep`;
- Tree-sitter symbols;
- dependency/configuration metadata;
- test-to-source relationships;
- recently changed files;
- Git history when relevant;
- task-specific relevance scoring;
- optional embeddings only where they add measurable value.

Each context item stores provenance: path, line range, retrieval method, reason, and relevance score.

## Rails-first specialization

Planned Rails-aware tools:

```text
find_route
find_controller_action
find_model
find_association
find_callback
find_job
find_policy
find_serializer
find_spec
detect_n_plus_one
run_rspec
run_rubocop
```

OVX should build a lightweight Rails relationship graph:

```text
Route → Controller#action → Service/Policy → Model → Job/Serializer
Specs ───────────────────────────────────────► related nodes
```

## Context budgeting

Model context is finite. OVX budgets it across:

```text
system instructions
conversation/task
task state/plan
repository evidence
tool observations
output reserve
```

## Planned structure

```text
ovx/
├── apps/
│   ├── desktop/
│   └── api/
├── services/
│   └── runtime/
├── packages/
│   ├── tool-schema/
│   └── shared-types/
├── evals/
│   └── rails/
├── docs/
├── docker/
├── scripts/
├── .ovx/
├── README.md
├── CONTRIBUTING.md
├── SECURITY.md
└── LICENSE
```

## MVP roadmap

### 0.0.1 — Vertical slice
Prove one end-to-end Rails fix: repository → context → plan → patch → RSpec → diff.

### 0.0.2 — Durable execution
Add explicit state machine, tool persistence, retries, cancellation, crash recovery, idempotency, and ambiguous-state handling.

### 0.0.3 — Security and approvals
Add capability policy, network controls, filesystem scopes, secret isolation, approval flow, and audit trail.

### 0.1 — Rails intelligence
Add Rails graph, specialized tools, RSpec/RuboCop integration, and evaluation suite.

### 0.2 — Desktop
Add Tauri/React task view, tool timeline, approvals, diff viewer, test results, retry/cancel/resume.

### 0.3 — Collaboration
Add GitHub/GitLab and pull-request workflows.

## Evaluation

Measure:

```text
task success rate
test pass rate
regression rate
unsafe-action attempts
patch size
tool-call count
retry count
execution time
context usage
recovery success
```

## Development principles

1. Reliability before autonomy
2. Local-first
3. Provider-independent
4. Tool-driven
5. Capability-based security
6. Human approval for risk
7. Repository-aware
8. Tests determine completion
9. Observable by design
10. Durable and resumable
11. Do not persist private chain-of-thought
12. Build the vertical slice before the platform

## Documentation

1. [`01_PRODUCT_REQUIREMENTS.md`](docs/01_PRODUCT_REQUIREMENTS.md)
2. [`02_MVP_ARCHITECTURE.md`](docs/02_MVP_ARCHITECTURE.md)
3. [`03_AGENT_RUNTIME.md`](docs/03_AGENT_RUNTIME.md)
4. [`04_TOOLS_AND_SANDBOX.md`](docs/04_TOOLS_AND_SANDBOX.md)
5. [`05_CONTEXT_AND_INDEXING.md`](docs/05_CONTEXT_AND_INDEXING.md)
6. [`06_MODEL_STRATEGY.md`](docs/06_MODEL_STRATEGY.md)
7. [`07_API_AND_DATA_MODEL.md`](docs/07_API_AND_DATA_MODEL.md)
8. [`08_SECURITY_AND_PERMISSIONS.md`](docs/08_SECURITY_AND_PERMISSIONS.md)
9. [`09_TESTING_EVALS_OBSERVABILITY.md`](docs/09_TESTING_EVALS_OBSERVABILITY.md)
10. [`10_DEPLOYMENT_AND_LOCAL_DEV.md`](docs/10_DEPLOYMENT_AND_LOCAL_DEV.md)
11. [`11_MVP_ROADMAP.md`](docs/11_MVP_ROADMAP.md)
12. [`12_FULL_VERSION_ROADMAP.md`](docs/12_FULL_VERSION_ROADMAP.md)
13. [`13_PRODUCT_UX.md`](docs/13_PRODUCT_UX.md)
14. [`14_ENGINEERING_BACKLOG.md`](docs/14_ENGINEERING_BACKLOG.md)
15. [`15_ADRS_RUNBOOKS_RELEASE.md`](docs/15_ADRS_RUNBOOKS_RELEASE.md)
16. [`16_REFERENCES.md`](docs/16_REFERENCES.md)

Consolidated specification: `docs/OVX_FULL_DEVELOPMENT_SPEC.md`

## Ownership

**OVX** is an Ompirit Software project.

Website: https://www.ompirit.com

## License

A project license should be finalized before external contributions. Apache-2.0 is a strong candidate, but the final choice should reflect Ompirit Software's product and legal strategy.

**Build the smallest agent that can safely complete a real coding task. Make it observable, recoverable, testable, and useful. Then scale it into the broader OVX platform.**
