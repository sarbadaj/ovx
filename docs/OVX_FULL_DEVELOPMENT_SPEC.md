# OVX Full Development Specification

## Executive summary

OVX is a provider-independent, repository-aware coding agent designed to perform real software-engineering tasks safely.

```text
Model        = proposes
Rails Agent  = orchestrates
Go Runtime   = validates and executes
Sandbox      = contains
PostgreSQL   = remembers
Human        = approves risky actions
Tests        = determine completion
```

The MVP succeeds when OVX can complete a real repository task, recover from interruption, provide an auditable execution history, and verify output with tests.

## Product scope
OVX supports repository understanding, search, reading, planning, patching, sandboxed execution, testing, Git diff, approvals, and durable state. Initial product is local-first and Rails-specialized.

## Architecture
### Rails Control Plane
Task lifecycle, agent state, repository metadata, context engine, model orchestration, approvals, policy, events, recovery.

### Model Gateway
Normalized `ModelAdapter` across llama.cpp, vLLM, and future providers.

### Go Runtime
Tool validation, filesystem scope, process control, timeout, cancellation, resource limits, sandbox lifecycle.

### Sandbox
Docker initially isolates repository, tests, commands, Git, and build tools.

## Agent lifecycle
```text
create task
prepare workspace
collect context
create plan
invoke model
validate action
request approval if needed
execute tool
persist result
continue
validate
show diff
complete
```

## Durable execution
Tool states:
```text
requested
authorized
started
completed
failed
cancelled
unknown
```

Non-idempotent external actions must not be blindly retried. OVX supports retries, cancellation, crash recovery, ambiguous-state reconciliation, and resumable tasks.

## Security
Capabilities:
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

Defaults: workspace read allowed; network/host secrets/host filesystem denied; external mutations require approval.

## Tools
Initial tools:
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

Every tool has schema, capabilities, timeout, scope, audit event, and normalized result.

## Context
Use file tree, ripgrep, Tree-sitter, Rails structure, dependencies, tests, Git history, and relevance ranking. Store provenance. Embeddings optional.

## Rails intelligence
Map routes, controllers, models, services, policies, serializers, jobs, and specs. Specialized tools include route/model/spec lookup, RSpec/RuboCop, and N+1 detection.

## Data model
Core entities:
```text
Repository
Workspace
Task
TaskRun
AgentEvent
ContextItem
ModelInvocation
ToolCall
ToolAttempt
Approval
Patch
TestRun
Artifact
PolicyProfile
```

## Model layer
`ModelAdapter` normalizes messages, tools, structured outputs, streaming, usage, finish reasons, and capabilities.

## Evaluation
Measure task success, tests, regressions, unsafe attempts, tool calls, retries, execution time, patch size, context size, and recovery success.

## UX
Repository selection, task chat, plan, context evidence, tool timeline, approvals, diff, tests, retry/cancel/resume.

## MVP roadmap
### 0.0.1 Vertical slice
Rails failing spec → context → plan → patch → RSpec → diff.

### 0.0.2 Durable execution
State machine, persistence, retries, cancellation, recovery.

### 0.0.3 Security
Capability policy, sandbox hardening, approvals, audit.

### 0.1 Rails intelligence
Rails graph, tools, evals.

### 0.2 Desktop
Tauri + React.

### 0.3 Collaboration
GitHub/GitLab and PR workflows.

## Full product direction
Remote runtimes, IDE integration, issue-to-code, PR review, team workspaces, model routing, organization intelligence, multi-agent workflows, enterprise identity, cloud-hosted OVX.

## Principles
1. Reliability before autonomy.
2. Local-first.
3. Provider-independent.
4. Tool-driven.
5. Capability-based security.
6. Human approval for risk.
7. Repository-aware context.
8. Tests determine completion.
9. Observable by design.
10. Durable and resumable.
11. No persisted private chain-of-thought dependency.
12. Build vertical slice before platform.

## MVP definition of done
- real Rails coding task completes end-to-end;
- execution sandboxed;
- network denied by default;
- sensitive actions approved;
- crashes preserve task history;
- ambiguous side effects are not blindly repeated;
- tests validate completion;
- diff/timeline inspectable;
- eval suite provides repeatable quality baseline.
