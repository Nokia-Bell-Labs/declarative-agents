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
{{- end }}

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
        chunks{{ $i }}: $from(rag_query{{ $i }}).mapped.documents
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
