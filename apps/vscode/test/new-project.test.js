'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { createRubyProject } = require('../src/new-project');

test('new project has a runnable test and never overwrites an existing folder', async t => {
  const parent = await fs.mkdtemp(path.join(os.tmpdir(), 'ovx-new-project-'));
  t.after(() => fs.rm(parent, { recursive: true, force: true }));
  const directory = await createRubyProject(parent, 'sample_app');
  const source = await fs.readFile(path.join(directory, 'lib/project.rb'), 'utf8');
  assert.match(source, /module Project/);
  assert.match(await fs.readFile(path.join(directory, 'test/project_test.rb'), 'utf8'), /test_project_loads/);
  await assert.rejects(createRubyProject(parent, 'sample_app'), { code: 'EEXIST' });
  for (const name of ['../outside', '.hidden', 'one/two', 'a']) await assert.rejects(createRubyProject(parent, name));
});
