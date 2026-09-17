{{/* Copyright (c) 2026 Nokia */}}
{{/* SPDX-License-Identifier: BSD-3-Clause */}}
{{/*
The trace collector every application installs: one agent-runtime Deployment
spooling or relaying OTLP, and its Service. The three applications hand-copied
this workload and drifted (GH-2045); the copies converge here.

Call with a dict carrying the root context and the app's own inputs:

  {{- include "agent-services.collector" (dict "root" . "profilePath" "/profiles/agents/collector/profile.yaml" "profilesVolume" $volume) }}

Required: root, profilePath, profilesVolume (a list of volume mappings).
Optional: image and imagePullPolicy (an app whose collector.image names a
different product passes the runtime image), workingDir, dataDir, spoolPath, serviceName (an OTel service name,
omitted when absent), podLabels, podAnnotations,
extraArgs, extraEnv, extraContainerPorts, extraVolumeMounts, extraVolumes,
initContainers, extraServicePorts.
*/}}
{{- define "agent-services.collector" -}}
{{- $root := .root -}}
{{- $values := $root.Values -}}
{{- $collector := $values.collector -}}
{{- $fullname := include "agent-services.fullname" $root -}}
{{- $dataDir := .dataDir | default "/data" -}}
{{- $spoolPath := .spoolPath | default (printf "%s/collector.ndjson" $dataDir) -}}
{{- $external := default "" $collector.externalOTLPEndpoint -}}
{{- $podAnnotations := merge (dict "checksum/config" (toYaml $collector | sha256sum)) (.podAnnotations | default dict) ($values.podAnnotations | default dict) -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $fullname }}-collector
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: collector
spec:
  replicas: 1
  {{- with $collector.progressDeadlineSeconds }}
  progressDeadlineSeconds: {{ . }}
  {{- end }}
  selector:
    matchLabels:
      {{- include "agent-services.selectorLabels" $root | nindent 6 }}
      app.kubernetes.io/component: collector
  template:
    metadata:
      labels:
        {{- include "agent-services.selectorLabels" $root | nindent 8 }}
        app.kubernetes.io/component: collector
        {{- range $key, $value := (.podLabels | default dict) }}
        {{ $key }}: {{ $value | quote }}
        {{- end }}
      annotations:
        {{- range $key, $value := $podAnnotations }}
        {{ $key }}: {{ $value | quote }}
        {{- end }}
    spec:
      automountServiceAccountToken: false
      {{- with $values.podSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .initContainers }}
      initContainers:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: collector
          image: {{ .image | default (include "agent-services.collectorImage" $root) | quote }}
          imagePullPolicy: {{ .imagePullPolicy | default $collector.image.pullPolicy }}
          workingDir: {{ .workingDir | default (dir .profilePath) | quote }}
          {{- with $values.containerSecurityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          args:
            - "--profile"
            - {{ .profilePath | quote }}
            - "--directory"
            - {{ $dataDir | quote }}
            {{- with .serviceName }}
            - "--otel-service-name"
            - {{ . | quote }}
            {{- end }}
            {{- with .extraArgs }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          env:
            - {name: COLLECTOR_BIND_HOST, value: "0.0.0.0"}
            - {name: COLLECTOR_RECEIVER_ADDRESS, value: "0.0.0.0:{{ $collector.otlpGRPCPort }}"}
            - {name: COLLECTOR_RELAY_ENDPOINT, value: {{ $external | quote }}}
            - {name: COLLECTOR_SPOOL_PATH, value: {{ $spoolPath | quote }}}
            - {name: COLLECTOR_MODE, value: {{ ternary "relay" "spool" (ne $external "") | quote }}}
            - {name: COLLECTOR_CONTROL_PORT, value: {{ $collector.controlPort | quote }}}
            - {name: COLLECTOR_MONITOR_PORT, value: {{ $collector.monitorPort | quote }}}
            - {name: COLLECTOR_QUERY_PORT, value: {{ $collector.queryPort | quote }}}
            {{- with .extraEnv }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          ports:
            - {name: otlp-grpc, containerPort: {{ $collector.otlpGRPCPort }}, protocol: TCP}
            - {name: control, containerPort: {{ $collector.controlPort }}, protocol: TCP}
            - {name: monitor, containerPort: {{ $collector.monitorPort }}, protocol: TCP}
            - {name: query, containerPort: {{ $collector.queryPort }}, protocol: TCP}
            {{- with .extraContainerPorts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          readinessProbe:
            httpGet: {path: /api/lifecycle/health, port: control}
            initialDelaySeconds: 2
            periodSeconds: 5
          livenessProbe:
            httpGet: {path: /api/lifecycle/health, port: control}
            initialDelaySeconds: 10
            periodSeconds: 15
          resources:
            {{- toYaml $collector.resources | nindent 12 }}
          volumeMounts:
            - {name: profiles, mountPath: {{ $values.profiles.mountPath }}, readOnly: true}
            - {name: spool, mountPath: {{ $dataDir }}}
            - {name: tmp, mountPath: /tmp}
            {{- with .extraVolumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
      volumes:
        {{- toYaml .profilesVolume | nindent 8 }}
        - name: spool
          emptyDir: {}
        - name: tmp
          emptyDir: {}
        {{- with .extraVolumes }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $fullname }}-collector
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: collector
spec:
  type: ClusterIP
  selector:
    {{- include "agent-services.selectorLabels" $root | nindent 4 }}
    app.kubernetes.io/component: collector
  ports:
    - {name: otlp-grpc, port: {{ $collector.otlpGRPCPort }}, targetPort: otlp-grpc, protocol: TCP}
    - {name: control, port: {{ $collector.controlPort }}, targetPort: control, protocol: TCP}
    - {name: query, port: {{ $collector.queryPort }}, targetPort: query, protocol: TCP}
    {{- with .extraServicePorts }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
{{- end -}}
