'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const vscode = require('vscode');
const { startFixture } = require('./fixture');

async function waitFor(condition, message) {
  const deadline = Date.now() + 10000;
  while (!condition()) {
    if (Date.now() > deadline) throw new Error(message);
    await new Promise(resolve => setTimeout(resolve, 50));
  }
}

async function run() {
  const checks = [];
  let fixture;
  let controller;
  let failure;
  try {
    assert.ok(vscode.workspace.workspaceFolders?.length, 'Launch this test with a disposable local folder argument.');
    assert.equal(vscode.workspace.isTrusted, true, 'Use --disable-workspace-trust in the isolated test profile.');
    fixture = await startFixture();
    await vscode.workspace.getConfiguration('ovx').update('serviceUrl', fixture.url, vscode.ConfigurationTarget.Global);
    const extension = vscode.extensions.getExtension('ompirit.ovx-local');
    assert.ok(extension, 'OVX development extension must be installed in the extension host.');
    controller = await extension.activate();
    checks.push('Extension activates in the real VS Code extension host');
    await controller.context.secrets.store(`ovx.apiToken:${fixture.url}`, fixture.token);
    await vscode.commands.executeCommand('ovx.connect');
    assert.equal(controller.connected, true, controller.error);
    checks.push('Connect command reads SecretStorage and authenticates to local API');
    await vscode.commands.executeCommand('ovx.open');
    await waitFor(() => controller.view, 'OVX sidebar view was not resolved.');
    assert.ok(controller.view.webview.html.includes('Content-Security-Policy'));
    checks.push('Activity bar command resolves the protected sidebar webview');
    await controller.message({ type: 'create', request: 'Fix the failing calculator test' });
    assert.equal(controller.error, '');
    assert.equal(controller.task.state, 'completed');
    assert.equal(controller.history.length, 1);
    checks.push('Prompt creates a task and refreshes persisted history, events and test output');
    await controller.message({ type: 'diff', index: 0, taskId: controller.task.id });
    assert.equal(controller.error, '');
    await waitFor(() => vscode.workspace.textDocuments.filter(document => document.uri.scheme === 'ovx-diff').length >= 2, 'Native diff documents did not open.');
    const snapshots = vscode.workspace.textDocuments.filter(document => document.uri.scheme === 'ovx-diff').map(document => document.getText());
    assert.ok(snapshots.includes('a - b\n'));
    assert.ok(snapshots.includes('a + b\n'));
    checks.push('Native VS Code diff editor opens immutable before and after documents');
    await controller.message({ type: 'action', action: 'accept', taskId: controller.task.id });
    assert.equal(controller.task.resolution, 'accepted');
    assert.equal(fixture.applyCount, 1);
    checks.push('Accept records the patch decision exactly once');
    await controller.message({ type: 'action', action: 'retry', taskId: controller.task.id });
    assert.equal(controller.task.id, 2);
    await controller.message({ type: 'action', action: 'reject', taskId: controller.task.id });
    assert.equal(controller.task.resolution, 'rejected');
    checks.push('Run again creates a fresh task and rejection clears its changes');
    console.log(`OVX EXTENSION HOST TEST PASSED (${checks.length} checks)`);
  } catch (error) {
    failure = error;
    console.error(error);
    throw error;
  } finally {
    if (controller) {
      clearTimeout(controller.timer);
      controller.client?.dispose();
    }
    await fixture?.close();
    if (process.env.OVX_EXTENSION_TEST_REPORT) await fs.writeFile(process.env.OVX_EXTENSION_TEST_REPORT, JSON.stringify({ passed: !failure, checks, error: failure?.stack, vscode: vscode.version }, null, 2));
  }
}

module.exports = { run };
