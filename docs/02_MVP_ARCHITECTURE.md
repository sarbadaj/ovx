# OVX MVP Architecture

## Goals
The MVP must be provider-independent, local-first, safe, resumable, observable, and extensible.

## Core systems

### Rails Control Plane
Owns:
- API;
- task/session state;
- agent loop;
- context construction;
- model invocation;
- approvals;
- policy decisions;
- events;
- recovery coordination;
- repository metadata.

Rails must not directly execute arbitrary repository commands on the host.

### Model Gateway
Owns:
- normalized request/response;
- streaming;
- structured tool-call normalization;
- model capabilities;
- context/token limits;
- provider abstraction.

It never executes repository actions.

### Go Runtime Service
Owns:
- tool validation;
- filesystem scope;
- process execution;
- sandbox lifecycle;
- timeouts;
- cancellation;
- CPU/memory/PID/disk limits;
- result normalization.

### Sandbox
Contains:
- repository;
- tests;
- Git;
- shell/build tools;
- untrusted repository execution.

Docker is the initial mechanism.

## Logical architecture
```text
Desktop / CLI
      │
      ▼
Rails Control Plane
 ┌────┴──────────────┐
 │                   │
 ▼                   ▼
Model Gateway     Go Runtime
 │                   │
 ▼                   ▼
Model Provider     Sandbox
                     │
                     ▼
              Repository/Tests/Git
```

PostgreSQL stores durable state. Redis/Sidekiq supports asynchronous indexing/work.

## Trust boundaries
Trusted:
- OVX policy configuration;
- OVX application/runtime code;
- database integrity controls.

Untrusted:
- model output;
- repository contents;
- dependency scripts;
- generated patches;
- shell commands;
- test code;
- Git hooks;
- external network responses.

## Request flow
1. User submits task.
2. Rails creates task/workspace.
3. Context engine selects evidence.
4. Model Gateway gets normalized request.
5. Model proposes response/tool call.
6. Rails validates task-state compatibility.
7. Policy evaluates capabilities.
8. Approval occurs if needed.
9. Runtime validates again.
10. Runtime executes in sandbox.
11. Result is persisted.
12. Agent continues/validates.
13. User receives diff, tests, summary.

## Failure boundaries
- model failure must not corrupt execution state;
- runtime failure must not corrupt task history;
- sandbox failure must be observable/recoverable;
- database failure should stop new side effects until durable state is available.

## Initial local topology
```text
desktop/cli
rails-api
postgres
redis
runtime-go
model-server
docker-engine
```

## Future topology
May support remote runtimes, remote model pools, policy services, audit services, isolated worker fleets, and enterprise identity without rewriting the agent core.
