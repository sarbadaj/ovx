# OVX Tools and Sandbox

## Tool philosophy
Models never receive arbitrary host control. Side effects happen through structured tools with schema, capability declaration, scope, validation, timeout, and audit records.

## MVP tools
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

## Tool request envelope
```json
{
  "tool_call_id": "uuid",
  "tool": "run_tests",
  "workspace_id": "uuid",
  "arguments": {},
  "capabilities": [],
  "timeout_seconds": 300
}
```

## Capability model
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

Permissions can be scoped, for example `filesystem.write: workspace/**`.

## Policy results
```text
allow
allow_once
require_approval
deny
```

Inputs include tool, capability, path, command intent, network need, external side effect, task state, and user policy.

## Filesystem safety
Reject:
- paths outside workspace;
- traversal after normalization;
- symlink escape;
- unauthorized mounts;
- unsafe devices.

Mutations should support expected file hashes.

## Command execution
Controls:
- sandbox-only;
- workspace working directory;
- environment allowlist;
- timeouts;
- PID/memory/CPU/disk limits;
- output limits;
- no Docker socket;
- no privileged mode;
- no host PID namespace.

## Network
Default deny. Controlled package installation or external operations require policy/approval. Future policies may allow domains.

## Secrets
Never mounted automatically. Explicit authorization, scope, redaction, and short-lived injection where practical.

## Git
Read-only Git is low risk. Commit is local mutation. Push is external and requires approval by default. Untrusted hooks should not run automatically.

## Patch safety
`apply_patch` validates paths, expected hash/context, reports exact changes, and fails cleanly on conflicts.

## Sandbox image
Use minimum required tooling. Do not use privileged containers or host credential mounts.
