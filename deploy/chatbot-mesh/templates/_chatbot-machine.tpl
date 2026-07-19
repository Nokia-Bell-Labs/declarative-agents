{{/*
The chatbot request-machine.yaml, co-generated from .Values.ragUnits (srd015 R2,
GH-372): the sequential fan-out chain has one Retrieving state per RAG server, each
routing QueryResponded (chunks), CommandError (degraded, R3.2), or QueryRejected
(embedding-mismatch excluded, R3.3) on to the next RAG, the last routing to
Composing. The breadth derives from the same list as the fan-out words and the RAG
topology. The packaged request-machine.yaml is the two-RAG local default; this
render overrides that ConfigMap key.
*/}}
{{- define "chatbot-mesh.chatbotMachine" -}}
{{- $last := sub (len .Values.ragUnits) 1 -}}
name: chatbot-turn
purpose: |-
  One request-scoped chat turn (co-generated for {{ len .Values.ragUnits }} RAG
  server(s) from the deployment ragUnits, GH-372). embed_query embeds the message
  once; each rag_queryN fans that one vector out to a declared RAG server. A RAG
  that failed (CommandError, degraded) or rejected the query embedding (a 400
  mapped to QueryRejected, excluded) routes on to the next RAG, so degradation and
  exclusion are visible machine transitions; each RAG's outcome stays in command
  state. compose_prompt then builds the grounding prompt from the surviving
  per-RAG chunk lists, each under its own [ragN] header. route classifies the
  prompt to a chat-LLM word and the chosen word answers; a router misparse or
  error falls back to invoke_llm_fast. A RAG failure degrades to a mapped 200
  rather than a 500.
initial_state: AwaitingRequest
budget:
  max_iterations: {{ add 8 (len .Values.ragUnits) }}
states:
  - {name: AwaitingRequest, meaning: Seeded by the trusted chat endpoint with the request body.}
  - {name: Embedding, meaning: The message is being embedded once at the embedding provider.}
{{- range $i, $unit := .Values.ragUnits }}
  - {name: Retrieving{{ $i }}, meaning: RAG server {{ $i }} ({{ $unit.name }}) is being queried with the message vector.}
{{- end }}
  - {name: Composing, meaning: The grounding prompt is being composed from the message and the surviving per-RAG chunk lists.}
  - {name: Routing, meaning: The router classifier is picking a chat-LLM word for the composed prompt.}
  - {name: Parsing, meaning: The router response is being parsed into one chat-LLM tool call.}
  - {name: Answering, meaning: The chosen chat-LLM word is answering over the composed prompt.}
  - {name: LLMResponded, meaning: Terminal. A grounded answer is ready for the HTTP response.}
  - {name: Failed, meaning: Terminal. An embedding or model boundary word failed.}
terminal_states:
  - LLMResponded
  - Failed
signals:
  - {name: Seed, trigger: The chat endpoint starts a request-scoped turn.}
  - {name: QueryEmbedded, trigger: The provider returned the message embedding.}
  - {name: QueryResponded, trigger: The RAG server returned chunks.}
  - {name: QueryRejected, trigger: The RAG server rejected the query embedding (embedding-space mismatch, a mapped 400); the source is excluded.}
  - {name: Composed, trigger: The grounding prompt was rendered.}
  - {name: LLMResponded, trigger: A model word produced a response.}
  - {name: ToolDone, trigger: The router response parsed to one chat-LLM tool call.}
  - {name: ParseFailed, trigger: The router response did not parse to a declared chat-LLM word.}
  - {name: TaskCompleted, trigger: The router response parsed to the done tool.}
  - {name: CommandError, trigger: A boundary word failed.}
transitions:
  - {state: AwaitingRequest, signal: Seed,           next: Embedding,     action: embed_query}
  - {state: AwaitingRequest, signal: CommandError,   next: Failed}
  - {state: Embedding,       signal: QueryEmbedded,  next: Retrieving0,   action: rag_query0}
  - {state: Embedding,       signal: CommandError,   next: Failed}
{{- range $i, $unit := .Values.ragUnits }}
{{- if eq $i $last }}
{{- $next := "Composing" }}
{{- $act := "compose_prompt" }}
  - {state: Retrieving{{ $i }},     signal: QueryResponded, next: {{ $next }}, action: {{ $act }}}
  - {state: Retrieving{{ $i }},     signal: CommandError,   next: {{ $next }}, action: {{ $act }}}
  - {state: Retrieving{{ $i }},     signal: QueryRejected,  next: {{ $next }}, action: {{ $act }}}
{{- else }}
{{- $next := printf "Retrieving%d" (add $i 1) }}
{{- $act := printf "rag_query%d" (add $i 1) }}
  - {state: Retrieving{{ $i }},     signal: QueryResponded, next: {{ $next }}, action: {{ $act }}}
  - {state: Retrieving{{ $i }},     signal: CommandError,   next: {{ $next }}, action: {{ $act }}}
  - {state: Retrieving{{ $i }},     signal: QueryRejected,  next: {{ $next }}, action: {{ $act }}}
{{- end }}
{{- end }}
  - {state: Composing,       signal: Composed,       next: Routing,       action: route}
  - {state: Composing,       signal: CommandError,   next: Routing,       action: route}
  - {state: Routing,         signal: LLMResponded,   next: Parsing,       action: parse_route}
  - {state: Routing,         signal: CommandError,   next: Answering,     action: invoke_llm_fast}
  - {state: Parsing,         signal: ToolDone,       next: Answering,     action: $tool}
  - {state: Parsing,         signal: ParseFailed,    next: Answering,     action: invoke_llm_fast}
  - {state: Parsing,         signal: TaskCompleted,  next: Answering,     action: invoke_llm_fast}
  - {state: Answering,       signal: LLMResponded,   next: LLMResponded}
  - {state: Answering,       signal: CommandError,   next: Failed}
{{- end -}}
