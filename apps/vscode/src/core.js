'use strict';

const path = require('node:path');

function serviceUrl(value) {
  let url;
  try { url = new URL(value); } catch { throw new Error('OVX service URL must be an HTTP(S) loopback URL.'); }
  if (!['http:', 'https:'].includes(url.protocol) || !['127.0.0.1', '[::1]', 'localhost'].includes(url.hostname) ||
      url.username || url.password || url.search || url.hash || !['', '/'].includes(url.pathname)) {
    throw new Error('Use a loopback service URL such as http://127.0.0.1:3000, without credentials, paths, or query parameters.');
  }
  return url.origin;
}

function id(value) {
  const result = Number(value);
  if (!Number.isSafeInteger(result) || result < 1) throw new Error('Invalid task or approval ID.');
  return result;
}

function actions(task) {
  if (!task) return { accept: false, reject: false, retry: false, cancel: false, diff: false };
  const completed = task.state === 'completed';
  return {
    accept: completed && !task.resolution,
    reject: completed && !task.resolution,
    retry: ['failed', 'recovery_required', 'completed', 'cancelled'].includes(task.state),
    cancel: !['completed', 'failed', 'cancelled'].includes(task.state),
    diff: Boolean(task.workspace_path) && task.resolution !== 'rejected'
  };
}

function tokenFromEnv(text) {
  const line = text.split(/\r?\n/).find(value => /^\s*(?:export\s+)?OVX_API_TOKEN\s*=/.test(value));
  if (!line) throw new Error('OVX_API_TOKEN was not found in this installation’s .env. Run bin/setup or set the token manually.');
  let token = line.replace(/^\s*(?:export\s+)?OVX_API_TOKEN\s*=\s*/, '').trim();
  if (token.startsWith('"') || token.startsWith("'")) {
    const quote = token[0];
    const end = token.indexOf(quote, 1);
    if (end < 0 || !/^\s*(?:#.*)?$/.test(token.slice(end + 1))) throw new Error('OVX_API_TOKEN has invalid quoting.');
    token = token.slice(1, end);
  } else {
    token = token.replace(/\s+#.*$/, '').trim();
  }
  return validateToken(token);
}

function validateToken(token) {
  if (typeof token !== 'string' || !token.trim() || /[\r\n\u0000]/.test(token) || token.length > 8192) {
    throw new Error('Enter a nonempty API token on one line.');
  }
  return token.trim();
}

function isWithin(folder, file) {
  const relative = path.relative(folder, file);
  return relative === '' || (!relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative));
}

function tests(task) {
  return (task?.tool_calls || []).filter(call => call.tool_name === 'run_tests').flatMap(call =>
    (call.tool_attempts || []).map(attempt => ({
      id: attempt.id,
      command: call.arguments?.command || 'Tests',
      state: attempt.state,
      exit_code: attempt.exit_code,
      started_at: attempt.started_at,
      finished_at: attempt.finished_at,
      result: attempt.result,
      error: attempt.error_message
    })));
}

function publicTask(task) {
  if (!task) return null;
  return {
    id: task.id, repository_id: task.repository_id, user_request: task.user_request,
    state: task.state, resolution: task.resolution, plan: task.plan,
    final_message: task.final_message, error_message: task.error_message,
    base_revision: task.base_revision, created_at: task.created_at,
    actions: actions(task), tests: tests(task),
    // Tool output can contain whole source files. Keep the sidebar responsive
    // while retaining the full event history in the service for diagnostics.
    events: (task.agent_events || []).slice(-80).map(event => ({
      id: event.id, event_type: event.event_type, created_at: event.created_at,
      payload: {
        step: event.payload?.step,
        tool: event.payload?.tool,
        state: event.payload?.state,
        reason: event.payload?.reason,
        message: event.payload?.message?.slice?.(0, 500),
        error: event.payload?.error?.slice?.(0, 500),
        command: event.payload?.arguments?.command,
        path: event.payload?.arguments?.path
      }
    })),
    approvals: (task.approvals || []).filter(approval => approval.status === 'pending').map(approval => ({
      id: approval.id, reason: approval.reason, capabilities: approval.requested_capabilities,
      tool: (task.tool_calls || []).find(call => call.id === approval.tool_call_id)
    }))
  };
}

module.exports = { serviceUrl, id, actions, tokenFromEnv, validateToken, isWithin, tests, publicTask };
