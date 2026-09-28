# OVX Agent Runtime

## Purpose
The runtime moves a coding task through context, planning, tool execution, validation, approval, and completion. It must be explicit, durable, bounded, and recoverable.

## Task states
```text
created
workspace_preparing
context_building
planning
executing
validating
awaiting_approval
paused
recovery_required
completed
failed
cancelled
```

## Agent loop
```text
observe → select context → model proposes → validate → execute → persist observation → continue/finish
```

Limits:
- max tool calls;
- max task duration;
- max retries;
- context budget;
- optional model budget.

## Tool-call lifecycle
```text
requested
authorized
started
completed
failed
cancelled
unknown
```

`unknown` is used when OVX cannot prove whether a side effect completed.

## Idempotency
Naturally idempotent:
- read/list/search;
- git status/diff.

Conditionally idempotent:
- apply patch;
- write file;
- run tests.

Non-idempotent/external:
- git push;
- create PR;
- deploy;
- Terraform apply;
- remote API mutations.

Never blindly replay ambiguous non-idempotent operations.

## Crash recovery
1. find non-terminal tasks;
2. inspect latest state/events;
3. identify `started` tool attempts;
4. reconcile if possible;
5. mark unresolved ones `unknown`;
6. move task to `recovery_required` if necessary;
7. require user decision when external state cannot be proven.

## Cancellation
Cancellation stops future dispatch, signals running process, force-stops after grace period if needed, and persists outcome.

## Retry policy
Retry automatically only when safe and transient. Do not retry ambiguous external side effects.

## Plans
Plans are user-visible operational summaries, not hidden reasoning. They include interpretation, constraints, steps, and validation.

## Completion
A task is complete only when:
- requirements are addressed;
- validation is recorded;
- diff is available if code changed;
- remaining risks are summarized;
- no unresolved approval remains.

Model confidence is not completion evidence.
