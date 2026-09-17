{{/* Copyright (c) 2026 Nokia */}}
{{/* SPDX-License-Identifier: BSD-3-Clause */}}
{{/*
The in-cluster Ollama tier: the model server StatefulSet, its Service, and the
preload Job that pulls the declared models. Two applications hand-copied this
workload and drifted (GH-2045); the copies converge here.

Call with the root context and the app's model list:

  {{- include "agent-services.ollama" (dict "root" . "models" (include "<app>.ollamaModels" .)) }}

Required: root, models (a space-separated model list).
Optional: podLabels.

An integration run may point ollama.preload.integrationCacheHostPath at a host
directory holding a warmed model cache; the server then mounts that directory
instead of a claim, and the preload restores and saves it.
*/}}
{{- define "agent-services.ollama" -}}
{{- $root := .root -}}
{{- $values := $root.Values -}}
{{- $ollama := $values.ollama -}}
{{- $fullname := include "agent-services.fullname" $root -}}
{{- $preload := default dict $ollama.preload -}}
{{- $cache := default "" $preload.integrationCacheHostPath -}}
{{- $models := .models -}}
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ $fullname }}-ollama
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: ollama
spec:
  serviceName: {{ $fullname }}-ollama
  replicas: 1
  selector:
    matchLabels:
      {{- include "agent-services.selectorLabels" $root | nindent 6 }}
      app.kubernetes.io/component: ollama
  template:
    metadata:
      labels:
        {{- include "agent-services.selectorLabels" $root | nindent 8 }}
        app.kubernetes.io/component: ollama
        {{- range $key, $value := (.podLabels | default dict) }}
        {{ $key }}: {{ $value | quote }}
        {{- end }}
    spec:
      automountServiceAccountToken: false
      {{- with $ollama.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $ollama.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: ollama
          image: "{{ $ollama.image.repository }}:{{ $ollama.image.tag }}"
          imagePullPolicy: {{ $ollama.image.pullPolicy }}
          {{- with $values.containerSecurityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          ports:
            - {name: http, containerPort: {{ $values.llm.port }}, protocol: TCP}
          readinessProbe:
            httpGet: {path: /api/tags, port: http}
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            {{- $resources := deepCopy $ollama.resources }}
            {{- if gt (int $ollama.gpu.count) 0 }}
            {{- $_ := set (index $resources "limits") "nvidia.com/gpu" ($ollama.gpu.count | toString) }}
            {{- end }}
            {{- toYaml $resources | nindent 12 }}
          volumeMounts:
            - {name: models, mountPath: /root/.ollama}
            - {name: tmp, mountPath: /tmp}
      volumes:
        - name: tmp
          emptyDir: {}
        {{- if $cache }}
        - name: models
          hostPath:
            path: {{ printf "%s/active" $cache | quote }}
            type: Directory
        {{- end }}
  {{- if not $cache }}
  volumeClaimTemplates:
    - metadata:
        name: models
      spec:
        accessModes: [ReadWriteOnce]
        {{- with $ollama.persistence.storageClass }}
        storageClassName: {{ . | quote }}
        {{- end }}
        resources:
          requests:
            storage: {{ $ollama.persistence.size }}
  {{- end }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $fullname }}-ollama
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: ollama
spec:
  type: ClusterIP
  selector:
    {{- include "agent-services.selectorLabels" $root | nindent 4 }}
    app.kubernetes.io/component: ollama
  ports:
    - {name: http, port: {{ $values.llm.port }}, targetPort: http, protocol: TCP}
---
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ $fullname }}-ollama-preload
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: ollama-preload
  annotations:
    # Re-render the Job on a model-list change so an added model is pulled;
    # ollama pull is idempotent, so a re-run over present models is a no-op.
    checksum/models: {{ $models | sha256sum }}
spec:
  suspend: {{ default false $preload.suspend }}
  backoffLimit: 6
  template:
    metadata:
      labels:
        {{- include "agent-services.selectorLabels" $root | nindent 8 }}
        app.kubernetes.io/component: ollama-preload
    spec:
      restartPolicy: OnFailure
      automountServiceAccountToken: false
      containers:
        - name: preload
          image: "{{ $ollama.image.repository }}:{{ $ollama.image.tag }}"
          imagePullPolicy: {{ $ollama.image.pullPolicy }}
          {{- with $values.containerSecurityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          env:
            - {name: OLLAMA_HOST, value: "http://{{ $fullname }}-ollama:{{ $values.llm.port }}"}
          command: ["/bin/sh", "-c"]
          args:
            - |
              set -eu
              {{- if $cache }}
              if [ -f /cache/seeded ] || [ -f /cache/ready ]; then
                echo "restoring identity-matched integration model cache"
                cp -a /cache/models/. /models/
              fi
              {{- end }}
              # The ollama image ships neither wget nor curl, so reachability is
              # probed with the ollama CLI itself (it honors OLLAMA_HOST), not an
              # HTTP client that is absent from the image.
              until ollama list >/dev/null 2>&1; do
                echo "waiting for ollama at $OLLAMA_HOST..."; sleep 3
              done
              for m in {{ $models }}; do
                echo "pulling $m"; ollama pull "$m"
              done
              {{- if $cache }}
              rm -rf /cache/models
              mkdir -p /cache/models
              cp -a /models/. /cache/models/
              touch /cache/ready
              sync
              {{- end }}
              echo "preload complete"
          volumeMounts:
            - {name: tmp, mountPath: /tmp}
            {{- if $cache }}
            - {name: active-models, mountPath: /models}
            - {name: integration-model-cache, mountPath: /cache}
            {{- end }}
      volumes:
        - name: tmp
          emptyDir: {}
        {{- if $cache }}
        - name: active-models
          hostPath:
            path: {{ printf "%s/active" $cache | quote }}
            type: Directory
        - name: integration-model-cache
          hostPath:
            path: {{ $cache | quote }}
            type: Directory
        {{- end }}
{{- end -}}
