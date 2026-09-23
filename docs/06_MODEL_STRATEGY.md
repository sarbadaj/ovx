# OVX Model Strategy

## Goal
The agent depends on a stable OVX interface, not provider-specific SDK behavior.

## ModelAdapter
Conceptual interface:
```text
chat(request)
stream(request)
capabilities()
health()
```

## Normalized request
```text
messages
tools
response_format
max_output_tokens
sampling policy
task metadata
```

## Normalized response
```text
content
tool_calls
usage
finish_reason
provider_metadata
```

## Capabilities
Models differ in:
- tool calling;
- structured output;
- streaming;
- context window;
- max output;
- coding quality;
- patch generation;
- instruction following;
- latency;
- hardware needs.

Agent should query capabilities.

## Initial providers
### llama.cpp
Local Apple Silicon, quantized open-weight models, offline workflows.

### vLLM
GPU servers, concurrency, production-like self-hosting.

### Future providers
Adapters may support additional APIs while preserving the internal contract.

## Routing
MVP uses one configured provider. Later routing may consider complexity, privacy, cost, latency, context size, and tool reliability.

## Prompt strategy
Prompts define role, tool rules, safety constraints, task constraints, and completion requirements.

Do not persist private chain-of-thought. Persist plans, tool calls, summaries, states, and outcomes.

## Failure handling
Classify timeout, malformed tool call, schema violation, provider unavailable, context overflow, and repeated non-progress.

## Evaluation
Every model change should run the same repository-task benchmark.
