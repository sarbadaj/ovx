'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { Controller, webviewHtml } = require('../src/extension');
const { startFixture } = require('./fixture');

function fakeEnvironment(folder, url) {
  const secrets = new Map([[`ovx.apiToken:${url}`, 'test-token-only']]);
  const memory = new Map();
  const commands = [];
  const errors = [];
  const settings = { serviceUrl: url };
  const executions = [];
  const makeUri = (file, scheme = 'file') => ({ scheme, fsPath: file, toString: () => `${scheme}://${file}` });
  const vscode = {
    StatusBarAlignment: { Left: 1 }, ConfigurationTarget: { Global: 1 },
    TaskRevealKind: { Always: 1 }, TaskPanelKind: { Dedicated: 2 },
    ShellExecution: class { constructor(command, args, options) { Object.assign(this, { command, args, options }); } },
    Task: class { constructor(definition, scope, name, source, execution) { Object.assign(this, { definition, scope, name, source, execution }); } },
    tasks: { executeTask: async task => { const execution = { task, terminate: () => { execution.terminated = true; } }; executions.push(execution); return execution; } },
    Uri: { from: value => makeUri(`${value.authority}${value.path}`, value.scheme), joinPath: (uri, ...parts) => makeUri(path.join(uri.fsPath, ...parts), uri.scheme) },
    window: { createOutputChannel: () => ({ appendLine() {}, append() {}, show() {}, dispose() {} }), createStatusBarItem: () => ({ show() {}, dispose() {} }), showErrorMessage: value => errors.push(value), showInformationMessage() {} },
    workspace: { isTrusted: true, workspaceFolders: [{ name: 'repository', uri: makeUri(folder) }], textDocuments: [],
      getConfiguration: () => ({ inspect: name => ({ globalValue: settings[name], workspaceValue: 'http://evil.test' }) }) },
    commands: { executeCommand: async (...args) => commands.push(args) }
  };
  const context = { subscriptions: [], extensionUri: makeUri(path.dirname(__dirname)),
    secrets: { get: async key => secrets.get(key), store: async (key, value) => secrets.set(key, value) },
    workspaceState: { get: key => memory.get(key), update: async (key, value) => memory.set(key, value) }
  };
  return { vscode, context, commands, errors, settings, makeUri, executions };
}

async function setup(t) {
  const fixture = await startFixture();
  t.after(fixture.close);
  const folder = await fs.mkdtemp(path.join(os.tmpdir(), 'ovx-extension-'));
  t.after(() => fs.rm(folder, { recursive: true, force: true }));
  const environment = fakeEnvironment(folder, fixture.url);
  const controller = new Controller(environment.vscode, environment.context);
  t.after(() => controller.dispose());
  return { fixture, folder, controller, ...environment };
}

test('sidebar flow registers a repository, creates task, reviews native diff and accepts', async t => {
  const { controller, fixture, commands } = await setup(t);
  await controller.connect();
  await controller.message({ type: 'create', request: 'Fix the failing calculator test' });
  assert.equal(controller.error, '');
  assert.equal(controller.history.length, 1);
  assert.equal(controller.task.state, 'completed');
  assert.equal(controller.files[0].path, 'lib/calculator.rb');
  assert.equal(fixture.applyCount, 0, 'review must not mutate source');
  await controller.message({ type: 'diff', index: 0, taskId: 1 });
  assert.equal(commands[0][0], 'vscode.diff');
  assert.equal(controller.documents.get(commands[0][1].toString()), 'a - b\n');
  assert.equal(controller.documents.get(commands[0][2].toString()), 'a + b\n');
  await controller.connect();
  assert.equal(controller.files.length, 1, 'reconnecting restores the review snapshot');
  assert.equal(controller.reviewedPatchHash, 'a'.repeat(64));
  await controller.message({ type: 'action', action: 'accept', taskId: 1 });
  assert.equal(controller.task.resolution, 'accepted');
  assert.equal(fixture.applyCount, 1);
  await controller.message({ type: 'action', action: 'accept', taskId: 1 });
  assert.match(controller.error, /unavailable/);
  assert.equal(fixture.applyCount, 1, 'a resolved result must not be submitted twice');
});

test('reject, rerun, cancel, retry and approval decisions use the current task', async t => {
  const { controller, fixture } = await setup(t);
  await controller.connect();
  await controller.createTask('Fix calculator');
  await controller.taskAction('reject');
  assert.equal(controller.task.resolution, 'rejected');
  assert.equal(controller.files.length, 0);
  await controller.taskAction('retry');
  assert.equal(controller.task.id, 2);
  fixture.tasks[1].state = 'executing';
  await controller.refresh(true);
  await controller.taskAction('cancel');
  assert.equal(controller.task.state, 'cancelled');
  fixture.tasks[1].state = 'failed';
  await controller.refresh(true);
  await controller.taskAction('retry');
  assert.equal(controller.task.state, 'completed');
  for (const decision of ['approve', 'deny']) {
    fixture.tasks[1].state = 'awaiting_approval';
    fixture.tasks[1].approvals = [{ id: 7, status: 'pending', tool_call_id: 9, reason: 'Run linter', requested_capabilities: ['process.execute'] }];
    fixture.tasks[1].tool_calls.push({ id: 9, tool_name: 'run_command', arguments: { command: 'bundle exec rubocop' } });
    await controller.refresh(true);
    await controller.message({ type: 'approval', id: 7, decision, taskId: 2 });
    assert.equal(controller.error, '');
    assert.equal(controller.task.state, decision === 'approve' ? 'completed' : 'failed');
  }
  await controller.message({ type: 'action', action: 'retry', taskId: 1 });
  assert.match(controller.error, /selected task changed/);
});

