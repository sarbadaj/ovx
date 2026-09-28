# OVX API and Data Model

## Core entities
```text
User
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

## Repository
```text
id, name, path_or_reference, default_branch, framework, language_summary, timestamps
```

## Workspace
```text
id, repository_id, task_id, sandbox_reference, base_revision, status, timestamps
```

## Task
```text
id, repository_id, user_request, constraints, state, current_plan,
max_tool_calls, started_at, completed_at, cancelled_at
```

## AgentEvent
Append-only:
```text
id, task_id, sequence, event_type, payload, created_at
```

Events include task/context/plan/model/tool/approval/validation/completion transitions.

## ToolCall
```text
id, task_id, tool_name, arguments, input_hash, state, risk_level,
required_capabilities, idempotency_key, created_at
```

## ToolAttempt
```text
id, tool_call_id, attempt_number, state, started_at, finished_at,
exit_code, stdout_reference, stderr_reference, error_class, result
```

## Approval
```text
id, task_id, tool_call_id, status, requested_capabilities,
reason, requested_at, decided_at, decided_by
```

## ContextItem
```text
id, task_id, path, start_line, end_line, retrieval_method,
reason, relevance_score, content_hash
```

## ModelInvocation
```text
id, task_id, provider, model, request_summary, input_units,
output_units, finish_reason, latency_ms, created_at
```

Store operationally useful summaries, not private reasoning.

## Patch
```text
id, task_id, tool_call_id, path, expected_hash, result_hash,
patch_text, status
```

## TestRun
```text
id, task_id, command, scope, examples, failures, exit_code,
duration_ms, result_summary
```

## API sketch
```text
POST /api/repositories
GET  /api/repositories/:id
POST /api/tasks
GET  /api/tasks/:id
POST /api/tasks/:id/cancel
POST /api/tasks/:id/resume
GET  /api/tasks/:id/events
GET  /api/tasks/:id/diff
POST /api/approvals/:id/approve
POST /api/approvals/:id/deny
```

## Concurrency
Use optimistic locking/version checks. One task should not have independent side-effect executors without coordination.

## Retention
Make logs, model metadata, tool output, indexes, and artifacts configurable.
