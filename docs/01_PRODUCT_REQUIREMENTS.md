# OVX Product Requirements

## Purpose
OVX is a repository-aware coding agent that helps developers understand, modify, test, and review software safely. It is provider-independent and local-first.

The MVP must prove that OVX can complete a real repository task without unrestricted host access and without depending on one proprietary model API.

## Target users
Primary:
- individual software engineers;
- Ruby on Rails developers;
- maintainers of legacy applications;
- teams evaluating local/self-hosted coding agents.

Later:
- platform teams;
- enterprise engineering organizations;
- regulated teams requiring auditability.

## Primary jobs
A developer should be able to:
- open a repository;
- ask repository questions;
- request a bug fix or small refactor;
- investigate failing tests;
- run targeted tests;
- inspect actions;
- approve/reject risky operations;
- review diffs before commit;
- resume interrupted tasks.

## MVP use cases
### Fix a failing test
Identify relevant code, propose a plan, patch, run the relevant spec, and present diff plus validation.

### Explain repository behavior
Answer with repository evidence and visible source provenance.

### Safe refactor
Update implementation while preserving explicit constraints and public behavior.

### Investigate a defect
Trace code paths and tests, propose the smallest change, and avoid claiming success until validation passes.

## Functional requirements
### Repository
- register/open repository;
- create isolated workspace;
- list/search/read files;
- Git status/diff;
- framework detection;
- Rails structure detection.

### Agent
- create task;
- preserve constraints;
- build context;
- create plan;
- execute bounded loop;
- persist state/events;
- recover from crashes;
- cancel/resume;
- summarize result and risks.

### Tools
Each tool has:
- schema;
- capabilities;
- validation;
- timeout;
- cancellation;
- audit event;
- deterministic error classification where practical.

### Security
- sandbox execution;
- default-deny network;
- workspace-scoped filesystem;
- secret isolation;
- resource limits;
- approval for elevated capabilities;
- no model-to-host execution.

### Validation
- targeted tests;
- optional linting;
- Git diff;
- explicit completion criteria.

## Non-functional requirements
### Reliability
No silent failures, safe retries, durable state, recoverable interruption, and explicit `unknown` state for ambiguous side effects.

### Security
Treat repository content and model output as untrusted.

### Performance
Optimize correctness before latency. Keep search practical on medium repositories.

### Auditability
Users can inspect model/tool activity at an operational level, approvals, state transitions, file changes, and test results.

## MVP exclusions
Not required initially:
- multi-agent orchestration;
- browser automation;
- cloud task farms;
- organization-wide indexing;
- autonomous production deployment;
- background issue processing;
- VS Code extension;
- plugin marketplace;
- mandatory vector database.

## Success criteria
The MVP repeatedly solves controlled coding tasks with:
- correct patch;
- required tests passing;
- no forbidden side effects;
- auditable action trail;
- recoverable task state;
- stable provider adapter behavior.
