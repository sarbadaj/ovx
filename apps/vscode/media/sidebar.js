'use strict';

(() => {
  const vscode = acquireVsCodeApi();
  const element = id => document.getElementById(id);
  let state = {};
  const prompt = element('prompt');
  prompt.value = vscode.getState()?.prompt || '';
  prompt.addEventListener('input', () => vscode.setState({ prompt: prompt.value }));

  function send(type, data = {}) { vscode.postMessage({ type, ...data }); }
  function node(tag, text, className) {
    const value = document.createElement(tag);
    if (text !== undefined) value.textContent = String(text);
    if (className) value.className = className;
    return value;
  }
  function button(text, message, className = 'secondary') {
    const value = node('button', text, className);
    value.type = 'button';
    value.disabled = state.busy || !state.trusted;
    value.addEventListener('click', () => send(message.type, message));
    return value;
  }
  function text(id, value) { element(id).textContent = value || ''; }
  function show(id, visible) { element(id).hidden = !visible; }
  function pretty(value) { return typeof value === 'string' ? value : JSON.stringify(value, null, 2); }
  function details(key, title, output, opened) {
    const wrapper = node('details');
    wrapper.dataset.key = key;
    wrapper.open = opened.has(key);
    wrapper.append(node('summary', title), node('pre', output || 'No output recorded.'));
    return wrapper;
  }
  function options(select, values, selected, placeholder) {
    const signature = JSON.stringify([values, selected, placeholder]);
    if (select.dataset.signature === signature) return;
    select.dataset.signature = signature;
    select.replaceChildren();
    if (!values.length) select.append(new Option(placeholder, ''));
    for (const value of values) select.append(new Option(value.text, String(value.value)));
    if (selected) select.value = String(selected);
  }

  function render() {
    const opened = new Set(Array.from(document.querySelectorAll('details[open]')).map(value => value.dataset.key));
    text('connection', state.connected ? `Connected · ${state.serviceUrl}` : `Disconnected · ${state.serviceUrl || 'local service'}`);
    element('connection').className = state.connected ? 'connected' : 'muted';
    text('notice', !state.trusted ? 'Trust this workspace in VS Code to run OVX.' : !state.folders?.length ? 'Open a local project folder to begin.' : !state.connected ? 'Start the local OVX service, then connect to begin.' : '');
    show('notice', !state.trusted || !state.folders?.length || !state.connected);
    text('error', state.error);
    show('error', Boolean(state.error));
    const folders = state.folders || [];
    options(element('folder'), folders.map(folder => ({ value: folder.uri, text: folder.name })), state.selectedFolder, 'No local folder open');
    text('folderPath', folders.find(folder => folder.uri === state.selectedFolder)?.path);
    element('folder').disabled = state.busy || folders.length < 2;
    element('submit').disabled = state.busy || !state.trusted || !folders.length;
    element('submit').textContent = state.busy ? 'Sending…' : 'Send ↗';
    for (const control of document.querySelectorAll('[data-message]')) control.disabled = state.busy || (!state.trusted && control.dataset.message !== 'logs');
    options(element('history'), (state.history || []).map(task => ({ value: task.id, text: `#${task.id} · ${task.resolution || task.state} · ${task.user_request}` })), state.task?.id, 'No tasks yet');
    element('history').disabled = state.busy || !(state.history || []).length;
    const localTest = state.localTest;
    show('localTest', Boolean(localTest));
    if (localTest) {
      text('localTestState', localTest.state.replaceAll('_', ' '));
      text('localTestCommand', localTest.command);
      text('localTestPath', `Working directory: ${localTest.folderPath}`);
      text('localTestResult', localTest.error || (localTest.exitCode !== null ? `${localTest.exitCode === 0 ? 'PASS' : 'FAIL'} · exit code ${localTest.exitCode}` : localTest.state === 'cancelled' ? 'Run cancelled.' : 'Running in the integrated terminal…'));
      const localActions = element('localTestActions');
      localActions.replaceChildren();
      if (['starting', 'running', 'cancelling'].includes(localTest.state)) localActions.append(button('Cancel RSpec', { type: 'cancelRSpec', runId: localTest.id }));
      else localActions.append(button('Run full RSpec again', { type: 'runRSpec' }));
    }
    const task = state.task;
    show('task', Boolean(task));
    show('emptyState', !task);
    if (!task) return;
    text('taskTitle', `· task #${task.id}`);
    text('taskState', task.resolution || task.state.replaceAll('_', ' '));
    text('workingMessage', task.final_message ? '' : task.error_message ? 'OVX stopped while working on this request.' : task.state === 'awaiting_approval' ? 'Waiting for your approval to continue.' : ['planning', 'executing', 'created'].includes(task.state) ? 'Working on your request… Activity updates appear below.' : '');
    text('request', task.user_request);
    text('revision', task.base_revision ? `Base commit ${task.base_revision.slice(0, 12)}` : '');
    text('taskError', task.error_message);
    show('taskError', Boolean(task.error_message));
    const controls = element('taskActions');
    controls.replaceChildren();
    for (const [action, label] of [['accept', 'Accept changes'], ['reject', 'Reject changes'], ['retry', task.state === 'recovery_required' ? 'Resume safely' : ['completed', 'cancelled'].includes(task.state) ? 'Run again' : 'Retry'], ['cancel', 'Cancel task']]) {
      if (task.actions[action]) controls.append(button(label, { type: 'action', action, taskId: task.id }, action === 'accept' ? '' : 'secondary'));
    }
    const approvals = element('approvals');
    approvals.replaceChildren();
    for (const approval of task.approvals) {
      const card = node('div', undefined, 'approval');
      card.append(node('h2', 'Approval required'), node('p', approval.reason), node('pre', approval.tool?.arguments?.command || pretty(approval.tool?.arguments || {})), node('p', (approval.capabilities || []).join(' · '), 'muted'));
      const actions = node('div', undefined, 'toolbar');
      actions.append(button('Allow once', { type: 'approval', id: approval.id, decision: 'approve', taskId: task.id }, ''), button('Deny', { type: 'approval', id: approval.id, decision: 'deny', taskId: task.id }));
      card.append(actions);
      approvals.append(card);
    }
    text('plan', task.plan);
    show('planSection', Boolean(task.plan));
    text('result', task.final_message);
    show('resultSection', Boolean(task.final_message));
    text('activityCount', `· ${task.events.length} recent events`);
    text('changeCount', `· ${(state.files || []).length} files`);
    text('testCount', `· ${task.tests.length} runs`);
    const files = element('files');
    files.replaceChildren();
    for (const file of state.files || []) {
      const control = button(`${file.status}  ${file.path}${file.binary ? ' (binary)' : ''}`, { type: 'diff', index: file.index, taskId: task.id }, 'file');
      control.title = `Open before / after diff for ${file.path}`;
      files.append(control);
    }
    text('diffNotice', state.diffError || (task.resolution === 'rejected' ? 'Result rejected and task workspace removed.' : state.files?.length ? 'Select a file to review its before / after diff.' : ['completed', 'failed', 'cancelled', 'recovery_required'].includes(task.state) ? 'No file changes.' : 'Changes will appear when execution stops.'));
    const tests = element('tests');
    tests.replaceChildren();
    if (!task.tests.length) tests.append(node('p', 'No test execution recorded yet.', 'muted'));
    for (const test of task.tests) {
      const result = test.result || {};
      const status = test.state !== 'completed' ? test.state : test.exit_code === 0 ? 'PASS' : `FAIL (exit ${test.exit_code ?? '?'})`;
      const duration = test.started_at && test.finished_at ? ` · ${Math.max(0, (new Date(test.finished_at) - new Date(test.started_at)) / 1000).toFixed(1)}s` : '';
      const output = test.error || (result.stdout !== undefined || result.stderr !== undefined ? [result.stdout, result.stderr].filter(Boolean).join('\n') : pretty(result));
      const record = details(`test-${test.id}`, `${status}${duration} · ${test.command}`, output, opened);
      record.className = test.exit_code === 0 ? 'passed' : 'test';
      tests.append(record);
    }
    const events = element('events');
    events.replaceChildren();
    for (const event of [...task.events].reverse()) {
      const time = event.created_at ? new Date(event.created_at).toLocaleTimeString() : '';
      const payload = Object.fromEntries(Object.entries(event.payload || {}).filter(([, value]) => value !== undefined && value !== null));
      const subject = payload.tool || payload.command || payload.path || '';
      events.append(details(`event-${event.id}`, `${time}  ${event.event_type.replaceAll('_', ' ')}${subject ? ` · ${subject}` : ''}`, pretty(payload), opened));
    }
  }

  document.querySelectorAll('[data-message]').forEach(control => control.addEventListener('click', () => send(control.dataset.message)));
  element('folder').addEventListener('change', event => send('folder', { uri: event.target.value }));
  element('history').addEventListener('change', event => { if (event.target.value) send('selectTask', { id: Number(event.target.value) }); });
  function submit() { if (!element('submit').disabled && prompt.value.trim()) send('create', { request: prompt.value }); }
  element('promptForm').addEventListener('submit', event => { event.preventDefault(); submit(); });
  prompt.addEventListener('keydown', event => { if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) { event.preventDefault(); submit(); } });
  window.addEventListener('message', event => {
    if (event.data.type === 'state') { state = event.data; render(); }
    if (event.data.type === 'submitted') { prompt.value = ''; vscode.setState({ prompt: '' }); }
  });
  send('ready');
})();
