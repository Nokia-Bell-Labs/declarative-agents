---
id: release-gates
title: Release gates (eng03)
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: push -->
## Push

ask

<!-- field: stages -->
## Stages

<!-- stage: audit -->
### audit

<!-- field: gates -->
#### Gates

| name | command |
| --- | --- |
| audit | mage audit |

<!-- stage: test -->
### test

<!-- field: gates -->
#### Gates

| name | command |
| --- | --- |
| unit tests | go test ./... |
| install integration | mage integration:install |
| cross build | mage crossBuild |
| out of tree | mage outOfTree |
