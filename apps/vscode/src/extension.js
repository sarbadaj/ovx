'use strict';

const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');
const { Client } = require('./client');
const { serviceUrl, id, actions, tokenFromEnv, validateToken, isWithin, publicTask } = require('./core');
const { isRSpecRequest, rspecRoots } = require('./local-tests');

class Controller {
  constructor(vscode, context) {
    this.vscode = vscode;
    this.context = context;
    this.output = vscode.window.createOutputChannel('OVX');
    this.status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 20);
    this.status.command = 'ovx.open';
    this.status.show();
    this.documents = new Map();
    this.history = [];
    this.files = [];
    this.reviewedPatchHash = null;
    this.connected = false;
    this.busy = false;
    this.error = '';
    this.epoch = 0;
    this.disposed = false;
    this.selectedFolder = context.workspaceState.get('ovx.selectedFolder');
    this.selectedTaskId = null;
    this.task = null;
    this.repository = null;
    this.diffError = '';
    this.localTest = null;
    this.localTestExecution = null;
    this.updateStatus();
  }

  dispose() {
    this.disposed = true;
    clearTimeout(this.timer);
    this.client?.dispose();
    this.documents.clear();
    this.output.dispose();
    this.status.dispose();
  }

  setting(name, fallback) {
    // Machine-scoped settings must never execute a path or send a token chosen by repository settings.
    return this.vscode.workspace.getConfiguration('ovx').inspect(name)?.globalValue ?? fallback;
  }

  folders() {
    return (this.vscode.workspace.workspaceFolders || []).filter(folder => folder.uri.scheme === 'file');
  }

  folder() {
    const folders = this.folders();
    return folders.find(folder => folder.uri.toString() === this.selectedFolder) || folders[0];
  }

  trust() {
    if (!this.vscode.workspace.isTrusted) throw new Error('Trust this workspace in VS Code before using OVX.');
  }

  async run(operation) {
    if (this.busy) return;
    this.busy = true;
    this.error = '';
    this.epoch++;
    this.publish();
    try { return await operation(); }
    catch (error) {
      if (!this.disposed) {
        this.error = error.message || String(error);
        this.output.appendLine(`[${new Date().toISOString()}] ${this.error}`);
        this.vscode.window.showErrorMessage(this.error);
      }
    } finally { this.busy = false; this.publish(); }
  }

  async setToken() {
    this.trust();
    const token = await this.vscode.window.showInputBox({
      title: 'OVX API token', prompt: 'Paste OVX_API_TOKEN from the OVX installation’s .env. Stored in VS Code SecretStorage.',
      password: true, ignoreFocusOut: true,
      validateInput: value => { try { validateToken(value); return undefined; } catch (error) { return error.message; } }
    });
    if (token === undefined) return;
    const url = serviceUrl(this.setting('serviceUrl', 'http://127.0.0.1:3000'));
    await this.context.secrets.store(`ovx.apiToken:${url}`, validateToken(token));
    await this.connect();
  }

  async chooseInstallation() {
    this.trust();
    const selected = await this.vscode.window.showOpenDialog({ canSelectFiles: false, canSelectFolders: true, canSelectMany: false, title: 'Select the OVX installation containing bin/ovx' });
    if (!selected?.[0]) return null;
    const directory = selected[0].fsPath;
    await this.validateInstallation(directory);
    await this.vscode.workspace.getConfiguration('ovx').update('installationPath', directory, this.vscode.ConfigurationTarget.Global);
    return directory;
  }

  async validateInstallation(directory) {
    if (!directory || !path.isAbsolute(directory)) throw new Error('Choose the absolute OVX installation directory with “OVX: Choose OVX Installation”.');
    const stat = await fs.stat(path.join(directory, 'bin', 'ovx')).catch(() => null);
    if (!stat?.isFile()) throw new Error('This directory does not contain bin/ovx. Choose the OVX installation directory.');
    return directory;
  }

  async installation() {
    const directory = this.setting('installationPath', '') || await this.chooseInstallation();
    if (!directory) return null;
    return this.validateInstallation(directory);
  }

  async importToken() {
    this.trust();
    const directory = await this.installation();
    if (!directory) return;
    const text = await fs.readFile(path.join(directory, '.env'), 'utf8').catch(() => { throw new Error('Cannot read .env in the chosen OVX installation. Run bin/setup there first.'); });
    const token = tokenFromEnv(text);
    const url = serviceUrl(this.setting('serviceUrl', 'http://127.0.0.1:3000'));
    await this.context.secrets.store(`ovx.apiToken:${url}`, token);
    await this.connect();
  }

  async startService() {
    this.trust();
    const directory = await this.installation();
    if (!directory) return;
    this.output.show(true);
    this.output.appendLine(`Starting ${path.join(directory, 'bin', 'ovx')} start`);
    await new Promise((resolve, reject) => {
      // No shell interpolation: installation names containing spaces or shell syntax are safe.
      const child = spawn(path.join(directory, 'bin', 'ovx'), ['start'], { cwd: directory, shell: false, env: process.env });
      child.stdout.on('data', chunk => this.output.append(chunk.toString()));
      child.stderr.on('data', chunk => this.output.append(chunk.toString()));
      child.once('error', reject);
      child.once('close', code => code === 0 ? resolve() : reject(new Error(`OVX start exited with ${code}. See the OVX output channel; run bin/setup if prerequisites are missing.`)));
    });
    await this.connect();
  }

  async connect() {
    this.trust();
    const url = serviceUrl(this.setting('serviceUrl', 'http://127.0.0.1:3000'));
    const token = await this.context.secrets.get(`ovx.apiToken:${url}`);
    if (!token) throw new Error('Configure your API token with “OVX: Set API Token” or “OVX: Import Token from OVX Installation”.');
    this.client?.dispose();
    this.client = new Client(url, token);
    this.connected = false;
    this.repository = null;
    this.history = [];
    this.task = null;
    this.files = [];
    this.reviewedPatchHash = null;
    this.diffVersion = null;
    this.diffError = '';
    await this.client.request('GET', '/health');
    const repositories = await this.client.request('GET', '/api/repositories');
    this.connected = true;
    this.error = '';
    const folder = this.folder();
    if (folder) {
      const realPath = await fs.realpath(folder.uri.fsPath);
      this.repository = repositories.find(repository => repository.path === realPath) || null;
      this.selectedFolder = folder.uri.toString();
      this.selectedTaskId = this.context.workspaceState.get(this.historyKey()) || null;
      if (this.repository) await this.refresh(true);
    }
    this.schedule();
    this.publish();
  }

  historyKey() { return `ovx.lastTask:${this.client?.url}:${this.selectedFolder}`; }

  async selectFolder(uri) {
    if (!this.folders().some(folder => folder.uri.toString() === uri)) throw new Error('Select an open local workspace folder.');
    this.selectedFolder = uri;
    this.selectedTaskId = null;
    this.task = null;
    this.files = [];
    this.repository = null;
    this.history = [];
    await this.context.workspaceState.update('ovx.selectedFolder', uri);
    if (this.connected) await this.connect();
  }

  async ensureRepository() {
    this.trust();
    if (!this.connected) await this.connect();
    const folder = this.folder();
    if (!folder) throw new Error('Open a local Git repository folder in VS Code first.');
    if (!this.repository) this.repository = await this.client.request('POST', '/api/repositories', { name: folder.name, path: await fs.realpath(folder.uri.fsPath) });
    return this.repository;
  }

  async createTask(request) {
    if (typeof request !== 'string' || !request.trim() || request.length > 20000) throw new Error('Enter a task between 1 and 20,000 characters.');
    if (isRSpecRequest(request)) return this.runLocalRSpec();
    const repository = await this.ensureRepository();
    this.assertSaved();
    try {
      const task = await this.client.request('POST', '/api/tasks', { repository_id: repository.id, request: request.trim() });
      await this.selectTask(task.id, task);
      this.view?.webview.postMessage({ type: 'submitted' });
      await this.refresh(true);
    } catch (error) {
      if (error.payload?.id) await this.selectTask(error.payload.id, error.payload);
      // A timeout can happen after the server accepted a task. Refresh durable history before resubmitting.
      try { await this.refresh(true); } catch { /* retain the original actionable error */ }
      throw error;
    }
  }

  assertSaved() {
    const folder = this.folder();
    const dirty = this.vscode.workspace.textDocuments.filter(document => document.isDirty && document.uri.scheme === 'file' && isWithin(folder.uri.fsPath, document.uri.fsPath));
    if (dirty.length) throw new Error('Save or discard unsaved edits in the selected repository before starting or accepting a task. OVX works with files on disk.');
  }

  async runLocalRSpec() {
    this.trust();
    const folder = this.folder();
    if (!folder) throw new Error('Open a local project folder in VS Code first.');
    if (this.localTest && ['starting', 'running', 'cancelling'].includes(this.localTest.state)) throw new Error('An OVX RSpec run is already active. Wait for it or cancel it first.');
    this.assertSaved();
    const roots = await rspecRoots(folder.uri.fsPath);
    let directory = roots[0] || folder.uri.fsPath;
    if (roots.length > 1) {
      const selected = await this.vscode.window.showQuickPick(roots.map(root => ({ label: path.relative(folder.uri.fsPath, root) || folder.name, description: root, directory: root })), {
        title: 'Run full RSpec', placeHolder: 'Choose the project containing Gemfile and spec'
      });
      if (!selected) return;
      directory = selected.directory;
    }
    const runId = crypto.randomBytes(12).toString('hex');
    this.localTest = { id: runId, command: 'bundle exec rspec', folderName: folder.name, folderPath: directory, state: 'starting', exitCode: null, startedAt: new Date().toISOString() };
    this.publish();
    const execution = new this.vscode.ShellExecution('bundle', ['exec', 'rspec'], { cwd: directory, env: { RAILS_ENV: 'test', RACK_ENV: 'test' } });
    const task = new this.vscode.Task({ type: 'ovx-rspec', ovxRunId: runId }, folder, 'Run full RSpec', 'OVX', execution, []);
    task.presentationOptions = { reveal: this.vscode.TaskRevealKind.Always, focus: false, panel: this.vscode.TaskPanelKind.Dedicated, clear: true, showReuseMessage: false };
    try {
      const running = await this.vscode.tasks.executeTask(task);
      if (this.localTest.id === runId && this.localTest.state === 'starting') {
        this.localTestExecution = running;
        this.localTest.state = 'running';
      }
      this.view?.webview.postMessage({ type: 'submitted' });
    } catch (error) {
      this.localTest.state = 'failed_to_start';
      this.localTest.error = error.message;
      this.localTest.finishedAt = new Date().toISOString();
      throw error;
    } finally { this.publish(); }
  }

  finishLocalRSpec(event, processEnded = true) {
    if (!this.localTest || event.execution.task.definition.ovxRunId !== this.localTest.id) return;
    if (!['starting', 'running', 'cancelling'].includes(this.localTest.state)) return;
    const cancelled = this.localTest.state === 'cancelling';
    this.localTest.state = cancelled ? 'cancelled' : processEnded && Number.isInteger(event.exitCode) ? 'completed' : 'ended';
    this.localTest.exitCode = processEnded && Number.isInteger(event.exitCode) ? event.exitCode : null;
    this.localTest.finishedAt = new Date().toISOString();
    if (!cancelled && this.localTest.exitCode === null) this.localTest.error = 'No process exit code was reported. Inspect the integrated terminal for the startup error.';
    this.localTestExecution = null;
    this.publish();
  }

  cancelLocalRSpec(runId = this.localTest?.id) {
    this.trust();
    if (!this.localTestExecution || this.localTest?.id !== runId) throw new Error('This RSpec run is no longer active.');
    this.localTest.state = 'cancelling';
    this.localTestExecution.terminate();
    this.publish();
  }

  async selectTask(taskId, supplied) {
    const numericId = id(taskId);
    if (!supplied && !this.history.some(task => task.id === numericId)) throw new Error('Choose a task from this repository’s history.');
    this.selectedTaskId = numericId;
    this.files = [];
    this.reviewedPatchHash = null;
    this.diffError = '';
    this.diffVersion = null;
    this.task = supplied || await this.client.request('GET', `/api/tasks/${numericId}`);
    await this.context.workspaceState.update(this.historyKey(), numericId);
    await this.loadDiff();
    this.publish();
  }

  async taskAction(action) {
    this.trust();
    const task = this.task;
    if (!task || !actions(task)[action]) throw new Error('This action is unavailable for the current task state. Refresh the task.');
    const taskId = id(task.id);
    if (action === 'retry' && isRSpecRequest(task.user_request)) return this.runLocalRSpec();
    if (action === 'retry' && ['completed', 'cancelled'].includes(task.state)) return this.createTask(task.user_request);
    if (action === 'accept') {
      this.assertSaved();
      if (!this.reviewedPatchHash) {
        this.diffVersion = null;
        await this.loadDiff();
        throw new Error(this.diffError || 'Review the loaded changes, then choose Accept changes again.');
      }
    }
    const endpoint = { accept: 'apply_result', reject: 'reject_result', retry: 'retry', cancel: 'cancel' }[action];
    if (!endpoint) throw new Error('Unsupported task action.');
    const payload = action === 'accept' ? { expected_patch_sha256: this.reviewedPatchHash } : {};
    this.task = await this.client.request('POST', `/api/tasks/${taskId}/${endpoint}`, payload);
    this.diffVersion = null;
    if (action === 'accept') this.vscode.window.showInformationMessage('OVX changes accepted into your repository. Review and commit them with your normal Git workflow.');
    await this.refresh(true);
  }

  async decideApproval(approvalId, decision) {
    this.trust();
    const numericId = id(approvalId);
    if (!['approve', 'deny'].includes(decision) || !this.task?.approvals?.some(approval => approval.id === numericId && approval.status === 'pending')) {
      throw new Error('This approval is no longer pending. Refresh the task.');
    }
    await this.client.request('POST', `/api/approvals/${numericId}/${decision}`, {});
    await this.refresh(true);
  }

  async refresh(force = false) {
    if (!this.connected || !this.repository || this.refreshing || (this.busy && !force)) return;
    this.trust();
    this.refreshing = true;
    const epoch = this.epoch;
    const taskId = this.selectedTaskId;
    const repositoryId = this.repository.id;
    try {
      const history = await this.client.request('GET', `/api/tasks?repository_id=${id(repositoryId)}`);
      if (epoch !== this.epoch || repositoryId !== this.repository?.id) return;
      this.history = Array.isArray(history) ? history : history.tasks || [];
      if (taskId && !this.history.some(task => task.id === taskId)) {
        // The history API returns a bounded page. A saved task can still be fetched directly.
        const task = await this.client.request('GET', `/api/tasks/${id(taskId)}`);
        if (epoch !== this.epoch || taskId !== this.selectedTaskId) return;
        if (task.repository_id !== repositoryId) throw new Error('Stored task belongs to a different repository. Select a task from history.');
        this.task = task;
      } else if (taskId) {
        const task = await this.client.request('GET', `/api/tasks/${id(taskId)}`);
        if (epoch !== this.epoch || taskId !== this.selectedTaskId) return;
        this.task = task;
      }
      if (!this.task && this.history.length && !taskId) await this.selectTask(this.history[0].id, await this.client.request('GET', `/api/tasks/${id(this.history[0].id)}`));
      else await this.loadDiff(epoch);
      this.error = '';
    } finally { this.refreshing = false; this.publish(); }
  }

  async loadDiff(epoch = this.epoch) {
    const task = this.task;
    if (!task || !actions(task).diff) { this.files = []; this.reviewedPatchHash = null; return; }
    // Snapshot completed/interrupted tasks. Never refresh a document while it is open for review.
    if (!['completed', 'failed', 'cancelled', 'recovery_required'].includes(task.state)) return;
    const version = `${task.id}:${task.state}:${task.resolution || ''}:${task.updated_at || ''}`;
    if (this.diffVersion === version) return;
    try {
      const review = await this.client.request('GET', `/api/tasks/${id(task.id)}/diff`);
      if (epoch !== this.epoch || task.id !== this.task?.id) return;
      this.files = Array.isArray(review.files) ? review.files : [];
      this.reviewedPatchHash = /^[a-f0-9]{64}$/.test(review.patch_sha256 || '') ? review.patch_sha256 : null;
      this.rawDiff = review.diff || '';
      this.diffVersion = version;
      this.diffError = '';
    } catch (error) {
      this.files = [];
      this.reviewedPatchHash = null;
      this.diffError = error.message;
    }
  }

  async openDiff(index) {
    this.trust();
    if (!Number.isInteger(index) || !this.files[index]) throw new Error('Choose a file from the current task’s changes.');
    const file = this.files[index];
    if (file.binary) throw new Error('Binary changes cannot be displayed as a text diff. Inspect the task workspace before accepting.');
    const snapshot = crypto.randomBytes(8).toString('hex');
    const makeUri = side => this.vscode.Uri.from({ scheme: 'ovx-diff', authority: snapshot, path: `/${this.task.id}/${side}/${file.path}` });
    const before = makeUri('before');
    const after = makeUri('after');
    this.documents.set(before.toString(), file.before ?? '');
    this.documents.set(after.toString(), file.after ?? '');
    await this.vscode.commands.executeCommand('vscode.diff', before, after, `${file.path} — OVX task #${this.task.id}`, { preview: true });
  }

  schedule() {
    clearTimeout(this.timer);
    if (this.disposed) return;
    const interval = Math.max(1000, Math.min(30000, Number(this.setting('pollIntervalMs', 1500)) || 1500));
    this.timer = setTimeout(async () => {
      try { await this.refresh(); }
      catch (error) { this.error = error.message; this.publish(); }
      this.schedule();
    }, interval);
    this.timer.unref?.();
  }

  updateStatus() {
    this.status.text = this.connected ? `$(plug) OVX${this.task ? `: ${this.task.state}` : ': connected'}` : '$(debug-disconnect) OVX';
    this.status.tooltip = this.error || 'Open the local OVX coding agent';
    if (this.localTest && ['starting', 'running', 'cancelling'].includes(this.localTest.state)) this.status.text = '$(beaker) OVX: running project RSpec';
  }

  publish() {
    if (this.disposed) return;
    this.updateStatus();
    this.view?.webview.postMessage({
      type: 'state', trusted: this.vscode.workspace.isTrusted, connected: this.connected, busy: this.busy,
      localTest: this.localTest,
      serviceUrl: this.setting('serviceUrl', 'http://127.0.0.1:3000'), error: this.error,
      folders: this.folders().map(folder => ({ uri: folder.uri.toString(), name: folder.name, path: folder.uri.fsPath })),
      selectedFolder: this.folder()?.uri.toString(), repository: this.repository,
      history: this.history.map(task => ({ id: task.id, state: task.state, resolution: task.resolution, user_request: task.user_request, created_at: task.created_at })),
      task: publicTask(this.task), files: this.files.map((file, index) => ({ index, path: file.path, status: file.status, binary: file.binary })),
      diffError: this.diffError || ''
    });
  }

  async message(message) {
    if (!message || typeof message !== 'object') return;
    if (message.type === 'ready') { this.publish(); return; }
    await this.run(async () => {
      if (['action', 'approval', 'diff'].includes(message.type) && message.taskId !== this.task?.id) {
        throw new Error('The selected task changed. Review the current task before choosing an action.');
      }
      switch (message.type) {
        case 'connect': return this.connect();
        case 'setToken': return this.setToken();
        case 'importToken': return this.importToken();
        case 'startService': return this.startService();
        case 'folder': return this.selectFolder(message.uri);
        case 'create': return this.createTask(message.request);
        case 'runRSpec': return this.runLocalRSpec();
        case 'cancelRSpec': return this.cancelLocalRSpec(message.runId);
        case 'selectTask': return this.selectTask(message.id);
        case 'action': return this.taskAction(message.action);
        case 'approval': return this.decideApproval(message.id, message.decision);
        case 'diff': return this.openDiff(message.index);
        case 'refresh': return this.refresh(true);
        case 'logs': return this.output.show(true);
      }
    });
  }

  resolveWebviewView(view) {
    this.view = view;
    const media = this.vscode.Uri.joinPath(this.context.extensionUri, 'media');
    view.webview.options = { enableScripts: true, localResourceRoots: [media] };
    view.webview.html = webviewHtml(view.webview, this.vscode, media);
    this.context.subscriptions.push(view.webview.onDidReceiveMessage(message => this.message(message)));
    this.context.subscriptions.push(view.onDidDispose(() => { if (this.view === view) this.view = null; }));
    this.publish();
    if (!this.connected && this.vscode.workspace.isTrusted) {
      const url = (() => { try { return serviceUrl(this.setting('serviceUrl', 'http://127.0.0.1:3000')); } catch { return ''; } })();
      this.context.secrets.get(`ovx.apiToken:${url}`).then(token => { if (token && !this.connected) return this.run(() => this.connect()); }).catch(() => {});
    }
  }
}

