'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { serviceUrl, id, actions, tokenFromEnv, isWithin, publicTask } = require('../src/core');

test('service credentials are confined to loopback origin', () => {
  for (const url of ['http://127.0.0.1:3000', 'http://localhost:3000/', 'https://[::1]:3000']) assert.ok(serviceUrl(url));
  for (const url of ['https://example.com', 'http://127.0.0.1.evil.test', 'file:///tmp/api', 'http://user:secret@localhost:3000', 'http://localhost:3000/api', 'http://localhost:3000?token=x', 'http://localhost:3000#x', 'http://0.0.0.0:3000']) assert.throws(() => serviceUrl(url));
});

test('token import parses data without evaluating shell expressions', () => {
  assert.equal(tokenFromEnv('OTHER=x\nexport OVX_API_TOKEN="long-token" # comment'), 'long-token');
  assert.equal(tokenFromEnv("OVX_API_TOKEN='literal$(touch /tmp/unwanted)'"), 'literal$(touch /tmp/unwanted)');
  assert.equal(tokenFromEnv('OVX_API_TOKEN=token # comment'), 'token');
  assert.throws(() => tokenFromEnv('OVX_RUNTIME_TOKEN=wrong-token'));
  assert.throws(() => tokenFromEnv('OVX_API_TOKEN="bad'));
  assert.throws(() => tokenFromEnv('OVX_API_TOKEN='));
});

test('task decisions and repository containment respect state and path boundaries', () => {
  assert.equal(actions({ state: 'completed', resolution: null }).accept, true);
  assert.equal(actions({ state: 'completed', resolution: 'accepted' }).accept, false);
  assert.equal(actions({ state: 'executing' }).retry, false);
  assert.equal(actions({ state: 'recovery_required' }).retry, true);
  assert.equal(actions({ state: 'awaiting_approval' }).cancel, true);
  assert.ok(isWithin('/tmp/project', '/tmp/project/lib/x.rb'));
  assert.equal(isWithin('/tmp/project', '/tmp/project-other/x.rb'), false);
  assert.equal(isWithin('/tmp/project', '/tmp/project/../secret'), false);
  for (const value of ['1/accept', -1, 0, NaN, Number.MAX_SAFE_INTEGER + 1]) assert.throws(() => id(value));
});

test('webview projection includes approval command and tests, omits transcript', () => {
  const task = publicTask({ id: 1, state: 'awaiting_approval', transcript: ['private'],
    tool_calls: [{ id: 2, tool_name: 'run_command', arguments: { command: 'bundle exec rubocop' } }, { id: 3, tool_name: 'run_tests', arguments: { command: 'bundle exec rspec' }, tool_attempts: [{ id: 4, state: 'completed', exit_code: 0, result: { stdout: 'passed' } }] }],
    approvals: [{ id: 5, tool_call_id: 2, status: 'pending' }, { id: 6, status: 'approved' }]
  });
  assert.equal(task.transcript, undefined);
  assert.equal(task.approvals.length, 1);
  assert.equal(task.approvals[0].tool.arguments.command, 'bundle exec rubocop');
  assert.equal(task.tests[0].command, 'bundle exec rspec');
  assert.equal(task.tests[0].exit_code, 0);
});
