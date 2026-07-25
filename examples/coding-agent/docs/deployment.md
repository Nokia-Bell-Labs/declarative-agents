# Coding-agent deployment

This guide covers the packaged Helm chart produced by the coding-agent example.
The runtime image is profile-free; `mage helm:package` resolves and embeds the
planner, executor, and critic role closures as chart files.

## Prerequisites

- Go and Mage for local build and package targets.
- Helm 3 for packaging, installation, upgrade, and rollback.
- A Kubernetes cluster and `kubectl` context.
- An RWX-capable StorageClass or an existing ReadWriteMany claim shared by all
  three role pods.
- An Ollama-compatible endpoint containing `qwen3.6:35b-mlx`, or capacity for
  the optional chart-owned Ollama StatefulSet.
- Enough resources for the configured role and observability limits.

For the live smoke additionally install Docker and kind and pre-pull:

```bash
docker pull golang:1.26-alpine
docker pull otel/opentelemetry-collector-contrib:0.127.0
docker pull jaegertracing/all-in-one:1.62.0
```

The smoke never skips a chart or runtime failure. It skips only when a required
host binary, checkout, Docker engine, or pre-pulled image is absent.

## Package

From `examples/coding-agent`:

```bash
mage package
mage packageValidate
mage helmPrepare
helm lint helm
mage helm:package
```

The installable artifact is:

```text
helm/dist/coding-agent-0.1.0.tgz
```

It contains the strict values schema and every generated role asset. Packaging
validates profile checksums, ConfigMap partitions, archive inventory, lint, and
an independent archive render.

## Install

Create or name an RWX claim first. With a dynamic RWX StorageClass:

```bash
helm install coding-agent helm/dist/coding-agent-0.1.0.tgz \
  --namespace coding-agent --create-namespace \
  --set workspace.storageClass=YOUR_RWX_CLASS
```

With an existing claim:

```bash
helm install coding-agent helm/dist/coding-agent-0.1.0.tgz \
  --namespace coding-agent --create-namespace \
  --set workspace.existingClaim=YOUR_SHARED_CLAIM
```

The chart creates one Deployment and ClusterIP Service each for planner,
executor, and critic. The planner request endpoint is port 18200. Control health
ports are 18201, 18211, and 18221.

## Upgrade and configuration

Export current values before changing them:

```bash
helm get values coding-agent -n coding-agent -a > current-values.yaml
helm upgrade coding-agent helm/dist/coding-agent-0.1.0.tgz \
  -n coding-agent -f current-values.yaml -f desired-values.yaml \
  --wait --timeout 10m
```

Profile checksums are role-scoped. A planner-only profile change rolls only the
planner Deployment. Values cannot choose profiles or profile paths.

Important values:

- `image.repository`, `image.tag`, `image.pullPolicy`: one shared agent-core
  image for all roles.
- `workspace.existingClaim`, `storageClass`, `accessModes`, `size`: shared
  workspace storage.
- `roles.<role>.resources`: pod requests and limits. Replicas are intentionally
  fixed at one until workspace concurrency is modeled.
- `collector.enabled`, `jaeger.enabled`: telemetry topology.
- `ollama.enabled`, `ollama.models`, `ollama.persistence`, `ollama.resources`,
  `ollama.gpu`: optional model tier.

The schema fixes `/profiles`, `/work`, and serving ports because those values
are part of the tested profile contract.

## External Ollama

External mode is the default:

```yaml
ollama:
  enabled: false
llm:
  externalURL: http://ollama.models.svc.cluster.local:11434
```

Planner and executor receive the endpoint through `OLLAMA_URL`. The cluster
must resolve and reach it, and it must report the configured canonical model
from `/api/tags`.

## In-cluster Ollama

Enable the chart-owned StatefulSet and preload Job:

```yaml
ollama:
  enabled: true
  models: ["qwen3.6:35b-mlx"]
  persistence:
    storageClass: YOUR_RWO_CLASS
    size: 25Gi
  gpu:
    count: 1
```

Planner and executor startup waits until every configured model appears. Size
CPU, memory, storage, GPU, node selectors, and tolerations for the selected
model. The kind smoke uses a deterministic model service instead; it does not
pull a 35B model.

## Shared workspace

