{{/*
The chatbot ux.yaml, co-generated from .Values.ragUnits (srd003 R2). The
monitored-agents list derives from the same RAG list as the topology and the
rest.yaml monitor_proxy upstreams, so the observability panel's per-agent
sub-panels track the deployed RAGs. The packaged ux/ux.yaml stays the local
source; this render overrides that ConfigMap key in the cluster.
*/}}
{{- define "chatbot-mesh.chatbotUX" -}}
id: chatbot-ui
title: Chatbot Agent UI
source_owner: agents/chatbot
routes:
  - id: chat
    path: /chat
    label: Chat
    action: chat_send
    resource: chat
  - id: observability
    path: /observability
    label: Observability
    action: observability_view
    resource: monitor
sidebar:
  title: Chatbot
  groups:
    chat:
      label: Chat
      order: 0
    observability:
      label: Observability
      order: 1
actions:
  chat_send:
    ui_action: chat_send
    request_machine_action: chat
    route: chat
    endpoint: /api/v1/chat
    method: POST
  observability_view:
    ui_action: observability_view
    route: observability
monitored_agents:
  - name: chatbot
    label: Chatbot
{{- range $i, $unit := .Values.ragUnits }}
  - name: {{ $unit.name }}
    label: RAG server {{ $i }}
{{- end }}
{{- if .Values.jaeger.enabled }}
trace_backend:
  name: jaeger
  query_path: /monitor-proxy/jaeger/api/traces/{trace_id}
{{- end }}
presentation:
  history_client_side: true
  source_citations: true
  degraded_indicator: true
  observability_per_agent_sse: true
  observability_turn_correlation: time-window
  observability_trace_waterfall: true
{{- end -}}
