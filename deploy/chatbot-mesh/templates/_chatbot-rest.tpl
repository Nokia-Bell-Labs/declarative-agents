{{/*
The chatbot rest.yaml, co-generated from .Values.ragUnits (srd015 R2). The RAG
client entries, the provider egress allowlist, and the monitor_proxy upstreams
all derive from the same list that renders the RAG Deployments and Services, so
the deployed topology and the chatbot's client configuration cannot drift. The
packaged agents/chatbot/rest.yaml stays the local integration source; this render
overrides that ConfigMap key in the cluster. Server addresses bind 0.0.0.0 so the
Services route to the pod. Runtime {{`{{ params.x }}`}} bodies are emitted
literally.
*/}}
{{- define "chatbot-mesh.chatbotRest" -}}
{{- $fullname := include "chatbot-mesh.fullname" . -}}
{{- $q := .Values.ragServer.ports.query -}}
{{- $mon := .Values.ragServer.ports.monitor -}}
{{- $llmURL := include "chatbot-mesh.llmURL" . -}}
{{- $llmHost := (urlParse $llmURL).hostname -}}
{{- $embedModel := .Values.chatbot.embeddingModel -}}
rest:
  version: v1
  auth:
    none:
      type: none
  limits:
    local_provider:
      timeout: 130s
      read_timeout: 130s
      max_request_bytes: 1048576
      max_response_bytes: 1048576
      redirect:
        mode: none
      network:
        schemes: [http]
        hosts:
          - 127.0.0.1
          - localhost
          - {{ $llmHost }}
{{- range .Values.ragUnits }}
          - {{ $fullname }}-{{ .name }}
{{- end }}
        ports: [{{ .Values.llm.port }}, {{ $q }}]
    local_chat_requests:
      timeout: 130s
      read_timeout: 130s
      max_request_bytes: 1048576
      max_response_bytes: 1048576
      redirect:
        mode: none
      network:
        schemes: [http]
        hosts: [127.0.0.1, localhost]
        ports: [{{ .Values.chatbot.ports.chat }}]
        allow_public_listener: true
    local_chatbot_control:
      timeout: 30s
      read_timeout: 5s
      max_request_bytes: 16384
      max_response_bytes: 65536
      redirect:
        mode: none
      network:
        schemes: [http]
        hosts: [127.0.0.1, localhost]
        ports: [{{ .Values.chatbot.ports.control }}]
        allow_public_listener: true
    local_chatbot_monitor:
      timeout: 5s
      read_timeout: 5s
      max_request_bytes: 4096
      max_response_bytes: 1048576
      redirect:
        mode: none
      network:
        schemes: [http]
        hosts: [127.0.0.1, localhost]
        ports: [{{ .Values.chatbot.ports.monitor }}]
        allow_public_listener: true

  clients:
    embedding:
      base_url: {{ $llmURL }}
      auth_ref: none
      limits_ref: local_provider
      operations:
        embed_query:
          method: POST
          path: /api/embeddings
          params:
            body_schema:
              type: object
              required: [input]
              properties:
                input: {type: string}
            body_source: previous_result
            input_mapping:
              input: $.message
            carry_forward: [input]
          body:
            model: {{ $embedModel }}
            prompt: "{{`{{ params.input }}`}}"
          success: {status: [200], signal: QueryEmbedded}
          response:
            output:
              embedding: $.embedding
          side_effects:
            - kind: external_api
              target: ollama.embeddings
              state: read_only
          reversibility:
            classification: reversible
            undo: noop
{{- range $i, $unit := .Values.ragUnits }}
    rag{{ $i }}:
      base_url: http://{{ $fullname }}-{{ $unit.name }}:{{ $q }}
      auth_ref: none
      limits_ref: local_provider
      operations:
        rag{{ $i }}_query:
          method: POST
          path: /api/v1/rag/query
          params:
            body_schema:
              type: object
              required: [query_embeddings]
              properties:
                query_embeddings: {type: array}
            body_source: command_state
            input_mapping:
              query_embeddings: $from(embed_query).mapped.embedding
          body:
            query_embeddings: "{{`{{ params.query_embeddings }}`}}"
            n_results: 5
          success: {status: [200], signal: QueryResponded}
          failures:
            # 400 embedding-space mismatch -> QueryRejected (excluded, srd014 R3.3),
            # distinct from a degraded (CommandError) RAG.
            - {status: [400], signal: QueryRejected}
          response:
            output:
              ids: $.ids
              documents: $.documents
              distances: $.distances
              embedding_model: $.embedding_model
          side_effects:
            - kind: external_api
              target: rag_server.query
              state: read_only
          reversibility:
            classification: reversible
            undo: noop
{{- end }}

  servers:
    chatbot_chat:
      address: 0.0.0.0:{{ .Values.chatbot.ports.chat }}
      limits_ref: local_chat_requests
      endpoints:
        chat:
          method: POST
          path: /api/v1/chat
          binding: machine_request
          request:
            body_schema:
              type: object
              required: [message]
              properties:
                message: {type: string}
                history: {type: array}
          machine_request:
            profile: profile.yaml
            machine: request-machine.yaml
            timeout: 130s
            request:
              body:
                message: $.message
                history: $.history
            response:
              terminal_signals:
                LLMResponded:
                  status: 200
                  content_type: application/json
                  body:
                    answer: $.output
                CommandError:
                  status: 500
                  content_type: application/json
                  body:
                    error: command_error
                    message: $.message
        monitor_proxy:
          method: GET
          path: /monitor-proxy/{agent}/{path...}
          binding: monitor_proxy
          monitor_proxy:
            upstreams:
              chatbot: http://127.0.0.1:{{ .Values.chatbot.ports.monitor }}
{{- range $unit := .Values.ragUnits }}
              {{ $unit.name }}: http://{{ $fullname }}-{{ $unit.name }}:{{ $mon }}
{{- end }}
{{- if .Values.jaeger.enabled }}
              jaeger: http://{{ $fullname }}-jaeger:{{ .Values.jaeger.queryPort }}
{{- end }}
          request:
            path:
              agent: {type: string}
              path: {type: string}
        ui:
          method: GET
          path: /ui/{path...}
          binding: static_assets
          static_assets:
            root: agents/chatbot/ui/app/dist
            index: index.html
            spa: true
          request:
            path:
              path: {type: string}
        root_redirect:
          method: GET
          path: /
          binding: redirect
          redirect:
            location: /ui/
            status: 302

    chatbot_control:
      address: 0.0.0.0:{{ .Values.chatbot.ports.control }}
      limits_ref: local_chatbot_control
      queue:
        name: chatbot_control
        capacity: 8
        timeout: 30s
      shutdown:
        timeout: 5s
        drain_policy: drain_then_stop
      endpoints:
        exit:
          method: POST
          path: /api/lifecycle/exit
          binding: emit_signal
          signal: ExitRequested
          request:
            body_schema:
              type: object
              properties:
                reason: {type: string}
                status: {type: string}
          response:
            output:
              accepted: "true"
        health:
          method: GET
          path: /api/lifecycle/health
          binding: health

    monitor:
      address: 0.0.0.0:{{ .Values.chatbot.ports.monitor }}
      limits_ref: local_chatbot_monitor
      queue:
        name: chatbot_monitor
        capacity: 8
        overflow: reject
        timeout: 100ms
      shutdown:
        timeout: 2s
        drain_policy: drain
        stop_listeners: true
        unblock_await_signal: ServerStopped
      endpoints:
        machine_spec:  {method: GET, path: /monitor/machine,       binding: read_state,    monitor_view: machine_spec}
        current_state: {method: GET, path: /monitor/state,         binding: read_state,    monitor_view: current_state}
        tools:         {method: GET, path: /monitor/tools,         binding: read_state,    monitor_view: tools}
        metrics:       {method: GET, path: /monitor/metrics,       binding: read_state,    monitor_view: metrics}
        recent_events: {method: GET, path: /monitor/events,        binding: read_state,    monitor_view: events}
        event_stream:  {method: GET, path: /monitor/events/stream, binding: stream_events, monitor_view: events}
        control_exit:
          method: POST
          path: /monitor/control/exit
          binding: emit_signal
          signal: ExitRequested
          request:
            body_schema:
              type: object
              properties:
                reason: {type: string}
          response:
            output:
              accepted: "true"
{{- end -}}
