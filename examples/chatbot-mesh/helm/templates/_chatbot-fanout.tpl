{{/*
The chatbot request-fanout.yaml, co-generated from .Values.ragUnits (srd015 R2,
GH-372): one rag_queryN word per RAG server and a compose that reads every
surviving per-source chunk list. The fan-out breadth derives from the same list
that renders the RAG objects and the rest.yaml clients, so a values change adds a
source everywhere at once. The packaged agents/chatbot/request-fanout.yaml is the
two-RAG local default; this render overrides that ConfigMap key. Runtime
{{`{{ params.x }}`}} / {{`{{ chunks }}`}} bodies are emitted literally.
*/}}
{{- define "chatbot-mesh.chatbotFanout" -}}
tools:
{{- range $i, $unit := .Values.ragUnits }}
  - name: rag_query{{ $i }}
    type: builtin
    init: rest_client_invoke
    visibility: internal
    category: boundary
    description: Query RAG server {{ $i }} ({{ $unit.name }}) with the message vector.
    problem: The chatbot fans the one message embedding out to each declared RAG server for grounding chunks.
    goals:
      - Thread the message embedding into this RAG server query.
      - Return the retrieved chunk ids, documents, distances, and embedding model.
    requirements:
      input:
        - The query embedding is selected from embed_query through command-state $from() addressing.
      output:
        - Output includes the retrieved chunk ids, documents, distances, and the RAG embedding model.
      errors:
        - Network, timeout, schema, and mapping failures emit CommandError so the turn degrades to the next RAG.
        - A 400 embedding-space mismatch (srd013 R2.3) emits QueryRejected so this source is excluded from the composed chunks.
    non_goals:
      - Does not author the query vector.
      - Does not accept runtime method, URL, host, or auth overrides.
    parameters:
      type: object
      properties:
        query_embeddings: {type: array}
    emits: [QueryResponded, QueryRejected, CommandError]
    output:
      description: Mapped RAG server query result.
      schema:
        type: object
        properties:
          ids: {type: array}
          documents: {type: array}
          distances: {type: array}
          embedding_model: {type: string}
    side_effects:
      - kind: external_api
        target: rag_server.query
        state: read_only
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Querying the RAG server does not mutate state.
    config:
      rest_ref: rag{{ $i }}
      operation: rag{{ $i }}_query

  - name: compare_model{{ $i }}
    type: builtin
    init: value_predicate
    visibility: internal
    category: response
    description: Compare RAG server {{ $i }} ({{ $unit.name }}) reported embedding model with the query embedding's model.
    problem: |
      A 400 from the RAG server only catches a vector the collection rejects, which is a
      dimension mismatch. Two models of the same width produce a 200, so without this
      comparison the turn composes chunks retrieved in a different embedding space than
      the query (srd002 R3.3).
    goals:
      - Compare the query embedding model identity with the model this RAG reported.
      - Emit a distinct signal for a matching and a differing identity so exclusion is a visible transition.
    requirements:
      input:
        - Both operands are read from prior steps through command-state $from() addressing.
      output:
        - Output records the verdict and both compared identities.
      errors:
        - An unresolved operand emits CommandError, so the source degrades rather than being silently kept.
    non_goals:
      - Does not drop the chunks itself; the machine routes past the keep word for a differing identity.
      - Does not call any provider.
    parameters:
      type: object
      properties: {}
      additionalProperties: false
    emits: [ModelMatched, ModelDiffered, CommandError]
    output:
      description: The embedding-model comparison verdict.
      schema:
        type: object
        properties:
          verdict: {type: string}
          left: {type: string}
          right: {type: string}
    side_effects: []
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Comparing two command-state values has no durable effect.
    config:
      left: $from(declare_query_model).model
      op: eq
      right: $from(rag_query{{ $i }}).mapped.embedding_model
      operand_type: string
      satisfied: ModelMatched
      unsatisfied: ModelDiffered

  - name: keep_chunks{{ $i }}
    type: builtin
    init: compose
    visibility: internal
    category: response
    description: Keep RAG server {{ $i }} ({{ $unit.name }}) chunks for composition after its embedding model matched.
    problem: |
      A machine transition alone cannot exclude a source, because the queried chunks stay
      addressable in command state after a differing identity. The machine dispatches this
      word only on the matching path, so the composed prompt reads chunks through a label
      that exists only for a source that survived (srd002 R3.3).
    goals:
      - Republish this RAG's chunks under a label that exists only when the source survived.
      - Leave an excluded or degraded source unresolved, which compose renders empty.
    requirements:
      input:
        - The chunks are selected from rag_query{{ $i }} through command-state $from() addressing.
      output:
        - Output is a JSON object whose documents field carries the surviving chunks.
      errors:
        - Reached only after a matching comparison, so the selector resolves.
    non_goals:
      - Does not order, cap, or rewrite chunks.
      - Does not decide whether the source survived; compare_model{{ $i }} and the machine do.
    parameters:
      type: object
      properties: {}
      additionalProperties: false
    emits: [ChunksKept{{ $i }}]
    output:
      description: The surviving chunks from RAG server {{ $i }}.
      schema:
        type: object
        properties:
          documents: {type: array}
    side_effects: []
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Republishing chunks has no durable effect.
    config:
      signal: ChunksKept{{ $i }}
      inputs:
        documents: $from(rag_query{{ $i }}).mapped.documents
      template: |
        {{ printf "{\"documents\": {{ documents }}, \"outcome\": \"composed\", \"reason\": \"\"}" }}

  - name: mark_excluded_model{{ $i }}
    type: builtin
    init: compose
    visibility: internal
    category: response
    description: Record that RAG server {{ $i }} ({{ $unit.name }}) was excluded for an embedding-model mismatch.
    problem: |
      srd002 R3.3 requires the exclusion to be reported in the response metadata, so the
      reason must be recorded where the response can read it.
    goals:
      - Record this source's outcome under a label the response metadata reads.
      - Keep the outcome a visible machine transition rather than an inference the client makes.
    requirements:
      input:
        - The outcome is fixed by the transition that dispatched this word; no selector is read.
      output:
        - Output is a JSON object naming the outcome and its reason.
      errors:
        - Rendering a configured constant does not fail.
    non_goals:
      - Does not decide the outcome; the machine transition that reaches this word does.
      - Does not call any provider.
    parameters:
      type: object
      properties: {}
      additionalProperties: false
    emits: [SourceMarked{{ $i }}]
    output:
      description: This source's outcome for the response metadata.
      schema:
        type: object
        properties:
          outcome: {type: string}
          reason: {type: string}
    side_effects: []
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Recording an outcome has no durable effect.
    config:
      signal: SourceMarked{{ $i }}
      inputs: {}
      template: |
        {{ printf "{\"outcome\": \"excluded\", \"reason\": \"embedding_model\"}" }}

  - name: mark_rejected{{ $i }}
    type: builtin
    init: compose
    visibility: internal
    category: response
    description: Record that RAG server {{ $i }} ({{ $unit.name }}) rejected the query vector.
    problem: |
      A mapped 400 excludes this source for a different reason than an identity mismatch,
      and R3.3 requires the two to be distinguishable in the response metadata.
    goals:
      - Record this source's outcome under a label the response metadata reads.
      - Keep the outcome a visible machine transition rather than an inference the client makes.
    requirements:
      input:
        - The outcome is fixed by the transition that dispatched this word; no selector is read.
      output:
        - Output is a JSON object naming the outcome and its reason.
      errors:
        - Rendering a configured constant does not fail.
    non_goals:
      - Does not decide the outcome; the machine transition that reaches this word does.
      - Does not call any provider.
    parameters:
      type: object
      properties: {}
      additionalProperties: false
    emits: [SourceMarked{{ $i }}]
    output:
      description: This source's outcome for the response metadata.
      schema:
        type: object
        properties:
          outcome: {type: string}
          reason: {type: string}
    side_effects: []
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Recording an outcome has no durable effect.
    config:
      signal: SourceMarked{{ $i }}
      inputs: {}
      template: |
        {{ printf "{\"outcome\": \"excluded\", \"reason\": \"vector_rejected\"}" }}

  - name: mark_degraded{{ $i }}
    type: builtin
    init: compose
    visibility: internal
    category: response
    description: Record that RAG server {{ $i }} ({{ $unit.name }}) failed and the turn degraded without it.
    problem: |
      srd002 R3.2 requires a per-RAG failure to be noted in the response metadata rather
      than left as a silently thinner answer.
    goals:
      - Record this source's outcome under a label the response metadata reads.
      - Keep the outcome a visible machine transition rather than an inference the client makes.
    requirements:
      input:
        - The outcome is fixed by the transition that dispatched this word; no selector is read.
      output:
        - Output is a JSON object naming the outcome and its reason.
      errors:
        - Rendering a configured constant does not fail.
    non_goals:
      - Does not decide the outcome; the machine transition that reaches this word does.
      - Does not call any provider.
    parameters:
      type: object
      properties: {}
      additionalProperties: false
    emits: [SourceMarked{{ $i }}]
    output:
      description: This source's outcome for the response metadata.
      schema:
        type: object
        properties:
          outcome: {type: string}
          reason: {type: string}
    side_effects: []
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Recording an outcome has no durable effect.
    config:
      signal: SourceMarked{{ $i }}
      inputs: {}
      template: |
        {{ printf "{\"outcome\": \"degraded\", \"reason\": \"query_failed\"}" }}
{{- end }}

  - name: compose_response
    type: builtin
    init: compose
    visibility: internal
    category: response
    description: Compose the chat response from the answer and each RAG source's outcome.
    problem: |
      srd002 R3.2 and R3.3 require a degraded or excluded source to be reported in the
      response metadata, but the terminal response body selects only from the response
      word's output, so the answer and the metadata have to reach it together. The
      answer comes from whichever chat-LLM word the router dispatched, which no
      $from(label) names, so it is read from the previous result.
    goals:
      - Pair the answer with one metadata entry per declared RAG source.
      - Name each source's outcome and, for an exclusion, which of the two reasons applies.
      - Report every source on every turn, so a fully grounded turn is distinguishable from a degraded one.
    requirements:
      input:
        - The answer is the previous result; each source's outcome is selected from the word that marked it.
      output:
        - Output is a JSON object carrying the answer and the per-source metadata.
      errors:
        - Exactly one outcome word resolves per source; the other three are unresolved and render empty, which is how the concatenation yields that source's single outcome.
    non_goals:
      - Does not decide any outcome; the machine transitions and their marker words do.
      - Does not alter the answer text.
    parameters:
      type: object
      properties: {}
      additionalProperties: false
    emits: [ResponseComposed]
    output:
      description: The chat answer with per-source grounding metadata.
      schema:
        type: object
        properties:
          answer: {type: string}
          metadata: {type: object}
    side_effects: []
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Composing a response has no durable effect.
    config:
      signal: ResponseComposed
      inputs:
        answer: $.
{{- range $i, $unit := .Values.ragUnits }}
        o_keep{{ $i }}: $from(keep_chunks{{ $i }}).outcome
        o_model{{ $i }}: $from(mark_excluded_model{{ $i }}).outcome
        o_vector{{ $i }}: $from(mark_rejected{{ $i }}).outcome
        o_degraded{{ $i }}: $from(mark_degraded{{ $i }}).outcome
        r_keep{{ $i }}: $from(keep_chunks{{ $i }}).reason
        r_model{{ $i }}: $from(mark_excluded_model{{ $i }}).reason
        r_vector{{ $i }}: $from(mark_rejected{{ $i }}).reason
        r_degraded{{ $i }}: $from(mark_degraded{{ $i }}).reason
        em{{ $i }}: $from(rag_query{{ $i }}).mapped.embedding_model
{{- end }}
        qmodel: $from(declare_query_model).model
      template: |
        {
          "answer": {{ printf "{{ json answer }}" }},
          "metadata": {
            "query_embedding_model": {{ printf "{{ json qmodel }}" }},
            "sources": [
{{- range $i, $unit := .Values.ragUnits }}
{{- $comma := "," }}{{ if eq $i (sub (len $.Values.ragUnits) 1) }}{{ $comma = "" }}{{ end }}
              {"name": "{{ $unit.name }}", "outcome": "{{ printf "{{ o_keep%d }}{{ o_model%d }}{{ o_vector%d }}{{ o_degraded%d }}" $i $i $i $i }}",
               "reason": "{{ printf "{{ r_keep%d }}{{ r_model%d }}{{ r_vector%d }}{{ r_degraded%d }}" $i $i $i $i }}",
               "reported_embedding_model": {{ printf "{{ json em%d }}" $i }}}{{ $comma }}
{{- end }}
            ]
          }
        }

  - name: compose_prompt
    type: builtin
    init: compose
    visibility: internal
    category: response
    description: Compose the grounding prompt from the message and each RAG's retrieved chunks.
    problem: The answer step needs the original message and the non-adjacent retrieved chunks from every RAG source in one prompt without carry_forward chains.
    goals:
      - Read the original message from embed_query and each RAG's chunks through command-state $from().
      - Render one grounding prompt for the model, each source under its own [ragN] header.
      - Render an unresolved source (a degraded or excluded RAG) as empty, so the turn still yields a prompt from the surviving sources.
    requirements:
      input:
        - The message and each RAG's chunks are selected from prior steps through $from() addressing.
      output:
        - Output is the rendered grounding prompt.
      errors:
        - An unresolved selector renders empty and is reported, so a degraded or excluded RAG still yields a prompt.
    non_goals:
      - Does not call any provider or author chunks.
      - Does not receive transport authority.
      - Does not order or cap chunks; per-source top-k is bounded upstream by the RAG query n_results (srd013 R2.1).
    parameters:
      type: object
      properties: {}
      additionalProperties: false
    emits: [Composed]
    output:
      description: The rendered grounding prompt.
      schema:
        type: object
    side_effects: []
    reversibility:
      classification: reversible
      undo: noop
    undo:
      strategy: noop
      description: Rendering a prompt has no durable effect.
    config:
      signal: Composed
      inputs:
        message: $from(embed_query).carried.input
{{- range $i, $unit := .Values.ragUnits }}
        chunks{{ $i }}: $from(keep_chunks{{ $i }}).documents
{{- end }}
      template: |
        A user asked the following question:

        {{`{{ message }}`}}

        The following corpus chunks were retrieved to ground the answer. Each item
        is a document; cite the retrieved chunk when you use it. The chunks are
        grouped by the RAG source they came from.
{{- range $i, $unit := .Values.ragUnits }}

        [rag{{ $i }}]
        {{ printf "{{ chunks%d }}" $i }}
{{- end }}

        Answer the user's question using only these retrieved chunks. If the chunks
        do not contain the answer, say so plainly and do not speculate.
{{- end -}}
