{{- define "agent-architecture.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "agent-architecture.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "agent-architecture.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "agent-architecture.labels" -}}
app.kubernetes.io/name: {{ include "agent-architecture.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "agent-architecture.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agent-architecture.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "agent-architecture.image" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}

{{- define "agent-architecture.collectorImage" -}}
{{- printf "%s:%s" .Values.collector.image.repository .Values.collector.image.tag -}}
{{- end -}}

{{- define "agent-architecture.otlpEndpoint" -}}
{{- if .Values.collector.enabled -}}
{{- printf "%s-collector:%v" (include "agent-architecture.fullname" .) .Values.collector.otlpGRPCPort -}}
{{- end -}}
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
{{- $seen := dict -}}
{{- range $name, $port := $ports -}}
{{- if hasKey $seen ($port | toString) -}}
{{- fail (printf "port conflict at %v (%s and %s)" $port (index $seen ($port | toString)) $name) -}}
{{- end -}}
{{- $_ := set $seen ($port | toString) $name -}}
{{- end -}}
{{- end -}}
