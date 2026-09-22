{{- define "coding-agent.name" -}}
{{- include "agent-services.name" . -}}
{{- end -}}

{{- define "coding-agent.fullname" -}}
{{- include "agent-services.fullname" . -}}
{{- end -}}

{{- define "coding-agent.labels" -}}
{{- include "agent-services.labels" . -}}
{{- end -}}

{{- define "coding-agent.selectorLabels" -}}
{{- include "agent-services.selectorLabels" . -}}
{{- end -}}

{{- define "coding-agent.image" -}}
{{- include "agent-services.image" . -}}
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
{{- include "agent-services.otlpEndpoint" . -}}
{{- end -}}

{{- define "coding-agent.collectorImage" -}}
{{- include "agent-services.collectorImage" . -}}
{{- end -}}

{{- define "coding-agent.executorGoImage" -}}
{{- include "agent-services.pinnedImage" .Values.executorTools.go.image -}}
{{- end -}}

{{- define "coding-agent.executorGolangciLintImage" -}}
{{- include "agent-services.pinnedImage" .Values.executorTools.golangciLint.image -}}
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
{{- $ports := dict "planner request" .Values.roles.planner.requestPort "planner control" .Values.roles.planner.controlPort "executor request" .Values.roles.executor.requestPort "executor control" .Values.roles.executor.controlPort "critic request" .Values.roles.critic.requestPort "critic control" .Values.roles.critic.controlPort "applier apply" .Values.applier.ports.apply "applier control" .Values.applier.ports.control "applier monitor" .Values.applier.ports.monitor -}}
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
