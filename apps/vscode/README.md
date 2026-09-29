# OVX for VS Code

Use your local OVX service from the VS Code sidebar: submit a coding task, follow its plan and tool timeline, inspect test results, open native before/after diffs, then accept or reject the result.

## Install and connect

1. Set up OVX from the repository root with `./bin/setup`, configure your model and workspace root, then run `./bin/ovx start`.
2. Build the extension from this directory: `npm run package`. It needs Node.js and Python 3, with no npm dependencies or downloads.
3. Install `dist/ovx-local-0.1.7.vsix` using **Extensions → … → Install from VSIX**, or `code --install-extension dist/ovx-local-0.1.7.vsix`.
4. Open any local project folder, including a dirty or non-Git folder. Trust the workspace in VS Code; saved files are snapshotted for agent tasks.
5. Open the OVX activity bar. Click **Set token** and paste `OVX_API_TOKEN` from the OVX installation’s `.env`. Alternatively, choose **Import token**, explicitly select the OVX installation directory, and import its token.
6. Click **Connect**, select the repository when using a multi-root workspace, and submit a task such as “Fix this failing RSpec test. Run the relevant tests and inspect the diff.”

The extension stores the API token in VS Code SecretStorage, separately for each service URL. It does not store tokens in workspace settings or the webview. **Start service** runs `bin/ovx start` from the installation you explicitly select; it never runs a launcher inferred from your project. One-time setup remains a terminal command so you can inspect dependency installation and Docker configuration.

## Review and control

- **History:** persisted tasks for the selected repository, restored when the editor reopens.
- **Plan and timeline:** live progress with expandable tool requests and results.
- **Tests:** commands, durations, exit codes, and captured output.
- **Changes:** click a file to open immutable before/after snapshots in VS Code’s diff editor.
- **Accept changes:** applies the completed task’s patch to the original repository. Save or discard editor buffers first. The service also verifies the repository’s revision and cleanliness.
- **Reject changes:** discards the task workspace and records rejection.
- **Cancel task:** requests cancellation through the local service.
- **Retry / Resume safely:** resumes failed or interrupted tasks through the service’s recovery flow. **Run again** starts a fresh task with the same request for completed or cancelled tasks.
- **Approvals:** inspect the exact command, reason, and capabilities, then choose **Allow once** or **Deny**. Allowing a command does not bypass runtime restrictions.

The agent works in its own task workspace until you accept. Source changes are not auto-committed. New and deleted files appear in the diff; binary changes need manual inspection. The model and sandbox image must be configured in OVX for your project; the extension does not download a model or install project dependencies. Mock mode is only a deterministic demonstration, not a general coding model.

## Settings and commands

| Setting | Default | Meaning |
| --- | --- | --- |
| `ovx.serviceUrl` | `http://127.0.0.1:3000` | HTTP(S) loopback API address; credentials, URL paths, and remote hosts are rejected. |
| `ovx.installationPath` | empty | Absolute OVX installation directory, selected explicitly. |
| `ovx.pollIntervalMs` | `1500` | Task refresh interval, 1–30 seconds. |

These settings are machine scoped. Repository settings cannot redirect a stored token or select a launcher. With Remote SSH / WSL / Dev Containers, the extension runs in the workspace extension host: OVX and the repository must be on that same host, and its loopback address is used. Virtual workspaces are unsupported. macOS and Linux are the supported OVX service hosts.

Command Palette commands use the **OVX:** prefix: **Open Agent**, **Connect to Local Service**, **Set API Token**, **Choose OVX Installation**, **Import Token from OVX Installation**, **Start Local Service**, and **Refresh Tasks**.

## Troubleshooting

- **Connection refused:** run `bin/ovx status` and `bin/ovx doctor`, then `bin/ovx start` from the OVX installation. Check the **OVX** output channel and `bin/ovx logs`.
- **Unauthorized:** import the current `OVX_API_TOKEN` again. API and runtime tokens are different; use the API token in this extension.
- **Repository rejected:** reconnect the selected folder and save editor buffers. OVX enrolls the exact folder and supports uncommitted/non-Git projects.
- **Run full RSpec:** type `run full rspec`, `run rspec`, `run rspec of this project`, or use **Run full RSpec in project**. OVX detects a nested `Gemfile` + `spec` folder (including `SmartGuruji/server`), prepares its locked gems, then runs `bundle exec rspec` in an isolated Docker copy. Output appears in the integrated terminal; host Ruby and the host database are not required.
- **Task remains queued:** inspect Sidekiq and model status from the CLI.
- **Task timed out during submission:** click **Refresh** and inspect history before submitting again; the service may already have accepted the task.
- **Accept rejected:** inspect `git status`, save editor buffers, and verify the source revision has not changed. Start a fresh task if the base has moved.
- **Missing model / sandbox gems:** configure the local model and build a sandbox image for the target repository. The extension displays the service error and recorded test output.

## Development and validation

Open this directory in VS Code and press **F5** to run an Extension Development Host. No transpilation is needed.

```sh
npm run check
npm test
npm run package
```

Tests use Node’s built-in test runner and a local HTTP fixture to exercise authenticated task creation, history, approvals, diff documents, patch decisions, retries, trust checks, and workspace switching. `test/extension-host.js` additionally exports a VS Code extension-test entry point that exercises the real editor APIs against the same local fixture. It can be run with a disposable VS Code profile:

```sh
mkdir -p /tmp/ovx-code-test-repository
code --user-data-dir /tmp/ovx-code-test-profile \
  --extensions-dir /tmp/ovx-code-test-extensions \
  --extensionDevelopmentPath="$PWD" \
  --extensionTestsPath="$PWD/test/extension-host.js" \
  --disable-workspace-trust --skip-welcome --skip-release-notes \
  /tmp/ovx-code-test-repository
```

This package is for local installation. Building the VSIX does not publish it to a marketplace. No separate license grant is introduced by this extension.
