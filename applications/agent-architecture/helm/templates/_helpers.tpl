{{- define "agent-architecture.name" -}}
{{- include "agent-services.name" . -}}
{{- end -}}

{{- define "agent-architecture.fullname" -}}
{{- include "agent-services.fullname" . -}}
{{- end -}}

{{- define "agent-architecture.labels" -}}
{{- include "agent-services.labels" . -}}
{{- end -}}

{{- define "agent-architecture.selectorLabels" -}}
{{- include "agent-services.selectorLabels" . -}}
{{- end -}}

{{- define "agent-architecture.image" -}}
{{- include "agent-services.image" . -}}
{{- end -}}

{{- define "agent-architecture.collectorImage" -}}
{{- include "agent-services.collectorImage" . -}}
{{- end -}}

{{- define "agent-architecture.roleProfile" -}}
{{- $prepared := .root.Files.Get "profiles/prepared-manifest.yaml" | fromYaml -}}
{{- if not $prepared -}}
{{- fail "no prepared manifest; run mage helmPrepare" -}}
{{- end -}}
{{- $profile := "" -}}
{{- range $entry := $prepared.roles -}}
{{- if eq $entry.role $.role -}}
{{- $profile = printf "%s/%s" $prepared.mount_path $entry.profile -}}
{{- end -}}
{{- end -}}
{{- if not $profile -}}
{{- fail (printf "chart role %s is not declared by agents/application.yaml" .role) -}}
{{- end -}}
{{- $profile -}}
{{- end -}}

{{- define "agent-architecture.otlpEndpoint" -}}
{{- include "agent-services.otlpEndpoint" . -}}
{{- end -}}

{{- define "agent-architecture.validateValues" -}}
{{- if ne .Values.profiles.mountPath "/profiles" -}}
{{- fail "profiles.mountPath must be /profiles" -}}
{{- end -}}
{{- $ports := dict
  "curator documentation" .Values.curator.documentationPort
  "curator control" .Values.curator.controlPort
  "curator monitor" .Values.curator.monitorPort -}}
{{- if .Values.collector.enabled -}}
{{- $_ := set $ports "collector otlp" .Values.collector.otlpGRPCPort -}}
{{- $_ := set $ports "collector control" .Values.collector.controlPort -}}
{{- $_ := set $ports "collector monitor" .Values.collector.monitorPort -}}
{{- $_ := set $ports "collector query" .Values.collector.queryPort -}}
{{- end -}}
{{- if .Values.applier.enabled -}}
{{- $_ := set $ports "applier apply" .Values.applier.ports.apply -}}
{{- $_ := set $ports "applier control" .Values.applier.ports.control -}}
{{- $_ := set $ports "applier monitor" .Values.applier.ports.monitor -}}
{{- end -}}
{{- $seen := dict -}}
{{- range $name, $port := $ports -}}
{{- if hasKey $seen ($port | toString) -}}
{{- fail (printf "port conflict at %v (%s and %s)" $port (index $seen ($port | toString)) $name) -}}
{{- end -}}
{{- $_ := set $seen ($port | toString) $name -}}
{{- end -}}
{{- end -}}