test('trust and unsaved changes stop tasks and acceptance; a workspace cannot redirect tokens', async t => {
  const { controller, fixture, vscode, folder, makeUri } = await setup(t);
  vscode.workspace.isTrusted = false;
  await assert.rejects(controller.connect(), /Trust this workspace/);
  assert.equal(fixture.requests.length, 0);
  vscode.workspace.isTrusted = true;
  await controller.connect();
  assert.equal(controller.client.url, fixture.url);
  vscode.workspace.textDocuments = [{ isDirty: true, uri: makeUri(path.join(folder, 'lib', 'calculator.rb')) }];
  await assert.rejects(controller.createTask('Fix calculator'), /unsaved edits/);
  assert.equal(fixture.tasks.length, 0);
  vscode.workspace.textDocuments = [];
  await controller.createTask('Fix calculator');
  vscode.workspace.textDocuments = [{ isDirty: true, uri: makeUri(path.join(folder, 'lib', 'calculator.rb')) }];
  await assert.rejects(controller.taskAction('accept'), /unsaved edits/);
  assert.equal(fixture.applyCount, 0);
});

test('multi-root selection isolates task history and restores the previous task', async t => {
  const { controller, fixture, vscode, folder, makeUri } = await setup(t);
  const second = path.join(folder, 'second');
  await fs.mkdir(second);
  vscode.workspace.workspaceFolders.push({ name: 'second', uri: makeUri(second) });
  await controller.connect();
  await controller.createTask('First repository');
  await controller.selectFolder(makeUri(second).toString());
  assert.equal(controller.task, null);
  assert.equal(controller.history.length, 0);
  await controller.createTask('Second repository');
  assert.equal(controller.task.repository_id, 2);
  await controller.selectFolder(makeUri(folder).toString());
  assert.equal(controller.task.id, 1);
  assert.deepEqual(controller.history.map(task => task.id), [1]);
  assert.equal(fixture.repositories.length, 2);
});

test('webview applies CSP, local resources and no embedded task content', async t => {
  const { vscode, context } = await setup(t);
  const html = webviewHtml({ cspSource: 'vscode-webview:', asWebviewUri: uri => uri.toString() }, vscode, context.extensionUri);
  assert.match(html, /default-src 'none'/);
  assert.match(html, /script-src 'nonce-/);
  assert.doesNotMatch(html, /unsafe-inline|https:\/\//);
  const frontend = await fs.readFile(path.join(__dirname, '../media/sidebar.js'), 'utf8');
  assert.doesNotMatch(frontend, /\.innerHTML|eval\(|insertAdjacentHTML/);
});

test('explicit RSpec request runs the nested project directly without Git, API or model', async t => {
  const { controller, fixture, folder, executions } = await setup(t);
  const server = path.join(folder, 'server');
  await fs.mkdir(path.join(server, 'spec'), { recursive: true });
  await fs.writeFile(path.join(server, 'Gemfile'), 'source "https://rubygems.org"\n');
  await controller.message({ type: 'create', request: 'run rspec of this project' });
  assert.equal(controller.error, '');
  assert.equal(fixture.requests.length, 0);
  assert.equal(executions.length, 1);
  assert.equal(executions[0].task.execution.command, 'bundle');
  assert.deepEqual(executions[0].task.execution.args, ['exec', 'rspec']);
  assert.deepEqual(executions[0].task.execution.options, { cwd: await fs.realpath(server), env: { RAILS_ENV: 'test', RACK_ENV: 'test' } });
  assert.equal(controller.localTest.state, 'running');
  controller.finishLocalRSpec({ execution: executions[0], exitCode: 1 });
  assert.equal(controller.localTest.state, 'completed');
  assert.equal(controller.localTest.exitCode, 1);
  assert.equal(controller.localTestExecution, null);
});

test('local RSpec enforces trust/saved buffers and cancels only the active execution', async t => {
  const { controller, fixture, vscode, folder, makeUri, executions } = await setup(t);
  vscode.workspace.isTrusted = false;
  await assert.rejects(controller.createTask('run full rspec'), /Trust/);
  vscode.workspace.isTrusted = true;
  vscode.workspace.textDocuments = [{ isDirty: true, uri: makeUri(path.join(folder, 'unsaved.rb')) }];
  await assert.rejects(controller.createTask('run full rspec'), /unsaved/);
  vscode.workspace.textDocuments = [];
  await controller.createTask('please run full rspec in this folder');
  assert.throws(() => controller.cancelLocalRSpec('old-run'), /no longer active/);
  controller.cancelLocalRSpec(controller.localTest.id);
  assert.equal(executions[0].terminated, true);
  controller.finishLocalRSpec({ execution: executions[0], exitCode: 130 });
  assert.equal(controller.localTest.state, 'cancelled');
  assert.equal(fixture.requests.length, 0);
});

test('retrying an old failed test-only API task uses local RSpec and preserves original record', async t => {
  const { controller, fixture, executions } = await setup(t);
  controller.task = { id: 41, state: 'failed', user_request: 'run rspec of this project' };
  await controller.message({ type: 'action', action: 'retry', taskId: 41 });
  assert.equal(controller.error, '');
  assert.equal(executions.length, 1);
  assert.equal(fixture.requests.length, 0);
  assert.equal(controller.task.state, 'failed');
});
