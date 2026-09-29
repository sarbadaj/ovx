'use strict';

const fs = require('node:fs/promises');
const path = require('node:path');

function projectName(name) {
  const value = String(name || '').trim();
  if (!/^[A-Za-z][A-Za-z0-9_-]{1,62}$/.test(value)) throw new Error('Use 2–63 letters, numbers, hyphens or underscores; start with a letter.');
  return value;
}

async function createRubyProject(parent, name) {
  const value = projectName(name);
  const directory = path.join(await fs.realpath(parent), value);
  // No recursive mkdir: an existing project is never overwritten.
  await fs.mkdir(directory);
  await fs.mkdir(path.join(directory, 'lib'));
  await fs.mkdir(path.join(directory, 'test'));
  await fs.writeFile(path.join(directory, 'README.md'), `# ${value}\n\nA Ruby project created by OVX. Ask OVX to add features and tests.\n\nRun tests: \`ruby test/project_test.rb\`\n`);
  await fs.writeFile(path.join(directory, 'lib', 'project.rb'), 'module Project\nend\n');
  await fs.writeFile(path.join(directory, 'test', 'project_test.rb'), `require 'minitest/autorun'\nrequire_relative '../lib/project'\n\nclass ProjectTest < Minitest::Test\n  def test_project_loads\n    assert defined?(Project)\n  end\nend\n`);
  return directory;
}

module.exports = { projectName, createRubyProject };
