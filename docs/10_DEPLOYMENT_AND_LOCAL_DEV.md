# OVX Deployment and Local Development

## Local goal
A developer should run OVX locally with a small service set:

```text
Rails API
PostgreSQL
Redis
Go runtime
Docker
local model server
optional desktop/CLI
```

## Prerequisites
- Ruby;
- Node.js;
- Go;
- PostgreSQL;
- Redis;
- Docker;
- Git;
- llama.cpp or compatible model server.

## Configuration
Suggested groups:
```text
DATABASE_*
REDIS_*
OVX_RUNTIME_*
OVX_MODEL_*
OVX_SANDBOX_*
OVX_POLICY_*
```

Never commit secrets.

## Developer commands
Recommended:
```text
make setup
make dev
make test
make eval
make lint
```

## Runtime connectivity
Rails talks to Go runtime over a local authenticated service interface. Runtime is not public by default.

## Model connectivity
Gateway supports local llama.cpp, local/remote vLLM, and future adapters.

## Sandbox lifecycle
1. create workspace;
2. safely copy/mount repository;
3. start sandbox with policy;
4. execute task;
5. preserve selected artifacts;
6. destroy sandbox;
7. clean according to retention policy.

## Future production
May include Rails nodes, Sidekiq workers, managed Postgres/Redis, runtime worker fleet, model serving, artifact storage, centralized observability.

## Compatibility
Version control-plane protocol, runtime protocol, tool schema, DB schema, and desktop API.
