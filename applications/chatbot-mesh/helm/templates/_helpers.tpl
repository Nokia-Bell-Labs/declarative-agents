{{/* Chart name. Not overridable: values.schema.json closes the root with
     additionalProperties false and declares no nameOverride, so any values file
     setting one is refused before a template runs. The branch that read it could
     never be reached (GH-220). */}}
{{- define "chatbot-mesh.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
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
{{- if .Values.ollama.enabled -}}
http://{{ include "chatbot-mesh.fullname" . }}-ollama:{{ .Values.llm.port }}
{{- else -}}
{{ .Values.llm.externalURL }}
{{- end -}}
{{- end -}}

{{/*
The full list of models the LLM tier must hold: the embedding model, every chat
model, and the tier-selector model, named once in values (GH-337). The preload Job pulls
these and the agent readiness probe checks for them, so a model cannot be preloaded
but unrendered or gated-on but unpulled.
*/}}
{{- define "chatbot-mesh.ollamaModels" -}}
{{- $models := list .Values.ollama.models.embedding .Values.ollama.models.tier -}}
{{- range .Values.ollama.models.chat -}}
{{- $models = append $models . -}}
{{- end -}}
{{- $models | uniq | join " " -}}
{{- end -}}

{{/*
The LLM-tier readiness init container (srd015 R6.3): an agent pod blocks in Init
until every declared model is present in the in-cluster Ollama /api/tags, so a
missing model is a deploy-time gate rather than a runtime turn failure. Rendered
only when the in-cluster tier is enabled; an external endpoint is the operator's to
have ready. busybox supplies wget and grep.
*/}}
{{- define "chatbot-mesh.llmReadyInit" -}}
{{- if .Values.ollama.enabled }}
- name: wait-for-llm-models
  image: busybox:1.36
  command: ["/bin/sh", "-c"]
  args:
    - |
      set -eu
      url="http://{{ include "chatbot-mesh.fullname" . }}-ollama:{{ .Values.llm.port }}/api/tags"
      for m in {{ include "chatbot-mesh.ollamaModels" . }}; do
        base="${m%%:*}"
        until wget -qO- "$url" 2>/dev/null | grep -q "$base"; do
          echo "waiting for model $m at $url..."; sleep 5
        done
      done
      echo "LLM tier ready: all models present"
{{- end }}
{{- end -}}

{{/* The OTLP endpoint agents export to: the collector, else empty. */}}
{{- define "chatbot-mesh.otlpEndpoint" -}}
{{- if .Values.collector.enabled -}}
{{ include "chatbot-mesh.fullname" . }}-collector:{{ .Values.collector.otlpGRPCPort }}
{{- end -}}
{{- end -}}

{{/*
In production agent mode the declarative collector owns both trace and OTLP
metric intake (GH-1207). Integration overlays may send agent metrics directly to
the persistent external collector because the declarative collector does not
relay its metric spool. Traces continue through the local collector and its
declared relay. The contrib collector-metrics gateway remains retired (GH-1366).
*/}}
{{- define "chatbot-mesh.otlpMetricEndpoint" -}}
{{- if and .Values.collector.enabled (eq .Values.collector.implementation "agent") -}}
{{- if .Values.collector.externalOTLPEndpoint -}}
{{ .Values.collector.externalOTLPEndpoint }}
{{- else -}}
{{ include "chatbot-mesh.fullname" . }}-collector:{{ .Values.collector.otlpGRPCPort }}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "chatbot-mesh.integrationResourceAttributes" -}}
{{- if .Values.collector.externalOTLPEndpoint -}}
test.repository={{ .Values.collector.integrationResource.repository }},test.module={{ .Values.collector.integrationResource.module }},test.target={{ .Values.collector.integrationResource.target }},vcs.ref.head.revision={{ .Values.collector.integrationResource.commit }},test.run.id={{ .Values.collector.integrationResource.runID }}
{{- end -}}
{{- end -}}

{{/*
chatbot-mesh.cogeneratedProfileKeys names the profile ConfigMap keys rendered
from values rather than packaged from disk, as a space-separated list.

One definition, because the two readers disagreed and the disagreement was
invisible: the ConfigMap excluded these keys from its file glob so the rendered
versions win, while the volume projection listed a different set -- naming two
files that are packaged verbatim and omitting the topology, which is actually
co-generated. The topology key reached the pod only because a packaging step had
left the staged file on disk for the glob to find (cohere-demo GH-220).
*/}}
{{- define "chatbot-mesh.cogeneratedProfileKeys" -}}
agents__chatbot__rest.yaml agents__chatbot__ui__ui.yaml agents__chatbot__request-topology-declarations.yaml
{{- end -}}

{{/*
The MySQL-wire DSN to the Dolt sql-server checkpoint backend (agent-core
srd035/srd036), or empty when Dolt is disabled. The chatbot persists checkpoints
here for explicit history, rollback, and resume operations. A Deployment rollout
drains active HTTP turns; persistence cannot reattach an existing client socket.
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
    {{- $cogen := splitList " " (include "chatbot-mesh.cogeneratedProfileKeys" .) }}
    {{- range $path, $_ := .Files.Glob "profiles/**" }}
      {{- $key := $path | trimPrefix "profiles/" | replace "/" "__" }}
      {{- /* Served UI bundles are projected from their own ConfigMaps by the
            agents that serve them, so they are not items of this shared
            projection (GH-131). */}}
      {{- if and (not (has $key $cogen)) (not (hasPrefix "agents__observer__ui__dist__" $key)) (not (hasPrefix "agents__chatbot__ui__app__dist__" $key)) }}
      - key: {{ $key }}
        path: {{ $path | trimPrefix "profiles/" }}
      {{- end }}
    {{- end }}
      {{- /* The co-generated keys, projected whether or not a packaging step placed the file on disk. */}}
      {{- range $key := splitList " " (include "chatbot-mesh.cogeneratedProfileKeys" $) }}
      - {key: {{ $key }}, path: {{ $key | replace "__" "/" }}}
      {{- end }}
{{- end -}}