function webviewHtml(webview, vscode, media) {
  const nonce = crypto.randomBytes(20).toString('base64');
  const script = webview.asWebviewUri(vscode.Uri.joinPath(media, 'sidebar.js'));
  const style = webview.asWebviewUri(vscode.Uri.joinPath(media, 'sidebar.css'));
  return `<!doctype html><html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
    <meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource}; script-src 'nonce-${nonce}';">
    <link rel="stylesheet" href="${style}"><title>OVX Local Agent</title></head><body>
    <header><div class="brand">OVX <span>LOCAL AGENT</span></div><div id="connection" role="status">Disconnected</div></header>
    <div id="notice" class="notice"></div><div id="error" role="alert" class="error" hidden></div>
    <div class="toolbar"><button data-message="connect">Connect</button><button class="secondary" data-message="startService">Start service</button><button class="secondary" data-message="setToken">Set token</button><button class="secondary" data-message="importToken">Import token</button></div>
    <label for="folder">Repository</label><select id="folder" aria-label="Workspace repository"></select><p id="folderPath" class="muted"></p>
    <form id="promptForm"><label for="prompt">What should OVX do?</label><textarea id="prompt" rows="5" maxlength="20000" placeholder="run full rspec — or describe a coding task" required></textarea><button id="submit" type="submit">Run task</button><span class="muted shortcut">⌘ / Ctrl + Enter</span></form>
    <p class="muted">“run full rspec” runs project tests in your local terminal. Coding requests use the OVX agent.</p><button class="secondary" data-message="runRSpec">Run full RSpec in project</button>
    <section id="localTest" hidden><div class="section-title"><h2>Project RSpec · local execution</h2><span id="localTestState" class="badge"></span></div><pre id="localTestCommand"></pre><p id="localTestPath" class="muted"></p><p id="localTestResult" class="preserve"></p><p class="muted">Output is in the integrated task terminal. Uses this project’s shell environment and files on disk.</p><div id="localTestActions" class="toolbar"></div></section>
    <section><div class="section-title"><h2>Tasks</h2><button class="secondary small" data-message="refresh">Refresh</button></div><select id="history" aria-label="Task history"><option value="">No tasks yet</option></select></section>
    <main id="task" hidden><div class="section-title"><h2 id="taskTitle"></h2><span id="taskState" class="badge"></span></div><p id="request" class="preserve"></p><p id="revision" class="muted"></p><div id="taskError" class="error" hidden></div><div id="taskActions" class="toolbar"></div>
    <section id="approvals"></section><section id="planSection"><h2>Plan</h2><pre id="plan"></pre></section><section id="resultSection"><h2>Result</h2><p id="result" class="preserve"></p></section>
    <section><h2>Changes</h2><p id="diffNotice" class="muted"></p><div id="files"></div></section><section><h2>Tests</h2><div id="tests"></div></section><section><h2>Timeline</h2><div id="events"></div></section></main>
    <footer>Runs through your local OVX service. <button class="link" data-message="logs">Open logs</button></footer>
    <script nonce="${nonce}" src="${script}"></script></body></html>`;
}

