<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 5. Application-profile grammar

## 5.1 The document

An application-profile is the `application.yaml` in an application module's
agents root. It specifies the composition (which agent-profiles the
application-instance runs), ownership, capability evidence, and packaging. It
is the document the population's closure invariant closes over (chapter 08): a
root declared here is the only way an agent-instance enters the population.

## 5.2 Keynames

{{keyname-table go:magefiles/appmanifest/manifest.go#Manifest}}

The `roots` sequence carries the composition. Each entry specifies one
agent-profile the application-instance seeds.

{{keyname-table go:magefiles/appmanifest/manifest.go#Root}}

A capability entry under `capabilities` declares a status and evidence
commands; the repository's magefiles hold the authoritative validation
semantics beyond keyname presence.

{{statement R-APP-001}}

## 5.3 Ownership

A root is `local` if its agent-profile is in the application module, or
`canonical` if it refers to the shared catalog. An application-profile focused
solely on composition declares no local roots and introduces no duplicate
agent-profiles, enabling the repository's reuse accounting to distinguish
implementations from compositions.
