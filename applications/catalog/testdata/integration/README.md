# Integration Fixture Manifest

This directory holds profile-owned integration fixtures and documentation-only
proof records. Each executable fixture is exercised by exactly one consuming
target; some targets live in this repository and some in an `agent-core` checkout
that mounts this repository through the demo.yaml catalog_root. The table below is the
authoritative map from each entry to its consumer, test suite, and use case. An
entry marked "No code consumer" is metadata and is not an executable test input.

## Manifest

| Fixture | Consuming target (repo / file) | Test suite | Use case |
| --- | --- | --- | --- |
| `specification-critic-charter-demo/` | applications/catalog / `magefiles/validation.go` (`mage validate`, validation-time) | `test-rel06.0-agent-profiles` | `rel06.0-uc002-profile-validation` |
| `specification-critic-charter-demo-failing/` | applications/catalog / `conformance/specification_critic_test.go` (`mage conformance`, `TestSpecificationCriticConformance/FailingCorpusFails`) | `test-rel06.0-agent-profiles` | `rel06.0-uc002-profile-validation` |
| `rel07-evaluator-generator/` | Retired fixture retained for conformance compatibility; the former Mage tracer used a fake child. Replacement: `applications/coding-agent` / `mage integration:criticGate`. | `test-rel07.0-profile-integrations` | `rel07.0-uc002-evaluator-generator-profile-boundary` |
| `rel07-monitor-control/` | applications/catalog / `magefiles/integration_monitor_control.go` (`mage integration:monitorControl`) | `test-rel07.0-profile-integrations` | `rel07.0-uc005-monitor-control-profile-boundary` |
| `rel07-planner-generator/` | Retired fixture retained as historical data; the former Mage tracer synthesized planner behavior and used a fake child. Replacement: `applications/coding-agent` / `mage integration:plannerDelegation`. | `test-rel07.0-profile-integrations` | `rel07.0-uc003-planner-generator-profile-boundary` |
| `uc001-generator-coding/` | Retired core fixture. Replacement: `applications/coding-agent` / `mage integration:executorLive`. | agent-core `test-rel01.0` | agent-core `rel01.0-uc001` |
| `uc002-evaluator-benchmark/` | Retired core fixture retained as benchmark sample data. Application gate replacement: `applications/coding-agent` / `mage integration:criticGate`. | agent-core `test-rel01.0` | agent-core `rel01.0-uc002` |
| `rel04-monitor/` | No code consumer. Proof-metadata record cited by docs (see below). | `test-rel06.0-agent-profiles` | agent-core `rel04.0-monitor` |

Note: `mage integration:documentationCurator` (`rel07.0-uc001`) and
`mage integration:benchEvaluator` (`rel07.0-uc004`) are fixture-free. They drive
the shipped profile assets through behavioral conformance rather than consuming
inputs under `testdata/integration/`, so they have no rows above.

## The `rel04-monitor` record

`rel04-monitor/monitor-rest.yaml` is not read by any Go code in either repository;
it is a proof-metadata record that documents where the runtime-state-reader proof
actually lives (agent-core Go tests such as `TestMonitorReleaseProfileProof` and
`mage integration:monitor`). It is retained rather than deleted because it has real
documentation consumers:

- `applications/catalog/README.md` ("records runtime-state-reader proof metadata").
- `applications/catalog/docs/specs/test-suites/test-rel06.0-agent-profiles.yaml`
  (`monitor_fixture`).
- `agent-core/docs/ARCHITECTURE.yaml` and
  `agent-core/docs/specs/test-suites/test-rel04.0-monitor.yaml`.

If a future change deletes this record, retarget all four citations in the same
change.

## Naming convention

New fixtures use `rel<NN.N>-<slug>`, where `<NN.N>` is a release number from the
release-number space shared with `agent-core` (see
`applications/catalog/docs/road-map.yaml`) and `<slug>` names the boundary the fixture
exercises. The `uc<NNN>-<slug>` and unprefixed names above are historical and are
kept as-is to avoid churning the `agent-core` references that resolve them.