function activate(context) {
  const vscode = require('vscode');
  const controller = new Controller(vscode, context);
  context.subscriptions.push(controller);
  context.subscriptions.push(vscode.window.registerWebviewViewProvider('ovx.agent', controller));
  context.subscriptions.push(vscode.workspace.registerTextDocumentContentProvider('ovx-diff', {
    provideTextDocumentContent(uri) { return controller.documents.get(uri.toString()) ?? 'This OVX snapshot is no longer available. Open the diff again from the task.'; }
  }));
  const commands = {
    'ovx.open': () => vscode.commands.executeCommand('ovx.agent.focus'),
    'ovx.connect': () => controller.connect(),
    'ovx.setToken': () => controller.setToken(),
    'ovx.chooseInstallation': () => controller.chooseInstallation(),
    'ovx.importToken': () => controller.importToken(),
    'ovx.startService': () => controller.startService(),
    'ovx.runRSpec': () => controller.runLocalRSpec(),
    'ovx.cancelRSpec': () => controller.cancelLocalRSpec(),
    'ovx.refresh': () => controller.connected ? controller.refresh(true) : controller.connect()
  };
  for (const [name, action] of Object.entries(commands)) context.subscriptions.push(vscode.commands.registerCommand(name, () => controller.run(action)));
  context.subscriptions.push(vscode.tasks.onDidEndTaskProcess(event => controller.finishLocalRSpec(event)));
  context.subscriptions.push(vscode.tasks.onDidEndTask(event => controller.finishLocalRSpec(event, false)));
  context.subscriptions.push(vscode.workspace.onDidChangeWorkspaceFolders(() => controller.run(async () => {
    controller.selectedFolder = controller.folder()?.uri.toString();
    if (controller.connected) await controller.connect();
  })));
  context.subscriptions.push(vscode.workspace.onDidGrantWorkspaceTrust(() => controller.publish()));
  context.subscriptions.push(vscode.workspace.onDidChangeConfiguration(event => {
    if (event.affectsConfiguration('ovx.serviceUrl')) {
      controller.epoch++;
      controller.client?.dispose();
      controller.connected = false;
      controller.task = null;
      controller.repository = null;
      controller.history = [];
      controller.files = [];
      controller.error = 'Service URL changed. Connect with the token for this address.';
      controller.publish();
    }
  }));
  context.subscriptions.push(vscode.workspace.onDidCloseTextDocument(document => {
    if (document.uri.scheme === 'ovx-diff') controller.documents.delete(document.uri.toString());
  }));
  return controller;
}

module.exports = { activate, Controller, webviewHtml };
