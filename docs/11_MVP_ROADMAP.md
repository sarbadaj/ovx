# OVX MVP Roadmap

## Goal
Build a usable coding agent through vertical milestones rather than implementing the entire platform first.

## 0.0.1 — End-to-end vertical slice
Scope:
- repository open;
- workspace;
- model adapter;
- context selection;
- list/read/search;
- patch;
- run tests;
- git diff;
- basic sandbox;
- basic event persistence.

Acceptance:
Given a small Rails repo with one failing RSpec test, OVX must inspect, plan, patch, run the affected spec, show passing result and diff, and make no unauthorized external call.

## 0.0.2 — Durable runtime
- explicit state machine;
- ToolCall/ToolAttempt persistence;
- retries;
- cancellation;
- timeout;
- crash recovery;
- `unknown` execution state;
- idempotency strategy.

## 0.0.3 — Security and approvals
- capability model;
- filesystem scopes;
- network default deny;
- approvals;
- secret isolation;
- resource limits;
- audit.

## 0.1 — Rails intelligence
- Rails detection;
- routes/controllers/models/associations/policies/jobs/specs;
- RSpec;
- RuboCop;
- Rails eval suite.

## 0.2 — Developer UX
- Tauri;
- repository picker;
- task view;
- plan;
- tool timeline;
- approvals;
- diff viewer;
- test viewer;
- cancel/retry/resume.

## 0.3 — Git collaboration
- commit;
- branch flow;
- GitHub/GitLab;
- pull-request creation after approval.

## MVP exit criteria
- stable task execution;
- durable recovery;
- runtime-enforced policy;
- measurable eval baseline;
- usable developer UI;
- installation docs;
- selected license.
