# OVX ADRs, Runbooks, and Release

## Architecture Decision Records
- ADR-001 Rails as control plane
- ADR-002 Go as execution runtime
- ADR-003 Docker as initial sandbox
- ADR-004 Provider-independent ModelAdapter
- ADR-005 Capability-based permissions
- ADR-006 Durable event journal
- ADR-007 Optimistic file mutation
- ADR-008 Network denied by default
- ADR-009 Rails-first specialization
- ADR-010 Vertical-slice-first MVP

## Runbook — Stuck task
1. inspect task state;
2. inspect latest event;
3. inspect active tool attempt;
4. determine if process exists;
5. cancel or mark unknown;
6. reconcile side effects;
7. resume or require user decision.

## Runbook — Sandbox failure
Persist failure, preserve diagnostics, destroy failed container, verify workspace integrity, create clean sandbox, retry only if safe.

## Runbook — Model unavailable
Persist error, retry by policy, optionally use configured fallback, preserve task state.

## Runbook — Ambiguous external action
Example: connection lost during `git push`.
1. mark attempt unknown;
2. inspect remote state;
3. compare expected ref;
4. mark completed if proven;
5. otherwise require user decision;
6. never blindly replay.

## Release checklist
- tests green;
- eval baseline met;
- migration review;
- security review;
- runtime protocol compatible;
- docs updated;
- changelog;
- version tag;
- artifacts.

## Versioning
Use semantic versioning after public releases begin.

## Compatibility
Track control-plane protocol, runtime protocol, tool-schema, database schema, and desktop API versions.
