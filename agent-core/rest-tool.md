# Configured REST Tool

*Design note for a generic REST boundary word and its use as the transport
layer beneath configurable LLM providers.*

## Problem

The current `invoke_llm` boundary word is conceptually generic, but its
provider boundary is still implemented as provider-specific Go code. Adding a
new model provider should not require adding another compiled adapter when the
provider exposes an ordinary HTTP API.

The same problem appears outside model calls. Many useful agent capabilities
are REST-shaped: AWS services, GitLab, issue trackers, webhooks, internal
control planes, evaluation harnesses, and agent-to-agent coordination. These
should be configured as words, not coded as one-off Go implementations.

The goal is a generic REST word whose authority comes from trusted agent
configuration. The model may fill allowed parameters, but it must not invent
URLs, methods, authentication, or network destinations at runtime.

## Design Principles

### REST is a capability, not a provider

The Go implementation provides the generic capability to make HTTP requests:
constructing requests, applying authentication, enforcing timeouts, sending
over `net/http`, interpreting response status, recording traces, and returning
structured output.

The tool declaration provides specificity: allowed operation names, URLs,
methods, headers, request templates, response mappings, success codes, retry
policy, and side-effect declarations.

This follows the existing grammar principle:

- Engine interprets grammar.
- Grammar dispatches words.
- Words interpret tool configuration.
- Boundary words interpret actor configuration.

### The LLM does not own transport authority

A REST word in the state machine is configured by the agent Go program through
loaded YAML. Its URL comes from configuration, not from text returned by an
LLM.

The LLM may produce an operation request such as:

```json
{
  "operation": "ollama.chat",
  "params": {
    "model": "qwen3.6:35b-mlx",
    "messages": []
  }
}
```

It must not produce:

```json
{
  "method": "POST",
  "url": "https://model-chosen.example/api",
  "headers": {}
}
```

The REST word resolves `operation` against trusted configuration and rejects
unknown operations or parameters that fail schema validation.

### LLM semantics and REST semantics are separate

The LLM boundary should be decomposable into words:

```text
prepare_llm_request -> call_rest -> accept_llm_response
```

`prepare_llm_request` knows model semantics: conversation history, prompt
assembly, selected tools, model profile, provider operation, and request
parameters.

`call_rest` knows transport semantics: method, URL, headers, auth, retries,
timeouts, status codes, and raw HTTP response capture.

`accept_llm_response` knows model response semantics: provider-specific
content extraction, token accounting, error normalization, and appending the
assistant response to conversation history.

From a parent grammar's perspective this sequence may still be hidden behind a
single non-terminal boundary word if simple agents want the old fused
`invoke_llm` behavior.

## Proposed Words

### `prepare_llm_request`

Builds a normalized REST operation request from current agent state.

Responsibilities:

- Read conversation history and prompt assembly state.
- Read selected tool manifest and model profile.
- Select a configured provider operation, such as `ollama.chat`,
  `openai.responses`, or `bedrock.invoke_model`.
- Produce operation parameters that match that operation's schema.
- Store or emit a structured operation request for the REST word.

Non-goals:

- Does not open a network connection.
- Does not parse provider responses.
- Does not append new assistant messages to history.

Emits:

- `LLMRequestPrepared`
- `CommandError`

### `call_rest`

Executes one configured REST operation.

Responsibilities:

- Resolve the requested operation name from trusted configuration.
- Validate runtime parameters against the operation input schema.
- Render URL path variables, query parameters, headers, and request body from
  approved templates.
- Apply configured authentication.
- Enforce timeout, retry, body-size, and redirect policy.
- Send the request using Go's HTTP client.
- Return structured status, headers, and body.
- Record trace metadata without leaking configured secrets.

Non-goals:

- Does not decide which operation should be called.
- Does not allow model-provided arbitrary URLs.
- Does not interpret LLM-specific response meaning.

Emits:

- `RESTResponded`
- `RESTFailed`
- `CommandError`

### `accept_llm_response`

