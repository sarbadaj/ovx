'use strict';

const fs = require('node:fs/promises');
const path = require('node:path');

// Recognize a small explicit test-only vocabulary. No prompt text becomes shell code.
function isRSpecRequest(request) {
  if (typeof request !== 'string') return false;
  const text = request.trim().toLowerCase().replace(/\s+/g, ' ').replace(/[.!]$/, '').trim();
  return /^(?:please )?(?:run (?:full )?rspec|bundle exec rspec|fix (?:the )?(?:failing )?rspec issue)(?: (?:of this project|in this project|for this project|in this folder))?(?: please)?$/.test(text);
}

const ignored = new Set(['.git', '.ovx', '.bundle', '.venv', 'node_modules', 'vendor', 'tmp', 'log', 'logs', 'coverage', 'dist', 'build', 'storage']);

async function rspecRoots(directory) {
  const root = await fs.realpath(directory);
  const queue = [{ directory: root, depth: 0 }];
  const candidates = [];
  let visited = 0;
  while (queue.length && visited++ < 200) {
    const current = queue.shift();
    const entries = await fs.readdir(current.directory, { withFileTypes: true }).catch(() => []);
    if (entries.some(entry => entry.name === 'Gemfile' && entry.isFile()) && entries.some(entry => entry.name === 'spec' && entry.isDirectory())) {
      candidates.push(current.directory);
      if (current.depth === 0) return candidates;
      continue;
    }
    if (current.depth >= 3) continue;
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
      if (entry.isDirectory() && !entry.name.startsWith('.') && !ignored.has(entry.name)) queue.push({ directory: path.join(current.directory, entry.name), depth: current.depth + 1 });
    }
  }
  return candidates;
}

module.exports = { isRSpecRequest, rspecRoots };
