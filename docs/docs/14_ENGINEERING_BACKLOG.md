# OVX Engineering Backlog

## P0 — Control plane
- [ ] Rails API skeleton
- [ ] Task/Workspace/AgentEvent models
- [ ] state machine
- [ ] cancellation
- [ ] recovery coordinator

## P0 — Model
- [ ] ModelAdapter
- [ ] llama.cpp adapter
- [ ] vLLM adapter
- [ ] capability discovery
- [ ] structured tool-call normalization

## P0 — Runtime
- [ ] Go service
- [ ] authenticated protocol
- [ ] tool registry
- [ ] timeout/cancellation
- [ ] resource limits
- [ ] normalized results

## P0 — Sandbox
- [ ] Docker sandbox
- [ ] non-privileged mode
- [ ] workspace-only mount
- [ ] network default deny
- [ ] PID/memory/CPU/disk limits
- [ ] no Docker socket
- [ ] cleanup lifecycle

## P0 — Tools
- [ ] list_files
- [ ] read_file
- [ ] search_code
- [ ] apply_patch
- [ ] run_tests
- [ ] run_command
- [ ] git_status
- [ ] git_diff
- [ ] git_log

## P0 — Reliability
- [ ] ToolCall lifecycle
- [ ] ToolAttempt lifecycle
- [ ] idempotency keys
- [ ] unknown execution state
- [ ] crash recovery
- [ ] safe retry rules
- [ ] optimistic file hashes

## P0 — Security
- [ ] capability model
- [ ] path canonicalization
- [ ] symlink protection
- [ ] network policy
- [ ] secret isolation
- [ ] approval engine
- [ ] audit schema
- [ ] Git hook protections

## P0 — Evals
- [ ] Rails fixtures
- [ ] eval runner
- [ ] success assertions
- [ ] forbidden-action assertions
- [ ] metrics
- [ ] baseline report

## P1 — Repository intelligence
- [ ] file tree
- [ ] framework detection
- [ ] Tree-sitter symbols
- [ ] Rails graph
- [ ] test/source mapping
- [ ] relevance scoring
- [ ] context budget
- [ ] provenance

## P1 — Rails
- [ ] find_route/controller/model/association/policy/job/spec
- [ ] run_rspec
- [ ] run_rubocop
- [ ] detect_n_plus_one

## P1 — Desktop
- [ ] Tauri app
- [ ] task view
- [ ] tool timeline
- [ ] approval dialog
- [ ] diff/test viewers
- [ ] cancel/retry/resume

## P2 — Integrations
- [ ] GitHub
- [ ] GitLab
- [ ] PRs/issues
- [ ] CI
- [ ] VS Code

## P3 — Platform
- [ ] remote workers
- [ ] org policies
- [ ] RBAC
- [ ] team workspaces
- [ ] model routing
- [ ] multi-agent experiments
