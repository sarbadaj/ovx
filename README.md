# OVX — local coding agent for VS Code

OVX runs coding tasks on your machine, prepares changes in an isolated Git workspace, runs tests in Docker, and presents the result in VS Code for acceptance or rejection. The model proposes structured actions; Rails and the Go runtime control their execution.

```text
VS Code + OVX extension
          │ authenticated localhost API
          ▼
Rails agent ─── PostgreSQL + Redis / Sidekiq
    │                           │
    ├── local llama.cpp model   └── Go runtime
    │                                  │
    └── plan / tool actions            ▼
                              isolated Git workspace
                              + Docker test sandbox
                                        │
                              tests → diff → Accept
                                        │
                              your original repository
```

The delivered interface is the VS Code extension and CLI. Read [implementation status](docs/IMPLEMENTATION_STATUS.md) for scope and [validation results](VALIDATION.md) for observed test results. The documents under [`docs/docs/`](docs/docs/) preserve the broader reference design and future roadmap.

## Set up and start

Run commands from the OVX checkout. Service hosts are macOS or Linux; the managed model installer targets Apple Silicon macOS. Prerequisites: Python **3.12+**, Git, Docker with Compose, Ruby 3.2+ and Bundler, Go 1.23+, ripgrep, Node.js 20+, npm, and VS Code. On Linux, install `llama-server` separately before `setup-model`; the automatic native runtime download supports Apple Silicon macOS. Start Docker Desktop/Engine first. Install VS Code’s `code` command in your shell for CLI extension installation.

```sh
./bin/ovx doctor
./bin/ovx setup
./bin/ovx setup-model
./bin/ovx start
./bin/ovx status
./bin/ovx install-extension
```

`setup` creates a private `.env` with separate API/runtime tokens, starts PostgreSQL and Redis, installs Rails dependencies, prepares the database, and builds the runtime and demo sandbox images. Setup and model installation download dependencies; ordinary tasks use the installed components.

`setup-model` installs checksum-verified **llama.cpp b11207** and **Qwen2.5-Coder-3B-Instruct Q4_K_M** and configures managed local inference. `start` manages the model, runtime, API, and worker together. A 3B model on an 8 GB machine is a constrained starting point: use small, targeted tasks and inspect tests and diffs carefully. Larger Rails changes may exceed its reasoning, context, or available-memory capacity. A configured model does not guarantee that an arbitrary coding task will succeed.

The generated VSIX is [`apps/vscode/dist/ovx-local-0.1.7.vsix`](apps/vscode/dist/ovx-local-0.1.7.vsix). If `code` is unavailable, run `./bin/ovx package-extension`, then use **Extensions → … → Install from VSIX**. Reload VS Code when prompted.

Use **New Ruby project** in the OVX sidebar to create a small tested Ruby starter in a folder you choose. The project opens in VS Code; submit a feature task after connecting. The agent supports Ruby/Rails tests in the configured sandbox. Other languages require a suitable runtime image before their tests can run.

For a Rails project, OVX prepares its test image when you submit a coding task. **Prepare Rails environment** also lets you run this setup separately. It selects Ruby from `.ruby-version` or `Gemfile.lock` and builds from the application's locked gems. Downloads happen during setup. Test runs use that image with a disposable PostgreSQL or SQLite database inside the network-disabled container; they do not use your host database or `.env`. The application needs `db/schema.rb` or its configured schema-load equivalent. External service integration tests still require project-specific fixtures or service setup.

The equivalent setup command for a nested application is:

```bash
python3 bin/ovx_prepare_rails.py /absolute/project --cwd server
```

Re-run preparation when locked dependencies change. Then submit `Fix the failing RSpec tests` in the sidebar; `run full rspec` alone runs the complete suite in an isolated project sandbox.

You can run the same suite without VS Code: `./bin/ovx run-rspec /absolute/path/to/project --cwd server`. Omit `--cwd server` when the `Gemfile` and `spec` directory are at the project root.

## Use the extension

1. Open the project folder in VS Code. Save editor buffers before starting a task; OVX now snapshots saved files, including uncommitted and non-Git projects.
2. The selected folder is enrolled explicitly when you connect. No commit or broad workspace-root allowlist is required.
3. Trust the workspace in VS Code. Run **OVX: Choose OVX Installation** and select this OVX checkout.
4. Run **OVX: Import Token from OVX Installation**, or use **OVX: Set API Token** with `OVX_API_TOKEN` from that installation’s `.env`.
5. Open the OVX activity bar, connect to `http://127.0.0.1:3000`, and select the repository when using a multi-root workspace.
6. Submit **“Fix this failing RSpec test. Keep the public API unchanged, run the relevant tests, and inspect the diff.”**

