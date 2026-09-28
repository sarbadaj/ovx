# OVX Context and Indexing

## Goal
Provide the smallest repository context sufficient for the task. Repository size must not directly determine prompt size.

## Sources
- file tree;
- file contents;
- lexical search;
- symbols;
- framework structure;
- dependencies;
- configuration;
- tests;
- Git changes/history;
- prior task observations.

## Retrieval order
1. user-mentioned files;
2. exact symbols;
3. lexical search;
4. framework relationships;
5. test/source relationships;
6. dependency relationships;
7. recent changes;
8. Git history;
9. embeddings if they measurably help.

## Provenance
Each context item includes:
```text
path
start_line
end_line
retrieval_method
reason
relevance_score
content_hash
```

## Context budget
Allocate context among:
```text
system instructions
conversation/task
task state/plan
repository evidence
tool observations
output reserve
```

## Indexing
MVP:
- language detection;
- Rails detection;
- file tree;
- Tree-sitter symbols where supported;
- tests/configuration;
- basic dependency metadata;
- incremental updates.

## Rails graph
Node types:
```text
route
controller_action
model
service
job
policy
serializer
mailer
spec
migration
```

Edges:
```text
routes_to
calls
authorizes_with
serializes_with
enqueues
tests
belongs_to
has_many
has_one
```

## Rails retrieval examples
Request-spec failure: failing spec → route → controller → policy/serializer → model/service.

N+1: query origin → association → serializer/view loop → scope → tests.

## Embeddings
Optional, not required for MVP. Add only if evals show gain over lexical + structural retrieval.

## Invalidation
When files change, invalidate hashes, update symbols/relationships, and avoid stale context.
