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
{{/* The versioned collector-storage contract (srd042 R10/R11; srd008). The
     filesystem backend preserves the emptyDir NDJSON spool; the object backend
     wires the application bucket into the collector's declared config channel
     and mounts a persistent WAL. Every setting is validated at render time so a
     bad identity or mode fails helm, not the pod (GH-2484). */}}
{{- $storage := $collector.storage | default dict -}}
{{- $backend := $storage.backend | default "filesystem" -}}
{{- if not (has $backend (list "filesystem" "object")) -}}
{{- fail (printf "collector.storage.backend %q must be filesystem or object" $backend) -}}
{{- end -}}
{{- $storageMode := $storage.mode | default (ternary "durable" "ephemeral" (eq $backend "object")) -}}
{{- $walMode := $storage.walMode | default (ternary "persistent" "ephemeral" (eq $storageMode "durable")) -}}
{{- if and (eq $storageMode "durable") (eq $walMode "ephemeral") -}}
{{- fail "collector.storage: durable mode requires a persistent WAL; set walMode: persistent, or mode: ephemeral for a test collector" -}}
{{- end -}}
{{- $walPath := $storage.walPath | default (printf "%s/wal" $dataDir) -}}
{{- $storageApp := $storage.application | default (include "agent-services.name" $root) -}}
{{- if and $storage.namespace (ne (toString $storage.namespace) $root.Release.Namespace) -}}
{{- fail (printf "collector.storage.namespace %q must be the release namespace %q" (toString $storage.namespace) $root.Release.Namespace) -}}
{{- end -}}
{{- $storageNamespace := $storage.namespace | default $root.Release.Namespace -}}
{{- $bucketURL := $storage.bucketURL | default "" -}}
{{- if and (eq $bucketURL "") ($storage.bucketName | default "") -}}
{{- $bucketURL = printf "gs://%s" $storage.bucketName -}}
{{- end -}}
{{- $endpoint := $storage.endpoint | default "" -}}
{{- if eq $backend "object" -}}
{{- if eq $bucketURL "" -}}{{- fail "collector.storage: object backend requires bucketURL or bucketName" -}}{{- end -}}
{{- if eq (trim $storageApp) "" -}}{{- fail "collector.storage: object backend requires a non-empty application identity" -}}{{- end -}}
{{- $scheme := first (regexSplit "://" $bucketURL -1) -}}
{{- if not (has $scheme (list "gs" "s3" "file" "mem")) -}}{{- fail (printf "collector.storage.bucketURL %q must use scheme gs, s3, file, or mem" $bucketURL) -}}{{- end -}}
{{- else if ne $endpoint "" -}}
{{- fail "collector.storage.endpoint is only valid with the object backend (a local emulator)" -}}
{{- end -}}
{{- if $storage.prefix -}}
{{- if or (hasPrefix "/" (toString $storage.prefix)) (contains ".." (toString $storage.prefix)) -}}
{{- fail (printf "collector.storage.prefix %q must be a safe relative in-bucket prefix" (toString $storage.prefix)) -}}
{{- end -}}
{{- end -}}
{{- $storagePrefix := $storage.prefix | default $storageApp -}}
{{- $retentionClass := $storage.retentionClass | default "application" -}}
{{- $retentionDays := $storage.retentionDays | default 0 -}}
{{- if lt (int $retentionDays) 0 -}}
{{- fail "collector.storage.retentionDays must not be negative" -}}
{{- end -}}
{{- $walVolume := ternary "wal-persistent" "wal-ephemeral" (eq $walMode "persistent") -}}
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
          imagePullPolicy: {{ include "agent-services.agentPullPolicy" (dict "root" $root "image" $collector.image "override" (.imagePullPolicy | default "")) }}
          workingDir: {{ .workingDir | default (dir .profilePath) }}
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
            {{/* The declared storage channel (srd042 R10.1/R11.1), rendered
                 only for the object backend so the filesystem collector is
                 byte-identical to before (R7). These flow into the collector's
                 declarations.yaml through ${VAR} substitution, never os.Getenv:
                 the chart is the single wiring boundary (R2). */}}
            {{- if eq $backend "object" }}
            - {name: COLLECTOR_STORAGE_BACKEND, value: {{ $backend | quote }}}
            - {name: COLLECTOR_OBJECT_BUCKET_URL, value: {{ $bucketURL | quote }}}
            - {name: COLLECTOR_OBJECT_ENDPOINT, value: {{ $endpoint | quote }}}
            - {name: COLLECTOR_OBJECT_PREFIX, value: {{ $storagePrefix | quote }}}
            - {name: COLLECTOR_TRACE_WAL_PATH, value: {{ printf "%s/traces.wal" $walPath | quote }}}
            - {name: COLLECTOR_METRICS_WAL_PATH, value: {{ printf "%s/metrics.wal" $walPath | quote }}}
            - {name: COLLECTOR_APPLICATION, value: {{ $storageApp | quote }}}
            - {name: COLLECTOR_NAMESPACE, value: {{ $storageNamespace | quote }}}
            - {name: COLLECTOR_RUN, value: {{ $root.Release.Name | quote }}}
            - {name: COLLECTOR_INSTANCE, value: {{ printf "%s-collector" $fullname | quote }}}
            - {name: COLLECTOR_RETENTION_CLASS, value: {{ $retentionClass | quote }}}
            - {name: COLLECTOR_RETENTION_DAYS, value: {{ toString $retentionDays | quote }}}
            {{- end }}
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
          {{/* Readiness and liveness probe intake and lifecycle health on the
               control server, never the remote object store (srd042 R6; GH-2484
               R6). A transient bucket outage leaves the WAL absorbing writes
               durably, so a WAL-backed collector stays ready; the query surface
               reports the degraded storage_status (rest.yaml /query/*) rather
               than letting the pod lie about persisted history by flipping
               unready. The probe target is storage-backend independent: object
               mode does not repoint it. */}}
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
            {{- if eq $backend "object" }}
            - {name: {{ $walVolume }}, mountPath: {{ $walPath }}}
            {{- end }}
            - {name: tmp, mountPath: /tmp}
            {{- with .extraVolumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
      volumes:
        {{- toYaml .profilesVolume | nindent 8 }}
        - name: spool
          emptyDir: {}
        {{/* The WAL is mounted apart from the remote bucket (srd008 R2, R6):
             a persistent claim in durable mode so bucket outage or a pod
             restart keeps replayable evidence; an emptyDir only in an
             explicitly ephemeral test collector (srd042 R10.4; GH-2484 R3). */}}
        {{- if eq $backend "object" }}
        {{- if eq $walMode "persistent" }}
        - name: {{ $walVolume }}
          persistentVolumeClaim:
            claimName: {{ $fullname }}-collector-wal
        {{- else }}
        - name: {{ $walVolume }}
          emptyDir: {}
        {{- end }}
        {{- end }}
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
{{- if and (eq $backend "object") (eq $walMode "persistent") }}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ $fullname }}-collector-wal
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: collector
spec:
  accessModes: [ReadWriteOnce]
  {{- with $storage.walStorageClass }}
  storageClassName: {{ . | quote }}
  {{- end }}
  resources:
    requests:
      storage: {{ $storage.walCapacity | default "1Gi" | quote }}
{{- end }}
{{- end -}}
