# Helm values schema fixtures

Each file is merged over `values.yaml`. Files named `valid-*` must lint and
render; files named `invalid-*` must fail schema or semantic validation.

- valid-external-llm: external model endpoint
- valid-incluster-ollama: chart-owned model tier
- valid-existing-workspace: operator-owned shared PVC
- valid-collector-spool: collector agent in spool mode (the default)
- valid-no-telemetry: collector disabled
- invalid-image: malformed OCI repository
- invalid-port: serving port differs from the profile contract
- invalid-resources: malformed Kubernetes resource quantity
- invalid-storage: malformed storage quantity
- invalid-replicas: unsupported concurrent role replicas
- invalid-url: unsupported external LLM URL
- invalid-mount: untrusted workspace mount path
- invalid-models: empty in-cluster model set
- valid-applier-enabled: srd006 applier enabled with its network policy on
- invalid-applier-port: applier apply port drifted from the profile contract
