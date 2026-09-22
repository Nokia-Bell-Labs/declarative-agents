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
