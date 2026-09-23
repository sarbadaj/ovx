# Security Policy

## Reporting
Do not open a public issue for vulnerabilities that could expose users, repositories, credentials, or execution environments. Until a dedicated security contact is published, report privately to OVX/Ompirit maintainers.

## Security principles
OVX treats model output, repository content, generated commands, and dependency scripts as untrusted.

OVX aims to enforce:
- sandboxed execution;
- default-deny network;
- workspace-scoped filesystem;
- secret isolation;
- capability-based permissions;
- human approval for sensitive actions;
- durable audit records.

## Important vulnerability classes
- sandbox escape;
- arbitrary host execution;
- credential leakage;
- path traversal;
- symlink escape;
- command-policy bypass;
- approval bypass;
- network-policy bypass;
- prompt injection causing unauthorized tool use;
- unsafe recovery that repeats external side effects.

## Supported versions
During pre-release development, only the latest main branch may receive fixes. Add a formal version table after public releases begin.
