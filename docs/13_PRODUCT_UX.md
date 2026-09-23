# OVX Product UX

## Principle
OVX should feel like a skilled engineering collaborator whose actions are visible and controllable.

Users should know:
- what OVX is doing;
- why;
- what changed;
- what validation ran;
- what needs approval.

## Core screens
### Repository
Open/recent repositories, detected language/framework, branch, workspace status.

### Task
Request, plan, messages, context evidence, status, tool activity, tests, diff.

### Approval
Show action, reason, capabilities, affected resources, risk.

Actions:
```text
Allow once
Allow for this task
Deny
```

### Diff viewer
File list, additions/deletions, inline diff, revert selected file, accept result.

### Test viewer
Command, scope, duration, passing/failing examples, useful output.

## Task timeline
```text
19:02 task created
19:02 repository scanned
19:03 plan created
19:03 searched UsersController
19:04 read UserPolicy
19:04 patch applied
19:05 ran 23 examples
19:05 23 passed
19:05 diff ready
```

## Recovery UX
For ambiguous interruption:
```text
Task interrupted during command execution.
OVX cannot prove whether the external action completed.

[Inspect State] [Resume Safely] [Mark Complete] [Cancel]
```

## Context UX
Advanced users can inspect why a file was selected, with retrieval reason and relevance.

## Error UX
Errors must be actionable, e.g. timeout details plus whether files changed.

## High-risk UX
Show exact command/operation, capabilities, remote target, and allow/deny controls.
