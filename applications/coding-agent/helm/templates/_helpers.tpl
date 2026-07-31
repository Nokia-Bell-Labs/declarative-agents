{{- define "coding-agent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "coding-agent.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "coding-agent.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "coding-agent.labels" -}}
app.kubernetes.io/name: {{ include "coding-agent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "coding-agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "coding-agent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "coding-agent.image" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}

{{- define "coding-agent.roleManifest" -}}
{{- $path := printf "profiles/manifests/%s.yaml" .role -}}
{{- required (printf "missing prepared role manifest %s; run mage helmPrepare" $path) (.root.Files.Get $path) -}}
{{- end -}}

{{- define "coding-agent.profileChecksum" -}}
{{- $root := .root -}}
{{- $role := .role -}}
{{- $manifest := include "coding-agent.roleManifest" . | fromYaml -}}
{{- $content := "" -}}
{{- range $manifest.files -}}
{{- $path := printf "profiles/%s/%s" $role . -}}
{{- $content = printf "%s\n%s\n%s" $content $path ($root.Files.Get $path) -}}
{{- end -}}
{{- $content | sha256sum -}}
{{- end -}}

{{- define "coding-agent.profilesVolume" -}}
{{- $root := .root -}}
{{- $role := .role -}}
{{- $manifest := include "coding-agent.roleManifest" . | fromYaml -}}
- name: profiles
  projected:
    defaultMode: 0444
    sources:
    {{- range $partition := $manifest.config_maps }}
      - configMap:
          name: {{ include "coding-agent.fullname" $root }}-{{ $role }}-profiles-{{ $partition.index }}
          items:
          {{- range $partition.files }}
            - key: {{ . | replace "/" "__" }}
              path: {{ . }}
          {{- end }}
    {{- end }}
{{- end -}}

{{- define "coding-agent.workspaceClaim" -}}
{{- if .Values.workspace.existingClaim -}}
{{- .Values.workspace.existingClaim -}}
{{- else -}}
{{- printf "%s-workspace" (include "coding-agent.fullname" .) -}}
{{- end -}}
{{- end -}}

{{- define "coding-agent.llmURL" -}}
{{- if .Values.ollama.enabled -}}
{{- printf "http://%s-ollama:%v" (include "coding-agent.fullname" .) .Values.llm.port -}}
{{- else -}}
{{- .Values.llm.externalURL -}}
{{- end -}}
{{- end -}}

{{- define "coding-agent.otlpEndpoint" -}}
{{- if .Values.collector.enabled -}}
{{- printf "%s-collector:%v" (include "coding-agent.fullname" .) .Values.collector.otlpGRPCPort -}}
{{- end -}}
{{- end -}}

{{- define "coding-agent.collectorImage" -}}
{{- printf "%s:%s" .Values.collector.image.repository .Values.collector.image.tag -}}
{{- end -}}

{{- define "coding-agent.ollamaModels" -}}
{{- .Values.ollama.models | uniq | join " " -}}
{{- end -}}

{{- define "coding-agent.validateValues" -}}
{{- if ne .Values.profiles.mountPath "/profiles" -}}
{{- fail "profiles.mountPath must be /profiles" -}}
{{- end -}}
{{- if ne .Values.workspace.mountPath "/work" -}}
{{- fail "workspace.mountPath must be /work" -}}
{{- end -}}
{{- $ports := dict "planner request" .Values.roles.planner.requestPort "planner control" .Values.roles.planner.controlPort "executor request" .Values.roles.executor.requestPort "executor control" .Values.roles.executor.controlPort "critic request" .Values.roles.critic.requestPort "critic control" .Values.roles.critic.controlPort -}}
{{- $seen := dict -}}
{{- range $name, $port := $ports -}}
{{- if hasKey $seen ($port | toString) -}}
{{- fail (printf "role ports conflict at %v (%s and %s)" $port (index $seen ($port | toString)) $name) -}}
{{- end -}}
{{- $_ := set $seen ($port | toString) $name -}}
{{- end -}}
{{- if and (not .Values.ollama.enabled) (not .Values.llm.externalURL) -}}
{{- fail "llm.externalURL is required when ollama is disabled" -}}
{{- end -}}
{{- if and .Values.ollama.enabled (eq (len .Values.ollama.models) 0) -}}
{{- fail "ollama.models must not be empty" -}}
{{- end -}}
{{- end -}}
