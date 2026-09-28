'use strict';
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { isRSpecRequest, rspecRoots } = require('../src/local-tests');

test('only explicit allowlisted RSpec prompts choose direct local execution', () => {
  for (const request of ['run full rspec', 'run rspec of this project', 'Please run full RSpec in this project.', 'bundle exec rspec', 'run rspec for this project please', 'run rspec in this folder']) assert.ok(isRSpecRequest(request), request);
  for (const request of ['fix failing specs', 'run rspec and fix failures', 'bundle exec rspec; rm -rf .', 'run rspec spec/x_spec.rb', 'run rspec\n$(touch foo)', 'run full rspec && echo done', 'run all tests']) assert.equal(isRSpecRequest(request), false, request);
});

test('RSpec root discovery skips dependencies and returns explicit choices for multiple projects', async t => {
  const root = await fs.mkdtemp(path.join(os.tmpdir(), 'ovx-rspec-roots-'));
  t.after(() => fs.rm(root, { recursive: true, force: true }));
  for (const name of ['server', 'admin', 'node_modules/ignored', 'vendor/ignored']) {
    await fs.mkdir(path.join(root, name, 'spec'), { recursive: true });
    await fs.writeFile(path.join(root, name, 'Gemfile'), '');
  }
  const real = await fs.realpath(root);
  assert.deepEqual(await rspecRoots(root), [path.join(real, 'admin'), path.join(real, 'server')]);
  await fs.mkdir(path.join(root, 'spec'));
  await fs.writeFile(path.join(root, 'Gemfile'), '');
  assert.deepEqual(await rspecRoots(root), [real]);
});
