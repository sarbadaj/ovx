# Validation — 28 September 2026

## Additional checks — 29 September 2026

- SmartGuruji's Rails app was prepared in an isolated Ruby 3.4.2 image with its locked gems and a disposable PostgreSQL database. OVX ran the full RSpec suite from the nested `server` directory: **141 examples, 1 failure**. The remaining failure is a 429 quota expectation receiving 201; it is a project logic or fixture issue, not a missing Gemfile/dependency error.
- A new Ruby starter project was created in a non-Git folder, registered with OVX, snapshotted, and its Minitest suite passed inside the runtime sandbox: **1 run, 1 assertion, 0 failures**.
- With the real local model, starter task 7 added `Project.add` and a matching assertion. OVX ran `ruby test/project_test.rb` successfully, produced a two-file diff, and left the source unchanged until the reviewed patch was accepted. Independent Minitest after acceptance passed: **1 run, 1 assertion, 0 failures**.
- Extension: **19 tests passed**; API: **38 runs, 133 assertions passed**; Go runtime suite passed; CLI/project-preparation: **9 tests passed**.
- The actual SmartGuruji repair has not been verified as complete. OVX's 3B local model does not guarantee correct diagnosis or edits for complex Rails code.

## Repair workflow update (0.1.2)

- Repair prompts such as `fix rspec issue` now create coding tasks, with detected nested application directories included as context.
- Sandbox tests support a validated `cwd` such as `server`; Docker execution, timeout, and metadata protection checks passed.
- Unsupported `bundle install` requests no longer create approvals that the runtime cannot execute. Repeated failed paths trigger repository discovery.
- Real local-model task 27 repaired the Rails fixture controller, passed RSpec, produced the expected diff, preserved source until acceptance, and passed an independent RSpec run afterward (1 example, 0 failures).
- This validates the bundled Rails environment. SmartGuruji still requires a sandbox containing its dependencies and test services; its full repair workflow is not yet verified.
- The conversation layout keeps the prompt composer below task progress and review. It is an OVX interface, not an exact reproduction of Codex.

Validated on this Apple Silicon Mac (8 GB RAM). These results cover the delivered
local workflow and bundled fixtures, not arbitrary repositories or model tasks.

| Check | Result |
| --- | --- |
| `./bin/ovx test` | Passed: Go runtime, 7 Python CLI tests, 12 extension tests, 30 Rails tests / 99 assertions. |
| `OVX_TEST_DOCKER=1 go test -race ./...` | Passed, including real Docker execution, protected Git metadata and timeout cleanup. |
| Real VS Code extension host | 7 checks passed in VS Code 1.139.1 using a loopback API fixture: activation, SecretStorage/auth, sidebar, tasks/history, native diff, acceptance, rerun/rejection. |
| Mock infrastructure smoke | Passed through Rails → Sidekiq → Go → Docker → tests → diff → acceptance (task 1). |
| Real local model / Rails workflow | Passed using Qwen2.5-Coder-3B-Instruct Q4_K_M served by llama.cpp b11207 (task 2). |
| VSIX installation | `ompirit.ovx-local@0.1.0` installed and confirmed in the user's VS Code extension list. |
| Local service status | API, runtime, worker and local model running. PostgreSQL/Redis available through local Docker. |
| Source formatting | `git diff --check` passed. |

## Real model evidence

The Rails fixture initially failed its request spec: `/sum?left=2&right=3`
returned `-1` instead of `5`. The local model read the repository, changed only
`app/controllers/calculations_controller.rb` from `left - right` to `left + right`,
ran RSpec successfully and inspected the diff.

Verification confirmed:

- the source repository was unchanged before acceptance;
- the reviewed patch changed only the controller;
- the request spec was not modified;
- acceptance applied the reviewed patch to the source;
- an independent RSpec run after acceptance reported **1 example, 0 failures**.

The real-model Rails test ran through the API/CLI. The real VS Code host test used
a loopback fixture; a combined live-model GUI session was not exercised.

## Repeat the checks

```sh
./bin/ovx test
./bin/verify-rails
```

`verify-rails` creates its own disposable committed Rails repository, requires the
running real model and `OVX_SANDBOX_IMAGE=ovx-sandbox-demo-rails:latest`, checks the
failing baseline, submits a task, verifies the diff and explicitly accepts that
demo-only patch. It writes `.ovx/validation/rails-e2e-result.json`.

For Docker race checks, run from `services/runtime` with the installed Go binary
on PATH and `OVX_TEST_DOCKER=1`. For the deterministic calculator smoke, switch the
service to `MODEL_PROVIDER=mock`, restart and run `./bin/ovx smoke`; switch it back
to `openai_compatible` and restart afterwards.

Local evidence: `.ovx/validation/tests.log`,
`.ovx/validation/rails-e2e-result.json`, `.ovx/validation/ready-demo.txt`.
Secrets, model weights, downloaded tools and validation workspaces are ignored by Git.

## Practical limits

The 3B model is suitable for small tasks; one successful fixture does not establish
reliable performance on large Rails changes. Real projects require a matching
sandbox image and test dependencies. Network-disabled tests cannot connect to
host databases or external services. See README.md for project configuration.

VS Code requires one-time installation selection and token import in the user's
profile. GUI control of that profile was unavailable, so this step remains a user
action; the extension and local services are already installed.
