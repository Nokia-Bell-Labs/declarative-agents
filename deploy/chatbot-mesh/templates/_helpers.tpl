{{/* Chart name, optionally overridden. */}}
{{- define "chatbot-mesh.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Fully qualified release name. */}}
{{- define "chatbot-mesh.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "chatbot-mesh.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Common labels. */}}
{{- define "chatbot-mesh.labels" -}}
app.kubernetes.io/name: {{ include "chatbot-mesh.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{/* Selector labels for a component. */}}
{{- define "chatbot-mesh.selectorLabels" -}}
app.kubernetes.io/name: {{ include "chatbot-mesh.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/* Component resource name: <fullname>-<component>. */}}
{{- define "chatbot-mesh.component" -}}
{{- printf "%s-%s" (include "chatbot-mesh.fullname" .root) .name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* The agent runtime image reference. */}}
{{- define "chatbot-mesh.image" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}

{{/* The LLM base URL: in-cluster Ollama Service or the external endpoint. */}}
{{- define "chatbot-mesh.llmURL" -}}
{{- if .Values.llm.inCluster -}}
http://{{ include "chatbot-mesh.fullname" . }}-ollama:{{ .Values.llm.port }}
{{- else -}}
{{ .Values.llm.externalURL }}
{{- end -}}
{{- end -}}

{{/* The OTLP endpoint agents export to: the collector, else empty. */}}
{{- define "chatbot-mesh.otlpEndpoint" -}}
{{- if .Values.collector.enabled -}}
{{ include "chatbot-mesh.fullname" . }}-collector:{{ .Values.collector.otlpGRPCPort }}
{{- end -}}
{{- end -}}

{{/*
The MySQL-wire DSN to the Dolt sql-server checkpoint backend (agent-core
srd035/srd036), or empty when Dolt is disabled. The chatbot persists its host
machine's checkpoints here, so a rollout resumes from durable state rather than
cold-starting.
*/}}
{{- define "chatbot-mesh.doltDSN" -}}
{{- if .Values.dolt.enabled -}}
{{ .Values.dolt.user }}@tcp({{ include "chatbot-mesh.fullname" . }}-dolt:{{ .Values.dolt.port }})/{{ .Values.dolt.database }}
{{- end -}}
{{- end -}}

{{/*
The profiles volume. Profile files live under the chart's profiles/ subtree and
are packaged into one ConfigMap with "/" in each path encoded as "__" (ConfigMap
keys cannot contain "/"). The volume projects each key back to its nested path
via items[].path, so the agent sees the original agents/<name>/... tree at the
mount. GH-314 co-generates the chatbot rest.yaml into this subtree before packaging.
*/}}
{{- define "chatbot-mesh.profilesVolume" -}}
- name: profiles
  configMap:
    name: {{ include "chatbot-mesh.fullname" . }}-profiles
    items:
    {{- range $path, $_ := .Files.Glob "profiles/**" }}
      {{- $key := $path | trimPrefix "profiles/" | replace "/" "__" }}
      {{- if and (ne $key "agents__chatbot__rest.yaml") (ne $key "agents__chatbot__ui__ux.yaml") }}
      - key: {{ $key }}
        path: {{ $path | trimPrefix "profiles/" }}
      {{- end }}
    {{- end }}
      {{- /* The co-generated keys, projected whether or not a packaging step placed the file on disk. */}}
      - {key: agents__chatbot__rest.yaml, path: agents/chatbot/rest.yaml}
      - {key: agents__chatbot__ui__ux.yaml, path: agents/chatbot/ui/ux.yaml}
{{- end -}}
