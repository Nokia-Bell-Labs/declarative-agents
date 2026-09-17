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
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}

{{- define "agent-services.collectorImage" -}}
{{- printf "%s:%s" .Values.collector.image.repository .Values.collector.image.tag -}}
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
