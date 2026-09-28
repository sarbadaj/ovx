'use strict';

const http = require('node:http');
const https = require('node:https');
const { serviceUrl } = require('./core');

class ApiError extends Error {
  constructor(message, status, payload) { super(message); this.name = 'ApiError'; this.status = status; this.payload = payload; }
}

class Client {
  constructor(url, token, options = {}) {
    this.url = serviceUrl(url);
    this.token = token;
    this.timeout = options.timeout || 45000;
    this.maxBytes = options.maxBytes || 24 * 1024 * 1024;
    this.requests = new Map();
  }

  dispose() {
    for (const [request, reject] of this.requests) {
      reject(new ApiError('OVX connection was closed.'));
      request.destroy();
    }
    this.requests.clear();
  }

  request(method, route, data) {
    if (!/^\/(?:api\/|health(?:$|\?))/.test(route) || route.includes('://')) return Promise.reject(new Error('Invalid API route.'));
    const url = new URL(route, this.url);
    if (url.origin !== this.url) return Promise.reject(new Error('OVX API requests must stay on the configured local service.'));
    const body = data === undefined ? undefined : JSON.stringify(data);
    return new Promise((resolve, reject) => {
      const transport = url.protocol === 'https:' ? https : http;
      const headers = { Accept: 'application/json' };
      if (this.token) headers.Authorization = `Bearer ${this.token}`;
      if (body) { headers['Content-Type'] = 'application/json'; headers['Content-Length'] = Buffer.byteLength(body); }
      const options = { method, headers };
      // localhost can resolve to an IPv6 address while Rails is bound only to IPv4.
      // Pin the lookup to loopback, also preventing a modified hosts file from sending credentials elsewhere.
      if (url.hostname === 'localhost') options.lookup = (_host, _options, callback) => {
        if (_options?.all) callback(null, [{ address: '127.0.0.1', family: 4 }]);
        else callback(null, '127.0.0.1', 4);
      };
      const request = transport.request(url, options, response => {
        let bytes = 0;
        const chunks = [];
        response.on('data', chunk => {
          bytes += chunk.length;
          if (bytes > this.maxBytes) {
            reject(new ApiError('OVX response is too large. Review a smaller task or the service logs.'));
            response.destroy();
            request.destroy();
            return;
          }
          chunks.push(chunk);
        });
        response.on('error', reject);
        response.on('end', () => {
          const status = response.statusCode || 0;
          let payload;
          try { payload = JSON.parse(Buffer.concat(chunks).toString('utf8') || '{}'); }
          catch { reject(new ApiError(`OVX returned invalid JSON (HTTP ${status}). Check the service URL and logs.`, status)); return; }
          if (status < 200 || status >= 300) {
            const detail = payload.error || payload.error_message || `HTTP ${status}`;
            const message = status === 401 ? 'OVX rejected the API token. Run “OVX: Set API Token” or import it from your installation.' : `OVX: ${detail}`;
            reject(new ApiError(message, status, payload));
          } else resolve(payload);
        });
      });
      this.requests.set(request, reject);
      const deadline = setTimeout(() => {
        reject(new ApiError(`OVX request timed out after ${Math.round(this.timeout / 1000)} seconds. Check task history before retrying a submitted task.`));
        request.destroy();
      }, this.timeout);
      request.once('close', () => { clearTimeout(deadline); this.requests.delete(request); });
      request.on('error', error => reject(new ApiError(error.code === 'ECONNREFUSED' ? `OVX is not running at ${this.url}. Start it with bin/ovx start or “OVX: Start Local Service”.` : error.message)));
      if (body) request.write(body);
      request.end();
    });
  }
}

module.exports = { Client, ApiError };
