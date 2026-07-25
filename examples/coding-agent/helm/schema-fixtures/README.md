# Helm values schema fixtures

Each file is merged over `values.yaml`. Files named `valid-*` must lint and
render; files named `invalid-*` must fail schema or semantic validation.

- valid-external-llm: external model endpoint
- valid-incluster-ollama: chart-owned model tier
- valid-existing-workspace: operator-owned shared PVC
- valid-collector-debug: collector without Jaeger
- valid-no-telemetry: both observability components disabled
- invalid-image: malformed OCI repository
- invalid-port: serving port differs from the profile contract
- invalid-resources: malformed Kubernetes resource quantity
- invalid-storage: malformed storage quantity
- invalid-replicas: unsupported concurrent role replicas
- invalid-url: unsupported external LLM URL
- invalid-mount: untrusted workspace mount path
- invalid-telemetry: Jaeger without its collector ingress
- invalid-models: empty in-cluster model set