Converts a REST response into the LLM contract expected by the agent.

Responsibilities:

- Read the operation response mapping selected by `prepare_llm_request`.
- Extract assistant text from the provider response.
- Extract token usage and latency metadata when available.
- Normalize provider errors into agent errors.
- Append the assistant message to conversation history exactly once on
  success.
- Emit the same high-level signal the current grammar expects from
  `invoke_llm`.

Non-goals:

- Does not execute HTTP.
- Does not dispatch tool calls requested by the model.
- Does not validate parsed tool requests; that remains `parse_response`.

Emits:

- `LLMResponded`
- `CommandError`

## Example State Machine

The current fused flow:

```text
assemble_prompt -> invoke_llm -> parse_response -> $tool
```

Can become:

```text
assemble_prompt
  -> prepare_llm_request
  -> call_rest
  -> accept_llm_response
  -> parse_response
  -> $tool
```

Example transition sketch:

```yaml
transitions:
  - state: Composing
    signal: PromptAssembled
    next: PreparingLLMRequest
    action: prepare_llm_request

  - state: PreparingLLMRequest
    signal: LLMRequestPrepared
    next: CallingProvider
    action: call_rest

  - state: CallingProvider
    signal: RESTResponded
    next: AcceptingLLMResponse
    action: accept_llm_response

  - state: AcceptingLLMResponse
    signal: LLMResponded
    next: Parsing
    action: parse_response
```

For compatibility, a blocking `invoke_llm` word can remain as a facade that
runs this internal sentence and returns `LLMResponded` or `CommandError`.

## Configuration Model

REST configuration should describe operations, not free-form requests.

```yaml
rest:
  operations:
    ollama.chat:
      method: POST
      url: "{{ provider_url }}/api/chat"
      success_codes: [200]
      timeout: 600s
      headers:
        Content-Type: application/json
      input_schema:
        type: object
        properties:
          model:
            type: string
          messages:
            type: array
          stream:
            type: boolean
        required: [model, messages]
      body:
        model: "{{ params.model }}"
        messages: "{{ params.messages }}"
        stream: false
      response:
        content: "$.message.content"
        prompt_tokens: "$.prompt_eval_count"
        completion_tokens: "$.eval_count"

    openai.responses:
      method: POST
      url: "https://api.openai.com/v1/responses"
      success_codes: [200]
      timeout: 600s
      auth:
        type: bearer
        token_ref: openai_api_key
      headers:
        Content-Type: application/json
      input_schema:
        type: object
        properties:
          model:
            type: string
          input:
            type: array
        required: [model, input]
      body:
        model: "{{ params.model }}"
        input: "{{ params.input }}"
      response:
        content: "$.output[0].content[0].text"
        prompt_tokens: "$.usage.input_tokens"
        completion_tokens: "$.usage.output_tokens"
```

The exact template language is an implementation choice, but it should be
structured and constrained. It should not evaluate arbitrary code.

## Tool Declaration Example

The generic REST word can be exposed as one or many configured tools.

```yaml
tools:
  - name: call_rest
    type: builtin
    init: rest_call
    visibility: internal
    category: boundary
    emits: [RESTResponded, RESTFailed, CommandError]
    description: "Call a configured REST operation."
    parameters:
      type: object
      properties:
        operation:
          type: string
          enum: [ollama.chat, openai.responses]
        params:
          type: object
      required: [operation, params]
    side_effects:
      - kind: external_api_call
        target: configured_rest_operation
        description: "Sends one HTTP request to a configured endpoint."
    reversibility:
      classification: irreversible
      undo: noop
    undo:
      strategy: noop
      description: "HTTP calls cannot be generally undone by the REST word."
```

For public model-selected tools, prefer narrow operation-specific names with
fixed operations:

```yaml
tools:
  - name: create_ticket
    type: builtin
    init: rest_call
    emits: [ToolDone, ToolFailed]
    config:
      operation: jira.create_issue
```

In that form the LLM only supplies `params`; it cannot choose the operation.

