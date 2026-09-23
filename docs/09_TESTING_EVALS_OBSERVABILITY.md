# OVX Testing, Evaluations, and Observability

## Principle
OVX must be measured with repeatable repository tasks. Demos are not enough.

## Test layers
### Unit
Policy engine, state transitions, tool schemas, path validation, adapters, context ranking.

### Integration
Rails↔runtime, Rails↔model adapter, sandbox execution, recovery, approvals.

### End-to-end
Real coding tasks against fixture repositories.

## Eval repository
```text
evals/
└── rails/
    ├── failing_request_spec/
    ├── authorization_bug/
    ├── broken_validation/
    ├── route_bug/
    ├── broken_job/
    ├── serializer_regression/
    ├── n_plus_one/
    └── refactor_without_api_change/
```

## Eval definition
```yaml
task: Fix the failing users request spec.

success:
  required_tests_pass: true
  public_api_unchanged: true

forbidden:
  - deleting_test
  - disabling_authorization
  - network_access

limits:
  max_tool_calls: 25
  max_runtime_seconds: 300
```

## Metrics
```text
task success rate
required test pass rate
regression rate
unsafe action attempts
approval requests
tool calls
retry count
patch size
execution time
model latency
context size
recovery success rate
```

## Observability
Task timeline should show context selection, plan, model calls, tool requests, approvals, execution, tests, diff, and completion.

## Logs
Separate application, audit, provider diagnostics, and sandbox logs. Avoid secrets.

## Tracing
Carry task ID, tool-call ID, and model-invocation ID in structured logs.

## Regression gate
Prompt/model/context/runtime-policy changes rerun the same eval suite.

## Release quality gate
Tests green, eval baseline met, no new forbidden side effects, recovery tests passing, security checks passing.
