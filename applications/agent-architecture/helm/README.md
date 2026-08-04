# agent-architecture Helm chart

This chart deploys the Agent Architecture demo: the documentation-curator and the
collector, and optionally the deployment-plane applier (srd001, srd002-applier).
The agent programs live beside the chart under `agents/` and in the pinned catalog,
so the chart carries them as staged profile closures rather than baking them into
the image. A bare `helm install` of the source directory would ship a chart with no
staged profiles, and the deployed workloads would mount an empty `/profiles`.

## Install the packaged chart

Package first, then install the self-contained archive:

```sh
mage helm:package
helm install my-release helm/dist/agent-architecture-*.tgz \
  --namespace agent-architecture --create-namespace
```

`mage helm:package` regenerates the curator and collector profile closures from
the catalog, stages the application-owned composition wrapper over the
canonical applier, validates the staged tree, and packages
`helm/dist/agent-architecture-0.1.0.tgz`. The archive lints and renders from a
directory with no source checkout present.

## Enable the applier

The applier is disabled by default: it runs the shared applier image (agent-core
plus helm and kubectl, built from `agent-core/applier.Dockerfile`; GH-1368) rather
than the profile-free runtime image the curator and collector use, so every other
cluster test installs the mesh without it. The image bakes no chart; the chart it
runs `helm upgrade agent-architecture /chart` against is delivered to the pod at
`/chart` as a mounted volume (`applier.chartArchive`, unpacked by an init
container), so the bytes travel with the Helm release. Enable it with the live-tier
overlay:

```sh
helm install my-release helm/dist/agent-architecture-*.tgz \
  --namespace agent-architecture \
  --values helm/ci/kind-applier-values.yaml
```

The applier exposes an apply surface (`POST /api/v1/apply`) that an authorized
operator or CI caller reaches to submit a decided values patch. It validates the
patch against `values.schema.json` with a helm dry-run, applies it as a rollout,
verifies the collector Deployment, and rolls the release back on a stall.

## Layout and packaging

See [PACKAGING.md](PACKAGING.md) for how the profile closures are staged into the
chart and how the applier's exec-declarations placeholders are rewritten to the
installed release at render time.
