{{/*
The observer monitor-rest.yaml, co-generated from .Values.ragUnits and the
collector toggle as the chatbot's rest.yaml and ui.yaml are (srd003 R2). The
monitor_proxy upstreams and the fleet UI's monitored agents derive from the same
list that renders the RAG objects, so the UI names exactly the agents the proxy
reaches (applications srd004 R2.1, R9.2). The upstreams are the in-cluster
Services, not loopback: the observer runs in its own pod. The packaged
agents/observer/monitor-rest.yaml stays the local integration source; this
render overrides that ConfigMap key in the cluster. The server binds 0.0.0.0 so
the Service routes to the pod.
*/}}
{{- define "chatbot-mesh.observerMonitorRest" -}}
{{- $fullname := include "chatbot-mesh.fullname" . -}}
{{- $collector := and .Values.collector.enabled (eq .Values.collector.implementation "agent") -}}
unit: mesh-observer-monitor-rest
rest:
  version: v1
  limits:
    monitor:
      timeout: 5s
      read_timeout: 5s
      max_request_bytes: 4096
      max_response_bytes: 1048576
      redirect: {mode: none}
      network:
        schemes: [http]
        hosts: [127.0.0.1, localhost]
        ports: [{{ .Values.observer.ports.monitor }}]
        allow_public_listener: true

  servers:
    observer_monitor:
      address: 0.0.0.0:{{ .Values.observer.ports.monitor }}
      limits_ref: monitor
      lifecycle_exit: {disabled: true}
      queue: {name: observer_monitor, capacity: 8, overflow: reject, timeout: 100ms}
      shutdown:
        timeout: 2s
        drain_policy: drain_then_stop
        stop_listeners: true
        unblock_await_signal: ServerStopped
      endpoints:
        machine_spec:  {method: GET, path: /monitor/machine, binding: read_state, monitor_view: machine_spec}
        declared_machines: {method: GET, path: /monitor/machines, binding: read_state, monitor_view: declared_machines}
        declared_tools: {method: GET, path: /monitor/tools/declared, binding: read_state, monitor_view: declared_tools}
        current_state: {method: GET, path: /monitor/state, binding: read_state, monitor_view: current_state}
        tools:         {method: GET, path: /monitor/tools, binding: read_state, monitor_view: tools}
        metrics:       {method: GET, path: /monitor/metrics, binding: read_state, monitor_view: metrics}
        recent_events: {method: GET, path: /monitor/events, binding: read_state, monitor_view: events}
        event_stream:  {method: GET, path: /monitor/events/stream, binding: stream_events, monitor_view: events}
        openapi:       {method: GET, path: /monitor/openapi, binding: static_metadata, monitor_view: openapi}
        fleet:
          method: GET
          path: /monitor/fleet
          binding: read_state
          monitor_view: command_state
          labels:
            - discover_mesh_pods
            - list_mesh_deployments
            - list_mesh_services
            - agent_machine_fanin
            - agent_state_fanin
            - agent_tools_fanin
            - agent_events_fanin
            - poll_pod_metrics
        monitor_proxy:
          method: GET
          path: /monitor-proxy/{agent}/{path...}
          binding: monitor_proxy
          monitor_proxy:
            upstreams:
              chatbot: http://{{ $fullname }}-chatbot:{{ .Values.chatbot.ports.monitor }}
{{- range $unit := .Values.ragUnits }}
              {{ $unit.name }}: http://{{ $fullname }}-{{ $unit.name }}:{{ $.Values.ragServer.ports.monitor }}
{{- end }}
{{- /* enabled as well as implementation, as the chatbot's upstream is gated,
       so a disabled collector declares no upstream at a Service the chart
       never renders (GH-220). */}}
{{- if $collector }}
              collector: http://{{ $fullname }}-collector:{{ .Values.collector.queryPort }}
{{- end }}
          request:
            path:
              agent: {type: string}
              path: {type: string}
        fleet_ui:
          method: GET
          path: /ui/{path...}
          binding: static_assets
          static_assets:
            bundle: observer
            spa: true
            config:
              title: Chatbot Mesh observer
              monitored_agents:
                - {name: chatbot, label: Chatbot}
{{- range $i, $unit := .Values.ragUnits }}
                - {name: {{ $unit.name }}, label: RAG server {{ $i }}}
{{- end }}
{{- if $collector }}
              trace_backend: collector
{{- end }}
          request:
            path:
              path: {type: string}
        root_redirect:
          method: GET
          path: /
          binding: redirect
          redirect: {location: /ui/, status: 302}
{{- end -}}
