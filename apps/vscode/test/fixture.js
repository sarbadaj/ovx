'use strict';

const http = require('node:http');

async function startFixture() {
  const fixture = { token: 'test-token-only', repositories: [], tasks: [], requests: [], applyCount: 0 };
  const server = http.createServer(async (request, response) => {
    const url = new URL(request.url, 'http://localhost');
    let body = '';
    for await (const chunk of request) body += chunk;
    const data = body ? JSON.parse(body) : {};
    fixture.requests.push({ method: request.method, path: url.pathname, query: url.search, data });
    const send = (status, value) => { response.writeHead(status, { 'Content-Type': 'application/json' }); response.end(JSON.stringify(value)); };
    if (url.pathname === '/health') return send(200, { status: 'ok' });
    if (request.headers.authorization !== `Bearer ${fixture.token}`) return send(401, { error: 'unauthorized' });
    if (url.pathname === '/api/repositories') {
      if (request.method === 'GET') return send(200, fixture.repositories);
      const repository = { id: fixture.repositories.length + 1, ...data };
      fixture.repositories.push(repository);
      return send(201, repository);
    }
    if (url.pathname === '/api/tasks') {
      if (request.method === 'GET') return send(200, fixture.tasks.filter(task => task.repository_id === Number(url.searchParams.get('repository_id'))).toReversed());
      const task = {
        id: fixture.tasks.length + 1, repository_id: data.repository_id, user_request: data.request,
        state: 'completed', resolution: null, workspace_path: '/tmp/ovx-fixture-task', base_revision: 'abcdef1234567890',
        plan: 'Read calculator\nFix addition\nRun tests and inspect diff', final_message: 'Addition fixed. Tests pass.',
        created_at: '2026-09-27T00:00:00Z', agent_events: [{ id: 1, event_type: 'task_completed', payload: { message: 'Done' }, created_at: '2026-09-27T00:00:01Z' }],
        tool_calls: [{ id: 1, tool_name: 'run_tests', arguments: { command: 'ruby test/calculator_test.rb' }, tool_attempts: [{ id: 1, state: 'completed', exit_code: 0, result: { stdout: '1 test, 0 failures', stderr: '' } }] }],
        approvals: []
      };
      fixture.tasks.push(task);
      return send(202, task);
    }
    const taskRoute = url.pathname.match(/^\/api\/tasks\/(\d+)(?:\/(\w+))?$/);
    if (taskRoute) {
      const task = fixture.tasks.find(task => task.id === Number(taskRoute[1]));
      if (!task) return send(404, { error: 'not found' });
      switch (taskRoute[2]) {
        case undefined: return send(200, task);
        case 'diff': return send(200, { patch_sha256: 'a'.repeat(64), diff: '- a - b\n+ a + b', files: [{ path: 'lib/calculator.rb', status: 'modified', before: 'a - b\n', after: 'a + b\n' }] });
        case 'apply_result':
          if (task.state !== 'completed' || task.resolution) return send(409, { error: 'invalid decision' });
          if (data.expected_patch_sha256 !== 'a'.repeat(64)) return send(409, { error: 'reviewed patch changed' });
          fixture.applyCount++;
          task.resolution = 'accepted';
          return send(200, task);
        case 'reject_result': task.resolution = 'rejected'; return send(200, task);
        case 'cancel': task.state = 'cancelled'; return send(200, task);
        case 'retry': task.state = 'completed'; task.error_message = null; return send(202, task);
      }
    }
    const approvalRoute = url.pathname.match(/^\/api\/approvals\/(\d+)\/(approve|deny)$/);
    if (approvalRoute) {
      const task = fixture.tasks.find(task => task.approvals.some(approval => approval.id === Number(approvalRoute[1])));
      if (!task) return send(404, { error: 'not found' });
      const approval = task.approvals.find(approval => approval.id === Number(approvalRoute[1]));
      approval.status = approvalRoute[2] === 'approve' ? 'approved' : 'denied';
      task.state = approval.status === 'approved' ? 'completed' : 'failed';
      return send(200, approval);
    }
    send(404, { error: 'not found' });
  });
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  fixture.url = `http://127.0.0.1:${server.address().port}`;
  fixture.close = () => new Promise(resolve => { server.close(resolve); server.closeAllConnections(); });
  return fixture;
}

module.exports = { startFixture };
