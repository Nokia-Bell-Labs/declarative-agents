{{/*
The chatbot rest.yaml, co-generated from .Values.ragUnits (srd015 R2). One
selected-target RAG operation serves every declared topology item; the provider
egress allowlist and monitor_proxy upstreams derive from the same list that
renders the RAG objects, so selected authority and deployment cannot drift. The
packaged agents/chatbot/rest.yaml stays the local integration source; this render
overrides that ConfigMap key in the cluster. Server addresses bind 0.0.0.0 so the
Services route to the pod. Runtime {{`{{ params.x }}`}} bodies are emitted
literally. The monitor server is not here: chatbotMonitorRest below renders it.
*/}}
{{- define "chatbot-mesh.chatbotRest" -}}
{{- $fullname := include "chatbot-mesh.fullname" . -}}
{{- $q := .Values.ragServer.ports.query -}}
{{- $mon := .Values.ragServer.ports.monitor -}}
{{- $llmURL := include "chatbot-mesh.llmURL" . -}}
{{- $llmHost := (urlParse $llmURL).hostname -}}
unit: mesh-chatbot-rest
expand:
  - fragment: /opt/agent-core/tools/rest/units/retrieval-query-client-fragment.yaml
    args:
      route_prefix: rag
      base_url_selector: $from(rag_unit).base_url
      input_selector: $from(normalize_query_embedding).mapped.embedding
      limits_ref: local_provider
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

  servers:
    chatbot_chat:
      address: 0.0.0.0:{{ .Values.chatbot.ports.chat }}
      limits_ref: local_chat_requests
      # A rollout asks the host machine to stop through the lifecycle endpoint.
      # Give an active machine_request the full endpoint bound plus drain
      # headroom before the old pod exits (GH-812).
      shutdown:
        timeout: 135s
        drain_policy: drain_then_stop
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
            # request-profile.yaml, not profile.yaml: the chat machine needs the
            # chat-LLM vocabulary in its tools selection, and the persistent
            # agent's profile must not carry it (GH-1900).
            profile: request-profile.yaml
            machine: request-machine.yaml
            timeout: 130s
            request:
              body:
                message: $.message
                history: $.history
            response:
              terminal_states:
                LLMResponded:
                  status: 200
                  content_type: application/json
                  body:
                    answer: $.answer
                    metadata: $.metadata
                Failed:
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
{{- /* enabled as well as implementation: with the collector off, the
       implementation default still reads "agent", and gating on it alone
       declared a monitor-proxy upstream at a Service the chart never renders.
       chatbot-mesh.otlpEndpoint checks both (GH-220). */}}
{{- if and .Values.collector.enabled (eq .Values.collector.implementation "agent") }}
              collector: http://{{ $fullname }}-collector:18193
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
            root: "${CHATBOT_UI_ROOT:-agents/chatbot/ui/app/dist}"
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
      # The exit route is agent-core's canonical one, injected at load
      # (GH-1264), as it is for the packaged profile.
      endpoints:
        health:
          method: GET
          path: /api/lifecycle/health
          binding: health
{{- end -}}

{{/*
The chatbot monitor-rest.yaml: an instantiation of agent-core's monitor server
fragment with the in-cluster bind address, and the limits that bound it. The
fragment is the one definition of the monitor surface, so the chart supplies
only what varies in the cluster, the address and its port; the views are the
eight every agent serves (GH-2183). It is its own file because the chatbot's
rest.yaml is shared with its request profile, which launches no monitor server
(srd052 R3.1).
*/}}
{{- define "chatbot-mesh.chatbotMonitorRest" -}}
unit: mesh-chatbot-monitor-rest
expand:
  - fragment: /opt/agent-core/tools/rest/units/monitor-server-fragment.yaml
    args: {address: "0.0.0.0:{{ .Values.chatbot.ports.monitor }}", limits_ref: local_chatbot_monitor, queue_name: chatbot_monitor}
rest:
  version: v1
  limits:
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
{{- end -}}
