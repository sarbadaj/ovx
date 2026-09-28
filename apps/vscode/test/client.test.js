'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { Client } = require('../src/client');
const { startFixture } = require('./fixture');

test('client authenticates API calls and reports unauthorized responses', async t => {
  const fixture = await startFixture();
  t.after(fixture.close);
  const client = new Client(fixture.url, fixture.token);
  t.after(() => client.dispose());
  assert.deepEqual(await client.request('GET', '/api/repositories'), []);
  const wrong = new Client(fixture.url, 'wrong');
  t.after(() => wrong.dispose());
  await assert.rejects(wrong.request('GET', '/api/repositories'), /rejected the API token/);
  await assert.rejects(client.request('GET', '//example.com/api/tasks'), /Invalid API route/);
});

test('localhost resolves only to loopback', async t => {
  const fixture = await startFixture();
  t.after(fixture.close);
  const client = new Client(fixture.url.replace('127.0.0.1', 'localhost'), fixture.token);
  t.after(() => client.dispose());
  assert.deepEqual(await client.request('GET', '/health'), { status: 'ok' });
});

test('redirects never forward credentials, response bounds and timeouts stop requests', async t => {
  let redirectReached = false;
  const server = http.createServer((request, response) => {
    if (request.url === '/api/redirect') { response.writeHead(302, { Location: '/api/leak' }); response.end('{}'); }
    if (request.url === '/api/leak') { redirectReached = true; response.end('{}'); }
    if (request.url === '/api/huge') response.end(JSON.stringify({ content: 'x'.repeat(1000) }));
    if (request.url === '/api/bad-json') response.end('<html>oops</html>');
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => { server.closeAllConnections(); server.close(); });
  const client = new Client(`http://127.0.0.1:${server.address().port}`, 'secret', { timeout: 80, maxBytes: 100 });
  t.after(() => client.dispose());
  await assert.rejects(client.request('GET', '/api/redirect'), /HTTP 302/);
  assert.equal(redirectReached, false);
  await assert.rejects(client.request('GET', '/api/huge'), /too large/);
  await assert.rejects(client.request('GET', '/api/bad-json'), /invalid JSON/);
  await assert.rejects(client.request('GET', '/api/hang'), /timed out/);
});
