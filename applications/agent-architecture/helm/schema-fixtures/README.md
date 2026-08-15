<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# Helm values schema fixtures

Each file is merged over `values.yaml`. Files named `valid-*` must pass schema
validation and render; files named `invalid-*` must fail schema or semantic
validation by their offending field.

- valid-collector-spool: collector agent in spool mode (the default)
- valid-no-telemetry: collector disabled, curator alone
- valid-applier-enabled: applier actuator enabled with its network policy on
- invalid-applier-port: applier apply port drifted from the profile contract
- invalid-image: malformed OCI repository
- invalid-port: curator documentation port drifted from the profile contract
- invalid-mount: untrusted profiles mount path
- invalid-replicas: unsupported concurrent curator replicas
- invalid-resources: malformed Kubernetes resource quantity
- invalid-collector-port: collector query port drifted from the profile contract
