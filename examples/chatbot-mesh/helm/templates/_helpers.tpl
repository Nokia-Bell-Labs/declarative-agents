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
{{- if .Values.ollama.enabled -}}
http://{{ include "chatbot-mesh.fullname" . }}-ollama:{{ .Values.llm.port }}
{{- else -}}
{{ .Values.llm.externalURL }}
{{- end -}}
{{- end -}}

{{/*
The full list of models the LLM tier must hold: the embedding model, every chat
model, and the router model, named once in values (GH-337). The preload Job pulls
these and the agent readiness probe checks for them, so a model cannot be preloaded
but unrendered or gated-on but unpulled.
*/}}
{{- define "chatbot-mesh.ollamaModels" -}}
{{- $models := list .Values.ollama.models.embedding .Values.ollama.models.router -}}
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
The mesh view (srd003 R4) the provisioner serves on its read path, projected as
JSON from the same values that render the topology, so what the panel reads is
what the chart deploys. Values-plane only: RAG list, LLM endpoint, parameters. No
per-agent runtime endpoint appears, so the read state carries no agent authority.
*/}}
{{- define "chatbot-mesh.meshView" -}}
{{- $rags := list -}}
{{- range .Values.ragUnits -}}
{{- $rags = append $rags (dict "name" .name "collection" .collection "embeddingModel" .embeddingModel "replicas" (int (default 1 .replicas))) -}}
{{- end -}}
{{- $view := dict
  "rags" $rags
  "llm" (dict "inCluster" .Values.ollama.enabled "externalURL" .Values.llm.externalURL "chatModel" (default "" .Values.provisioner.params.chatModel) "embedModel" .Values.chatbot.embeddingModel "chatModels" .Values.ollama.models.chat "routerModel" .Values.ollama.models.router "topology" .Values.ollama.topology)
  "params" (dict "nResults" (int .Values.provisioner.params.nResults) "chunkCap" (int .Values.provisioner.params.chunkCap) "routerDefault" .Values.provisioner.params.routerDefault)
-}}
{{- $view | toJson -}}
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
    {{- $cogen := list "agents__chatbot__rest.yaml" "ux__ux.yaml" "agents__chatbot__request-machine.yaml" "agents__chatbot__request-fanout.yaml" }}
    {{- range $path, $_ := .Files.Glob "profiles/**" }}
      {{- $key := $path | trimPrefix "profiles/" | replace "/" "__" }}
      {{- if not (has $key $cogen) }}
      - key: {{ $key }}
        path: {{ $path | trimPrefix "profiles/" }}
      {{- end }}
    {{- end }}
      {{- /* The co-generated keys, projected whether or not a packaging step placed the file on disk. */}}
      - {key: agents__chatbot__rest.yaml, path: agents/chatbot/rest.yaml}
      - {key: ux__ux.yaml, path: ux/ux.yaml}
      - {key: agents__chatbot__request-machine.yaml, path: agents/chatbot/request-machine.yaml}
      - {key: agents__chatbot__request-fanout.yaml, path: agents/chatbot/request-fanout.yaml}
{{- end -}}
