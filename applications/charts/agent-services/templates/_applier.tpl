{{/* Copyright (c) 2026 Nokia */}}
{{/* SPDX-License-Identifier: BSD-3-Clause */}}
{{/*
The applier (srd006): the deployment-plane actuation agent. It runs with a mounted
profile whose request machines bind the helm and kubectl CLIs as exec words, and
it edits deployment values and triggers rollouts only. The chart it upgrades is
not baked into the image (GH-1368): deployment tooling provisions it as a
ConfigMap outside the release, and the stage-chart init container unpacks it at
/chart.

With applier.cliDonor.image set, the CLIs arrive the same way (GH-2222): a
cli-donor init container copies helm and kubectl from a stock, digest-pinned
image into the tools emptyDir, mounted read-only at /opt/tools and first on
PATH, so the applier container runs the plain agent-core image and the CLI
versions are a values pin per environment. Kind overlays keep that same
upstream repository and version with pullPolicy Never after the rig imports
the image. Read-only, the binaries the agent execs cannot be replaced at
runtime. Without a donor, applier.image must carry the CLIs itself.

Three applications hand-copied this workload and drifted (GH-2045); the copies
converge here. Call with the root context and the app's own inputs:

  {{- include "agent-services.applier" (dict "root" . "profilePath" $path "profilesVolume" $volume "serviceName" "coding-applier") }}

Required: root, profilePath, profilesVolume, serviceName (the OTel service name).
Optional: workDir (default .Values.applier.workDir), resourceAttributes,
otlpEndpoint, otlpMetricEndpoint, podLabels, podAnnotations, chartWorkloadKinds
(the apps kinds the chart contains, default deployments), chartCoreResources
(the core kinds an upgrade writes), allowedIngressComponents (the release
components that may reach the apply port, default none).
*/}}
{{- define "agent-services.applier" -}}
{{- $root := .root -}}
{{- $values := $root.Values -}}
{{- $applier := $values.applier -}}
{{- $fullname := include "agent-services.fullname" $root -}}
{{- $workDir := .workDir | default $applier.workDir -}}
{{- $chartArchive := default "" $applier.chartArchiveConfigMap -}}
{{- $workloads := .chartWorkloadKinds | default (list "deployments") -}}
{{- $coreResources := .chartCoreResources | default (list "configmaps" "secrets" "services" "serviceaccounts") -}}
{{- $donor := (($applier.cliDonor | default dict).image | default dict) -}}
{{- $donorImage := "" -}}
{{- if $donor.repository -}}
{{- $donorImage = include "agent-services.pinnedImage" $donor -}}
{{- end -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ $fullname }}-applier
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: applier
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ $fullname }}-applier
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: applier
rules:
  # Read deployment state for the rollout verification (kubectl rollout status).
  - apiGroups: [""]
    resources: [configmaps, services]
    verbs: [get, list, watch]
  - apiGroups: [apps]
    resources: [{{ join ", " $workloads }}]
    verbs: [get, list, watch]
  # helm --wait and kubectl rollout status judge readiness by watching the
  # ReplicaSets and Pods a Deployment owns, so a verify reads them even though it
  # never writes them.
  - apiGroups: [apps]
    resources: [replicasets]
    verbs: [get, list, watch]
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, watch]
  {{- if $applier.allowApply }}
  # The apply path runs helm upgrade in-cluster, which manages the chart's objects
  # and the release Secret. Scoped to this namespace by the RoleBinding.
  #
  # helm reads the release Secrets before it does anything -- even --dry-run
  # queries them to find the current release -- so secrets appear in a read rule
  # here, not only in the write rule below.
  - apiGroups: [""]
    resources: [secrets, serviceaccounts]
    verbs: [get, list, watch]
  # An upgrade re-renders the whole chart (srd002-applier R2.2), so the applier
  # must read and write every object kind the chart contains. Writing Roles is
  # bounded: the RoleBinding keeps every grant namespace-scoped, and Kubernetes
  # forbids granting permissions the grantor does not already hold, so the applier
  # can re-apply its own Role but cannot widen it.
  - apiGroups: [networking.k8s.io]
    resources: [networkpolicies]
    verbs: [get, list, watch, create, update, patch]
  - apiGroups: [rbac.authorization.k8s.io]
    resources: [roles, rolebindings]
    verbs: [get, list, watch, create, update, patch]
  - apiGroups: [""]
    resources: [{{ join ", " $coreResources }}]
    verbs: [create, update, patch, delete]
  - apiGroups: [apps]
    resources: [{{ join ", " $workloads }}]
    verbs: [create, update, patch]
  {{- end }}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ $fullname }}-applier
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: applier
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ $fullname }}-applier
subjects:
  - kind: ServiceAccount
    name: {{ $fullname }}-applier
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ $fullname }}-applier
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: applier
spec:
  replicas: 1
  selector:
    matchLabels:
      {{- include "agent-services.selectorLabels" $root | nindent 6 }}
      app.kubernetes.io/component: applier
  template:
    metadata:
      labels:
        {{- include "agent-services.selectorLabels" $root | nindent 8 }}
        app.kubernetes.io/component: applier
        {{- range $key, $value := (.podLabels | default dict) }}
        {{ $key }}: {{ $value | quote }}
        {{- end }}
      {{- with (merge (.podAnnotations | default dict) ($values.podAnnotations | default dict)) }}
      annotations:
        {{- range $key, $value := . }}
        {{ $key }}: {{ $value | quote }}
        {{- end }}
      {{- end }}
    spec:
      # The applier calls the Kubernetes API through helm and kubectl, so unlike
      # the serving roles it mounts its ServiceAccount token.
      automountServiceAccountToken: true
      serviceAccountName: {{ $fullname }}-applier
      terminationGracePeriodSeconds: 30
      {{- with $values.podSecurityContext }}
      securityContext:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- if or $chartArchive $donorImage }}
      initContainers:
      {{- end }}
      {{- if $donorImage }}
        # Copy the pinned helm and kubectl into the tools volume (GH-2222). cp -f
        # replays safely on a pod restart; nothing from the donor stays resident.
        - name: cli-donor
          image: {{ $donorImage | quote }}
          imagePullPolicy: {{ $donor.pullPolicy | default "IfNotPresent" }}
          command: ["sh", "-c", "cp -f /usr/bin/helm /usr/bin/kubectl /tools/"]
          {{- with $values.containerSecurityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          volumeMounts:
            - {name: tools, mountPath: /tools}
      {{- end }}
      {{- if $chartArchive }}
        # Unpack the packaged chart into the shared /chart emptyDir so the
        # applier's `helm upgrade <release> /chart` word finds a chart directory.
        # --strip-components=1 drops the tarball's top-level chart directory so
        # /chart is the chart root.
        - name: stage-chart
          image: {{ include "agent-services.applierImage" $root | quote }}
          imagePullPolicy: {{ include "agent-services.agentPullPolicy" (dict "root" $root "image" $applier.image) }}
          command: ["sh", "-c", "tar -xzf /chart-src/chart.tgz -C /chart --strip-components=1"]
          {{- with $values.containerSecurityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          volumeMounts:
            - {name: chart-src, mountPath: /chart-src, readOnly: true}
            - {name: chart, mountPath: /chart}
      {{- end }}
      containers:
        - name: applier
          image: {{ include "agent-services.applierImage" $root | quote }}
          imagePullPolicy: {{ include "agent-services.agentPullPolicy" (dict "root" $root "image" $applier.image) }}
          {{- with $values.containerSecurityContext }}
          securityContext:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          args:
            - "--profile"
            - {{ .profilePath | quote }}
            - "--directory"
            - {{ $workDir | quote }}
            - "--otel-service-name"
            - {{ .serviceName | quote }}
            {{- with .otlpEndpoint }}
            - "--otel-otlp-endpoint"
            - {{ . | quote }}
            {{- end }}
            {{- with .otlpMetricEndpoint }}
            - "--otel-metric-otlp-endpoint"
            - {{ . | quote }}
            {{- end }}
          env:
            - {name: APPLIER_BIND_HOST, value: "0.0.0.0"}
            # The workspace the agent writes its values file into: the same path
            # mounted as the work volume and passed as --directory, so the write
            # stays inside the workspace wherever the mount moves.
            - {name: APPLIER_WORK_DIR, value: {{ $workDir | quote }}}
            - name: POD_NAME
              valueFrom:
                fieldRef: {fieldPath: metadata.name}
            - name: OTEL_RESOURCE_ATTRIBUTES
              value: {{ .resourceAttributes | default (printf "service.namespace=%s,service.instance.id=$(POD_NAME)" $root.Chart.Name) | quote }}
            # helm and kubectl caches under a writable path, because the root
            # filesystem is read-only.
            - {name: HOME, value: /tmp}
            - {name: HELM_CACHE_HOME, value: /tmp/helm/cache}
            - {name: HELM_CONFIG_HOME, value: /tmp/helm/config}
            - {name: HELM_DATA_HOME, value: /tmp/helm/data}
            {{- if $donorImage }}
            # The donor's CLIs come first, so the exec words' bare helm and
            # kubectl binaries resolve to the pinned copies.
            - {name: PATH, value: "/opt/tools:/usr/local/bin:/usr/bin:/bin"}
            {{- end }}
          ports:
            - {name: apply, containerPort: {{ $applier.ports.apply }}, protocol: TCP}
            - {name: control, containerPort: {{ $applier.ports.control }}, protocol: TCP}
            - {name: monitor, containerPort: {{ $applier.ports.monitor }}, protocol: TCP}
          readinessProbe:
            httpGet: {path: /api/lifecycle/health, port: control}
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            {{- toYaml $applier.resources | nindent 12 }}
          volumeMounts:
            - {name: profiles, mountPath: {{ $values.profiles.mountPath }}, readOnly: true}
            - {name: work, mountPath: {{ $workDir }}}
            - {name: tmp, mountPath: /tmp}
            {{- if $donorImage }}
            - {name: tools, mountPath: /opt/tools, readOnly: true}
            {{- end }}
            {{- if $chartArchive }}
            # The chart the stage-chart init container unpacked, at the /chart
            # path the helm_upgrade exec word references (GH-1368).
            - {name: chart, mountPath: /chart}
            {{- end }}
      volumes:
        {{- if $chartArchive }}
        - name: chart
          emptyDir: {}
        - name: chart-src
          configMap:
            name: {{ $chartArchive }}
        {{- end }}
        {{- toYaml .profilesVolume | nindent 8 }}
        - name: work
          emptyDir: {}
        - name: tmp
          emptyDir: {}
        {{- if $donorImage }}
        - name: tools
          emptyDir: {}
        {{- end }}
---
apiVersion: v1
kind: Service
metadata:
  name: {{ $fullname }}-applier
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: applier
spec:
  type: ClusterIP
  selector:
    {{- include "agent-services.selectorLabels" $root | nindent 4 }}
    app.kubernetes.io/component: applier
  ports:
    - {name: apply, port: {{ $applier.ports.apply }}, targetPort: apply}
    - {name: control, port: {{ $applier.ports.control }}, targetPort: control}
    - {name: monitor, port: {{ $applier.ports.monitor }}, targetPort: monitor}
{{- if $applier.networkPolicy.enabled }}
---
# The apply surface carries no inbound authentication (agent-core REST servers do
# not), so it is gated at the network layer (srd002-applier R4.3). The ingress
# admits only the release components named below; with none named it is a
# default-deny, and an authorized operator or CI caller reaches the port by an
# explicitly provisioned path, never from inside the release.
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ $fullname }}-applier
  labels:
    {{- include "agent-services.labels" $root | nindent 4 }}
    app.kubernetes.io/component: applier
spec:
  podSelector:
    matchLabels:
      {{- include "agent-services.selectorLabels" $root | nindent 6 }}
      app.kubernetes.io/component: applier
  policyTypes: [Ingress]
  {{- if .allowedIngressComponents }}
  ingress:
    - from:
        {{- range .allowedIngressComponents }}
        - podSelector:
            matchLabels:
              {{- include "agent-services.selectorLabels" $root | nindent 14 }}
              app.kubernetes.io/component: {{ . }}
        {{- end }}
      ports:
        - {port: apply, protocol: TCP}
  {{- else }}
  ingress: []
  {{- end }}
{{- end }}
{{- end -}}
