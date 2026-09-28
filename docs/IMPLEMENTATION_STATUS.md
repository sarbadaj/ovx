# OVX implementation status

The current product path is **VS Code extension → local Rails API → Sidekiq agent → local model and Go runtime → isolated Git workspace and Docker tests → reviewed patch acceptance**. Start with the root [README](../README.md). Check [VALIDATION.md](../VALIDATION.md) for observed results; the feature list below is not a claim of successful execution on every project or model.

## Delivered behavior

| Area | Implementation |
| --- | --- |
| VS Code | Installable VSIX, sidebar prompt, multi-root selection, task history, plans, events, test output, native diff snapshots, approvals, accept/reject, retry/rerun, cancellation. |
| Connection | Loopback-only extension URLs, bearer authentication, SecretStorage, workspace trust, explicit installation selection and token import. |
| Lifecycle | `bin/ovx setup/start/stop/restart/status/doctor/logs`; one managed launch rather than separate application terminals. |
| Model | Calculator mock or OpenAI-compatible inference. Managed local GGUF startup; Apple Silicon installer pins llama.cpp b11207 and Qwen2.5-Coder-3B-Instruct Q4_K_M with checksum verification. |
| Persistence | PostgreSQL tasks, events, tool calls/attempts, approvals, results, transcripts, and repositories; Redis/Sidekiq queue. |
| Runtime | Explicit repository roots, isolated Git clones, bounded reads/searches/patches, file creation/deletion, Git inspection, sandboxed commands/tests. |
| Review | Added/modified/deleted file snapshots and patch hashes; source revision/cleanliness checks and the extension’s reviewed-patch guard. Accepted edits remain uncommitted. |
| Execution control | Task locking, approvals, cancellation, ambiguous-tool recovery, retries, and test/diff requirements after the latest mutation. |
| Tests and packaging | Offline VSIX builder; Node tests and real VS Code extension-host fixture; Go/Rails/CLI tests and deterministic infrastructure smoke. |

Tools: `list_files`, `read_file`, `search_code`, `apply_patch`, `create_file`, `delete_file`, `run_tests`, `run_command`, `git_status`, `git_diff`, `git_log`. File deletion and general commands require approval. Approval applies to the requested operation once and does not override runtime restrictions.

## Operational limits

- The selected folder is explicitly enrolled on connect. Agent tasks snapshot saved files and support dirty or non-Git folders; save editor buffers before starting. Nested Rails projects are detected by the VS Code local RSpec runner.
- Only acceptance changes the original repository. Source changes or a new base commit can invalidate acceptance; use a fresh task when needed.
- A 3B model is a modest local option on an 8 GB machine. Reasoning quality, structured-action validity, context size, memory, and project-specific tests limit successful completion. No general Rails coding-success claim is implied.
- Project dependencies must exist in the chosen sandbox image. Demo images are not universal Rails environments. Networking is disabled during tests, so host databases and external services are unavailable; use an appropriate container-local test setup.
- OVX’s PostgreSQL and Redis store agent state; they are not application test services available inside the sandbox.
- Mock mode exercises the calculator fixture only. Real-model results and infrastructure smoke must be reported separately.
- Service hosts are macOS/Linux. Managed model setup targets Apple Silicon macOS and requires Python 3.12+. Remote VS Code use requires the service and repository on the extension host. Virtual workspaces are unsupported.

## Reference design and future work

The [original project overview](README.md) and [numbered specifications](docs/) preserve the broader product architecture and roadmap. Their planned Tauri/React interface, Rails relationship graph, Tree-sitter indexing, semantic retrieval, richer context provenance, and evaluation program are reference designs rather than delivered capabilities.

Further roadmap work includes automatic project-environment provisioning, test-service orchestration, remote runtimes, team/RBAC controls, GitHub/GitLab integration, commits/PRs/deployment, richer capability policies, and stronger isolation for multi-user hosting. The local runtime does not expose unrestricted shell, host secrets, network-enabled tasks, Git push, or deployment tools.
