# Contributing to OVX

Thank you for your interest in OVX.

## Focus
OVX is currently MVP-focused. Contributions should strengthen the core coding-agent workflow rather than expand the product surface too early.

## Preserve these principles
- provider independence;
- local-first operation;
- tool-driven side effects;
- capability-based security;
- explicit durable state;
- test-backed completion;
- auditability;
- minimal MVP scope.

## Workflow
1. create/select an issue;
2. discuss significant architecture changes;
3. create a branch;
4. add tests;
5. implement the smallest complete change;
6. run tests/evals;
7. open PR;
8. document behavior and risks.

## Pull requests
Explain problem, solution, architecture impact, tests, security implications, migrations, and eval impact.

## Architecture changes
Changes affecting control-plane/runtime boundaries, tool protocol, permissions, sandbox, persistent state, or model abstraction should add/update an ADR.

## Security
Do not publish exploitable vulnerabilities publicly. Follow `SECURITY.md`.
