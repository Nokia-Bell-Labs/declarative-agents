<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 5. Application-profile grammar

## 5.1 The document

An application-profile is the `application.yaml` at an application module's
agents root. It declares the composition — which agent-profiles the
application-instance runs — together with the module's ownership, capability
evidence, and packaging. It is the document the population's closure
invariant closes over (chapter 08): a root declared here is the only way an
agent-instance enters the population.

## 5.2 Keynames

{{keyname-table go:magefiles/appmanifest/manifest.go#Manifest}}

The `roots` sequence carries the composition. Each entry names one
agent-profile the application-instance seeds:

{{keyname-table go:magefiles/appmanifest/manifest.go#Root}}

A capability entry under `capabilities` declares a status and the evidence
commands that prove it; the manifest gates in the repository's magefiles hold
the authoritative validation semantics beyond keyname presence.

{{statement R-APP-001}}

## 5.3 Ownership

A root is `local` when its agent-profile lives in the application module and
`canonical` when it references the shared catalog; a composition-only
application-profile declares no local roots and contributes no duplicate
agent-profiles, which is how the repository's reuse accounting tells
implementations apart from compositions.
