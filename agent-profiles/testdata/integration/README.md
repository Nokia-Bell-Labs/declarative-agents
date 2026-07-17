# Integration Fixture Manifest

This directory holds profile-owned integration fixtures. Each fixture is exercised
by exactly one consuming target; some targets live in this repository and some in
an `agent-core` checkout that mounts this repository through `AGENT_PROFILES_ROOT`.
The table below is the authoritative map from fixture to consumer, test suite, and
use case so a reader never has to grep two repositories to learn who reads a
fixture.

## Manifest

| Fixture | Consuming target (repo / file) | Test suite | Use case |
| --- | --- | --- | --- |
| `jurist-charter-demo/` | agent-profiles / `magefiles/validation.go` (`mage validate`, validation-time) | `test-rel06.0-agent-profiles` | `rel06.0-uc002-profile-validation` |
| `rel07-bench-evaluator/` | agent-profiles / `magefiles/integration_bench.go` (`mage integration:benchEvaluator`) | `test-rel07.0-profile-integrations` | `rel07.0-uc004-bench-evaluator-profile-boundary` |
| `rel07-evaluator-generator/` | agent-profiles / `magefiles/integration_evaluator.go` (`mage integration:evaluatorGenerator`) | `test-rel07.0-profile-integrations` | `rel07.0-uc002-evaluator-generator-profile-boundary` |
| `rel07-monitor-control/` | agent-profiles / `magefiles/integration_monitor_control.go` (`mage integration:monitorControl`) | `test-rel07.0-profile-integrations` | `rel07.0-uc005-monitor-control-profile-boundary` |
| `rel07-planner-generator/` | agent-profiles / `magefiles/integration_planner.go` (`mage integration:plannerGenerator`) | `test-rel07.0-profile-integrations` | `rel07.0-uc003-planner-generator-profile-boundary` |
| `uc001-generator-coding/` | agent-core / `magefiles/integration.go:198` (`mage integration:uc001`, run with `AGENT_PROFILES_ROOT` pointing here) | agent-core `test-rel01.0` | agent-core `rel01.0-uc001` |
| `uc002-evaluator-benchmark/` | agent-core / `magefiles/integration.go:199` (`mage integration:uc002`, run with `AGENT_PROFILES_ROOT` pointing here) | agent-core `test-rel01.0` | agent-core `rel01.0-uc002` |
| `rel04-monitor/` | No code consumer. Proof-metadata record cited by docs (see below). | `test-rel06.0-agent-profiles` | agent-core `rel04.0-monitor` |

Note: `mage integration:documentationCurator` (`rel07.0-uc001`) is fixture-free — it
drives the `agents/knowledge-manager/documentation-curator` profile assets directly
rather than a `testdata/integration/` fixture, so it has no row above.

## The `rel04-monitor` record

`rel04-monitor/monitor-rest.yaml` is not read by any Go code in either repository;
it is a proof-metadata record that documents where the monitor profile proof
actually lives (agent-core Go tests such as `TestMonitorReleaseProfileProof` and
`mage integration:uc004`). It is retained rather than deleted because it has real
documentation consumers:

- `agent-profiles/README.md` ("records the monitor profile proof metadata").
- `agent-profiles/docs/specs/test-suites/test-rel06.0-agent-profiles.yaml`
  (`monitor_fixture`).
- `agent-core/docs/ARCHITECTURE.yaml` and
  `agent-core/docs/specs/test-suites/test-rel04.0-monitor.yaml`.

If a future change deletes this record, retarget all four citations in the same
change.

## Naming convention

New fixtures use `rel<NN.N>-<slug>`, where `<NN.N>` is a release number from the
release-number space shared with `agent-core` (see
`agent-profiles/docs/road-map.yaml`) and `<slug>` names the boundary the fixture
exercises. The `uc<NNN>-<slug>` and unprefixed names above are historical and are
kept as-is to avoid churning the `agent-core` references that resolve them.