The sidebar shows task history, plans, tool events, test output, pending approvals, and changed files. Select a file for its native before/after diff. **Accept changes** applies the reviewed patch; **Reject changes** removes the task workspace. **Retry / Resume safely** resumes eligible failed or interrupted tasks; **Run again** creates a fresh task. **Cancel task** requests execution cancellation.

Source files remain unchanged until acceptance. OVX protects unrelated edits when applying a reviewed snapshot patch; accepted edits remain uncommitted. Tokens live in VS Code SecretStorage; repository settings cannot redirect them or select a launcher. See the [extension guide](apps/vscode/README.md) for all controls and remote extension-host behavior.

For projects outside OVX’s workspace directory, configure narrowly scoped paths:

```dotenv
OVX_REPOSITORY_ROOTS="/absolute/path/to/my-rails-app:/absolute/path/to/another-project"
```

## Try the Rails fixture

```sh
./bin/ovx demo rails
```

Open the printed repository path in VS Code. Type `run full rspec` to run the complete suite in the detected project folder (for `SmartGuruji`, open the parent and OVX detects `server`). Coding requests still use the agent. This small real Rails fixture needs no external database. The real local model has passed this fixture end to end; see [VALIDATION.md](VALIDATION.md).

## Match the sandbox to your project

The demo images cover the bundled fixtures. Your Rails application needs its Ruby version, gems, and system libraries installed in a project-specific image. Start with [`docker/sandbox-rails.example.Dockerfile`](docker/sandbox-rails.example.Dockerfile), build it using your project’s `Gemfile` and `Gemfile.lock`, then set:

```dotenv
OVX_SANDBOX_IMAGE=my-project-ovx-sandbox
```

Run `./bin/ovx restart` after changing the image. Tests run with Docker networking disabled: they cannot reach host PostgreSQL, Redis, external APIs, or OVX’s control-plane database. Rails projects using `pg` or `sqlite3` get their locked gems during image preparation and a disposable container-local test database. Other external services still need project-specific test fixtures or container-local setup.

## CLI and diagnostics

| Command | Purpose |
| --- | --- |
| `./bin/ovx start` / `stop` / `restart` | Manage services; stopping retains database volumes. |
| `./bin/ovx status` | Check API, runtime, worker, and model availability. |
| `./bin/ovx doctor` | Check prerequisites and Docker readiness. |
| `./bin/ovx logs` | Show recent logs; optionally append `api`, `worker`, `runtime`, or `model`. |
| `./bin/ovx demo` | Create a fresh fixture. |
| `./bin/ovx add-repo /absolute/path` | Register an allowed repository. |
| `./bin/ovx task REPO_ID "Fix the failing spec"` | Submit a task. |
| `./bin/ovx wait TASK_ID` | Follow a task until it stops or needs user action. |
| `./bin/ovx show TASK_ID` / `events TASK_ID` / `diff TASK_ID` | Inspect saved state and review artifacts. |
| `./bin/ovx apply TASK_ID` / `reject TASK_ID` | Accept or discard a completed result. |
| `./bin/ovx approvals` / `approve APPROVAL_ID` / `deny APPROVAL_ID` | Inspect and decide approvals. |
| `./bin/ovx cancel TASK_ID` / `retry TASK_ID` | Cancel or resume eligible tasks. |
| `./bin/ovx test` | Run runtime, CLI, extension, and Rails tests. |

The CLI reads `.env` itself; separate application terminals and `source .env` are unnecessary. Logs and process records live under `.ovx/`. For queued tasks, inspect worker and model logs. After a request timeout, inspect persisted history before resubmitting. If acceptance fails, check `git status` and the base revision; start a fresh task if the source has moved.

For a deterministic infrastructure check, set `MODEL_PROVIDER=mock`, restart OVX, and run `./bin/ovx smoke`. This calculator smoke checks the agent/runtime/test/diff/accept wiring; mock mode cannot handle general requests. Restore `MODEL_PROVIDER=openai_compatible` and restart before using the real model.

## Scope

The runtime permits bounded tools and sandboxed commands. General commands and file deletion require approval, which does not bypass runtime restrictions. Network-enabled tasks, deployment, Git push, automatic commits/PRs, and host-secret access are outside this implementation. Local inference keeps model context on your machine; configuring an external model endpoint sends context there.

This is a local development implementation. Tauri desktop, Rails symbol graphs, semantic indexing, remote runtimes, and team access controls remain roadmap work. [Implementation status](docs/IMPLEMENTATION_STATUS.md) separates delivered behavior from those design goals.
