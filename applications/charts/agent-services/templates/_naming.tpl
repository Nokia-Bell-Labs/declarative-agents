{{/* Copyright (c) 2026 Nokia */}}
{{/* SPDX-License-Identifier: BSD-3-Clause */}}
{{/*
Naming, labelling, and image helpers every application chart shares. An app
chart keeps its own "<app>.name"-style defines as one-line wrappers around
these, so its templates read unchanged and only the bodies live here (GH-2045).
*/}}

{{- define "agent-services.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "agent-services.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "agent-services.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "agent-services.labels" -}}
app.kubernetes.io/name: {{ include "agent-services.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
{{- end -}}

{{- define "agent-services.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agent-services.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "agent-services.image" -}}
{{- include "agent-services.pinnedImage" .Values.image -}}
{{- end -}}

{{- define "agent-services.collectorImage" -}}
{{- include "agent-services.pinnedImage" .Values.collector.image -}}
{{- end -}}

{{/*
One image reference from an image map, carrying the digest when the map sets
one. ENG01 C2 has every image the cluster pulls resolve through a digest: the
tag names the version a reader recognizes, the digest is what the cluster
resolves, and a tag that moves upstream cannot change what installs. An image
this checkout builds sets no digest, because the digest does not exist until
the image is built (srd005 R2.1, R2.2).

Takes the image map itself, not the root context:

  {{ include "agent-services.pinnedImage" .Values.dolt.image }}
*/}}
{{- define "agent-services.pinnedImage" -}}
{{- $image := printf "%s:%s" .repository .tag -}}
{{- with .digest }}{{ $image = printf "%s@%s" $image . }}{{ end -}}
{{- $image -}}
{{- end -}}

{{/*
The collector's OTLP gRPC address, empty when no collector is installed, so a
workload renders no endpoint rather than one pointing at nothing.
*/}}
{{- define "agent-services.otlpEndpoint" -}}
{{- if .Values.collector.enabled -}}
{{- printf "%s-collector:%v" (include "agent-services.fullname" .) .Values.collector.otlpGRPCPort -}}
{{- end -}}
{{- end -}}
