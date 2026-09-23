<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# agent-services

The Helm library chart every declarative-agents application chart depends on. It
holds the named templates the applications would otherwise hand-copy: the
naming, labelling, and image helpers today, and the applier, collector, and
in-cluster Ollama workloads as they converge (GH-2045).

A library chart renders nothing of its own. An application chart declares it
under `dependencies:` and calls its defines.

## Vendoring

Helm resolves a dependency from the chart's own `charts/` directory, and the
application charts are packaged from a staging copy, so `mage helmPrepare`
copies this directory into `<app>/helm/charts/agent-services` rather than
fetching it. The copy is generated and is not tracked.

## Defines

| Define | Returns |
|---|---|
| `agent-services.name` | the chart name, or `nameOverride` |
| `agent-services.fullname` | release name joined to the chart name |
| `agent-services.labels` | the standard recommended labels |
| `agent-services.selectorLabels` | the name and instance labels a selector matches |
| `agent-services.image` | the one application agent image every agent workload runs |
| `agent-services.collectorImage` | the collector image: `collector.image` when set for a non-agent product, else the agent image |
| `agent-services.applierImage` | the applier image: `applier.image` when set, else the agent image |
| `agent-services.agentPullPolicy` | the pull policy for an agent workload: caller override, else the image map's policy, else the root image's |
| `agent-services.otlpEndpoint` | the in-release collector's OTLP gRPC address, empty when no collector is installed |

An application keeps its own `<app>.fullname`-style defines as one-line wrappers
around these, so its templates read unchanged.

## Collector storage contract

The collector persists telemetry through one versioned `collector.storage`
contract (srd042 R10/R11; srd008). It is the single deployment boundary for the
storage backend, so an application sets values rather than inventing WAL,
identity, or env wiring. The backend, bucket, endpoint, and prefix are declared
configuration that flows into the collector's `declarations.yaml` through
`${VAR}` substitution; container code reads no backend environment variable.

| Key | Meaning |
|---|---|
| `backend` | `filesystem` (default, the emptyDir NDJSON spool, unchanged) or `object` |
| `mode` | `durable` (default for `object`) or `ephemeral` (a test collector) |
| `walMode` | `persistent` (a claim) or `ephemeral` (`emptyDir`); durable requires persistent |
| `walPath`, `walCapacity`, `walStorageClass` | WAL mount path, claim size, and class |
| `application`, `namespace` | stamped on every object; `namespace` must be the release namespace |
| `bucketURL` or `bucketName` | the application bucket (`bucketName` becomes `gs://<name>`) |
| `endpoint` | a local emulator endpoint; valid only with the `object` backend |
| `prefix` | a safe relative in-bucket prefix; defaults to the application name |
| `retentionClass` | immutable metadata class stamped on each envelope; defaults to `application` |
| `retentionDays` | optional non-negative days used to stamp `retain_until`; `0` means no time-based expiry is declared |

Every object also records `purge_authority: app:purge`. This is metadata, not
an automatic deletion policy: `app:down` and platform lifecycle never act on
`retain_until`, and only the separately authorized purge workflow may delete
the application's retained set (GH-2490 R5).

The filesystem default renders exactly as before, so an application migrates
when it is ready. No credentials appear in any value: the local rig uses fake
GCS through an explicit endpoint, and cloud uses ambient workload identity.

Local fake GCS (kind):

```yaml
collector:
  storage:
    backend: object
    bucketName: chatbot-mesh
    endpoint: http://fake-gcs.fake-gcs.svc:4443/storage/v1/
    walCapacity: 1Gi
```

Cloud GCS (GKE, ambient workload identity, no endpoint):

```yaml
collector:
  storage:
    backend: object
    bucketURL: gs://acme-chatbot-mesh-telemetry
    walCapacity: 5Gi
```

Cloud S3 (EKS, ambient IRSA identity):

```yaml
collector:
  storage:
    backend: object
    bucketURL: s3://acme-chatbot-mesh-telemetry
```

An explicitly ephemeral test collector keeps compute-local storage:

```yaml
collector:
  storage: {backend: object, bucketName: test, mode: ephemeral}
```

## One agent image

The collector and the applier are agent workloads: they run the agent binary
over a mounted profile. By default they render the one application agent image
(`.Values.image`), the composition model's one-image invariant that
srd005-chart-conformance R9 gates against every rendered manifest. `collector.image`
and `applier.image` are optional overrides — the collector map for a genuinely
non-agent product such as a contrib OpenTelemetry gateway, the applier map only
where an environment must carry helm and kubectl in the applier image instead of
the pinned CLI donor. An absent or repository-less map inherits the agent image,
so an application that drops the value keeps rendering.
