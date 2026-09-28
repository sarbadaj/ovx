# OVX Security and Permissions

## Security premise
Assume repository content and model output can be hostile.

## Threats
- prompt injection from source/docs;
- malicious command generation;
- path traversal;
- symlink escape;
- host filesystem access;
- secret discovery/exfiltration;
- network tunneling;
- dependency-install scripts;
- malicious Git hooks;
- fork/memory/disk exhaustion;
- sandbox escape attempts;
- dangerous cloud/infrastructure commands.

## Security layers
```text
User policy
   ↓
Rails policy engine
   ↓
Approval gate
   ↓
Runtime validation
   ↓
Sandbox isolation
   ↓
OS/container controls
```

## Default matrix

| Operation | Default | Approval |
|---|---|---|
| Read workspace | Allow | No |
| Search | Allow | No |
| Write workspace | Allow in task | Configurable |
| Delete files | Restricted | Often |
| Run tests | Allow in sandbox | No |
| General process | Restricted | Policy |
| Network | Deny | Yes/allowlist |
| Host secrets | Deny | Yes + scoped |
| Git commit | Restricted | Yes initially |
| Git push | Deny | Yes |
| Deploy | Deny | Yes |
| Host filesystem | Deny | Exceptional |

## Path protections
Canonicalize before checks. Reject workspace escape, unsafe symlinks, devices, unauthorized mounts, host paths.

## Prompt injection
Repository content is data, not policy. It cannot grant capabilities or change approval rules.

## Network
Denied by default. Enabled only per-task/policy with audit.

## Secrets
Require explicit need, authorization, scoped injection, redaction, and no normal-log persistence.

## Sandbox hardening
- no privileged mode;
- no Docker socket;
- drop Linux capabilities where practical;
- limited writable workspace;
- CPU/memory/PID/disk quotas;
- no host networking;
- seccomp/AppArmor/SELinux where available.

## Destructive operations
Require approval: `git push`, broad deletion, `rails db:drop`, `terraform apply`, `kubectl delete`, production deploy, secret access, and broader network access.

## Audit
Record task/user/tool/capabilities/approval/result/time/affected resource.
