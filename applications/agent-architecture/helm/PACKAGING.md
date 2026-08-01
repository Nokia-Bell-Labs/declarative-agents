# Packaging the agent-architecture chart

`mage helm:package` produces a self-contained archive under `helm/dist/`. The chart
carries three profile closures under `profiles/`, staged fresh at package time
because `helm/profiles/` is gitignored:

- `profiles/curator/` and `profiles/collector/` are catalog-owned closures,
  regenerated from the pinned catalog checkout by `mage helmPrepare` and recorded in
  a checksum-bearing `prepared-manifest.yaml`.
- `profiles/applier/agents/applier/` is the application-owned applier profile,
  staged flat from `agents/applier/` alongside the catalog closures. It carries no
  manifest role entry; the `applier.yaml` template mounts it only when
  `applier.enabled` is set.

## Applier exec-declarations placeholder rewrite

The applier's `exec-declarations.yaml` ships placeholder coordinates -- the release
name `agent-architecture`, namespace `default`, and the `agent-architecture-collector`
Deployment -- kept as valid YAML so agent-core validation passes. At render time the
`applier.yaml` template rewrites those placeholders to the installed release: the
release name and namespace from `Release.Name` and `Release.Namespace`, and the
Deployment to `<fullname>-collector`. An applier installed under any release name
therefore targets its own release, namespace, and collector Deployment rather than a
baked default.

The applier verifies and reads the collector Deployment (the application's
persistent, k8s-ready server), not the bounded documentation-curator, so only the
collector Deployment token appears in the rewrite.