## Auth Strategies

The REST word should support pluggable authentication strategies. Provider
configuration chooses the strategy, and runtime credentials are resolved by
trusted agent configuration.

Initial strategies:

- `none`
- `basic`
- `bearer`
- `api_key_header`
- `api_key_query`

Later strategies:

- `oauth2_client_credentials`
- `aws_sigv4`

AWS is REST-like but not merely "method plus URL." A useful AWS configuration
layer needs SigV4 signing, region and service metadata, endpoint resolution,
pagination, throttling retries, and service-specific protocol details. These
can still be generic auth and protocol capabilities, but they should be
explicitly modeled instead of hidden in ad hoc templates.

## Safety Model

The REST word is a boundary word and must be conservative by default.

Required controls:

- Only configured operations may be called.
- Operation URLs are loaded from trusted configuration.
- Runtime parameters are schema-validated before rendering.
- Secrets are referenced by name and resolved by the agent, never supplied by
  model text.
- Redirects are disabled by default or constrained to the same host.
- Request and response body size limits are enforced.
- Headers containing credentials are redacted in traces and errors.
- Network allowlists may restrict hostnames, CIDRs, schemes, and ports.
- Irreversible operations declare side effects and confirmation policy.

The REST word should never be a model-controlled open proxy.

## State and Rollback

`prepare_llm_request` mutates no external state. It may store the prepared
operation request in command state.

`call_rest` performs an external API call. Most HTTP side effects are
irreversible from the generic word's perspective. The command can record the
request, response, status, operation name, and correlation IDs for audit and
possible compensation, but it should usually declare `undo: noop` unless an
operation-specific compensating action is configured.

`accept_llm_response` mutates conversation history. It should checkpoint the
conversation length before appending and undo by truncating to that checkpoint.
This preserves the current `invoke_llm` rollback behavior while moving the
transport into a separate word.

## Error Handling

Errors should preserve the layer where failure occurred.

- `prepare_llm_request` errors mean model/profile/conversation configuration
  could not produce valid operation parameters.
- `call_rest` errors mean operation lookup, schema validation, request
  rendering, authentication, network I/O, timeout, or HTTP status failed.
- `accept_llm_response` errors mean the HTTP call succeeded but the body could
  not be mapped into the expected LLM response contract.

The output should include structured diagnostic fields:

```json
{
  "operation": "ollama.chat",
  "stage": "response_mapping",
  "status_code": 200,
  "message": "missing $.message.content"
}
```

## Observability

Every REST call should emit trace attributes for:

- operation name
- method
- configured host
- status code
- elapsed time
- retry count
- request and response byte counts
- provider/model metadata when supplied by the caller

Traces should not include secrets. Full request and response bodies should be
captured only when explicit verbose tracing is enabled and redaction rules have
run.

## Migration Path

1. Introduce `rest_call` as an internal builtin word with operation
   configuration, schema validation, and a small auth strategy set.
2. Add tests for operation lookup, URL authority, parameter validation,
   template rendering, status handling, timeout, and redaction.
3. Split `invoke_llm` internally into request preparation, REST call, and
   response acceptance while preserving the external `invoke_llm` declaration.
4. Move Ollama request and response shape into configuration.
5. Add a second provider without new provider-specific Go code to prove the
   abstraction.
6. Expose operation-specific REST tools for non-LLM integrations.
7. Add AWS support as explicit protocol/auth capabilities, beginning with
   SigV4 and one narrow service operation.

## Open Questions

- Should `call_rest` read the prepared operation request from command state,
  previous result output, or both?
- Which template language gives enough structure without becoming embedded
  code?
- Should response mapping use JSONPath, jq-like selectors, or a small custom
  path syntax?
- How should credentials be resolved in this project without introducing
  direct environment-variable reads in tool code?
- Should non-idempotent operations require grammar-level approval, tool-level
  confirmation metadata, or both?
- Should AWS service definitions be hand-written YAML, generated from Smithy,
  or imported from another service catalog?