Planner, executor, and critic mount the same claim at `/work`. Executor edits
the workspace and runs Go validation there; critic reads the same files and
writes `critic-verdict.json`.

For single-node kind, the checked-in `helm/ci/kind-workspace.yaml` binds a
ReadWriteMany PVC to a hostPath on the kind node. The smoke creates and
permission-adjusts that host directory before applying the fixture. This
fixture is test-only and not a production storage design.

## Health and verification

Kubernetes readiness and liveness probes call the profiles' real lifecycle
endpoints:

- planner: `/api/lifecycle/health` on 18201
- executor: `/api/lifecycle/health` on 18211
- critic: `/api/lifecycle/health` on 18221

Inspect rollout status:

```bash
kubectl rollout status deployment/coding-agent-coding-agent-planner -n coding-agent
kubectl rollout status deployment/coding-agent-coding-agent-executor -n coding-agent
kubectl rollout status deployment/coding-agent-coding-agent-critic -n coding-agent
```

Run the disposable packaged-chart proof:

```bash
mage integration:helmSmoke
```

The target builds and kind-loads the local runtime and deterministic model,
installs the `.tgz`, verifies all health endpoints, submits one planner request,
checks the workspace and critic verdict, queries Jaeger for the connected
three-service trace, and tears down everything it owns.

## Telemetry and Jaeger

Each role exports OTLP gRPC to the chart collector with service names
`coding-planner`, `coding-executor`, and `coding-critic`. REST boundaries inject
and extract W3C `traceparent`. The collector batches and forwards traces to
Jaeger.

To inspect Jaeger:

```bash
kubectl port-forward -n coding-agent service/coding-agent-coding-agent-jaeger 16686:16686
open http://127.0.0.1:16686
```

If `collector.enabled=false`, `jaeger.enabled` must also be false. Collector
without Jaeger uses its debug exporter.

## Troubleshooting

Start with bounded status:

```bash
kubectl get deployments,pods,pvc -n coding-agent -o wide
kubectl get events -n coding-agent --sort-by=.metadata.creationTimestamp
helm status coding-agent -n coding-agent
helm history coding-agent -n coding-agent
```

Then inspect role and collector logs:

```bash
kubectl logs -n coding-agent -l app.kubernetes.io/component=planner --tail=100
kubectl logs -n coding-agent -l app.kubernetes.io/component=executor --tail=100
kubectl logs -n coding-agent -l app.kubernetes.io/component=critic --tail=100
kubectl logs -n coding-agent -l app.kubernetes.io/component=collector --tail=100
```

Pending pods usually indicate an RWX claim or resource scheduling problem.
Healthy role pods with request failures usually indicate model reachability,
workspace contents, or application semantics. `integration:helmSmoke` rechecks
Docker and Kubernetes readiness after a failure and labels infrastructure
unavailability separately from semantic failure.

## Security

- Profiles are mounted read-only and are never copied into the image.
- Agent pods run non-root with RuntimeDefault seccomp, no privilege escalation,
  a read-only root filesystem, all capabilities dropped, and no service-account
  token.
- Role transport destinations and profile paths are trusted configuration, not
  request input.
- Services are ClusterIP only. Add authentication, ingress, and NetworkPolicy
  appropriate to the deployment environment before exposing planner requests.
- Treat the shared workspace as mutable application data and apply storage
  encryption, backup, and access policies.

## Rollback and uninstall

Inspect revisions and roll back:

```bash
helm history coding-agent -n coding-agent
helm rollback coding-agent REVISION -n coding-agent --wait --timeout 10m
```

Uninstall:

```bash
helm uninstall coding-agent -n coding-agent --wait
kubectl delete namespace coding-agent
```

Helm does not delete an operator-supplied claim. Confirm retention and remove it
only when its workspace data is no longer needed.

## Known constraints

- One replica per role; concurrent workspace mutation is not modeled.
- All roles require one shared filesystem view.
- The default generated claim requests RWX, which many local default
  StorageClasses do not provide.
- No ingress or authentication layer is included.
- The packaged chart is version 0.1.0 and uses fixed serving ports.
- The live smoke is single-node and does not prove multi-node RWX behavior,
  production model performance, high availability, or disaster recovery.
