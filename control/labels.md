---
id: labels
title: Label schema
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: version -->
## Version

1

<!-- field: axes -->
## Axes

<!-- field: plane -->
### Plane

<!-- field: prefix -->
#### Prefix

plane::

<!-- field: exclusive -->
#### Exclusive

true

<!-- field: required -->
#### Required

all

<!-- field: values -->
#### Values

| name | color | description |
| --- | --- | --- |
| management | #5843ad | Changes intent - road-map release, system or subsystem SRD, objective document, sprint |
| control | #8e44ad | Changes a rule - policy files, label schema, state machine |
| data | #1f75cb | Work |

<!-- field: type -->
### Type

<!-- field: prefix -->
#### Prefix

type::

<!-- field: exclusive -->
#### Exclusive

true

<!-- field: required -->
#### Required

none

<!-- field: values -->
#### Values

| name | color | description |
| --- | --- | --- |
| tracking | #6699cc | Task list of member issues, or an open epic's mirror; never popped |
| sprint | #009966 | The sprint's exit checklist |
| recurring | #00b140 | Subscription to one artifact |

<!-- field: kind -->
### Kind

<!-- field: prefix -->
#### Prefix

kind::

<!-- field: exclusive -->
#### Exclusive

true

<!-- field: required -->
#### Required

data

<!-- field: values -->
#### Values

| name | color | description |
| --- | --- | --- |
| build | #428bca | Builds or changes software |
| experiment | #5cb85c | Produces a measurement |
| research | #69d100 | Produces a written result |
| spec | #8e7cc3 | Changes requirements or design documents under docs/ |
| review | #f0ad4e | Responds to another person's artifact |
| consume | #ff8c00 | Adapts to a released artifact |
| handoff | #ad8d43 | Decides what an arriving artifact means |
| release | #34495e | Publishes a version |
| defect | #d9534f | An artifact is wrong |
| request | #d10069 | An artifact lacks something |
| question | #a8d695 | Needs an answer, not a change |

<!-- field: component -->
### Component

<!-- field: prefix -->
#### Prefix

component::

<!-- field: exclusive -->
#### Exclusive

true

<!-- field: required -->
#### Required

all

<!-- field: values_from -->
#### Values from

subsystem SRDs (srd001 R21.1)

<!-- field: color -->
#### Color

#7f8c8d

<!-- field: op -->
### Op

<!-- field: prefix -->
#### Prefix

op::

<!-- field: exclusive -->
#### Exclusive

true

<!-- field: required -->
#### Required

none

<!-- field: values -->
#### Values

| name | color | description |
| --- | --- | --- |
| file | #95a5a6 | Filed by issue file |
| plan | #95a5a6 | Filed by plan push |
| goals | #95a5a6 | Filed by goals push |
| onboard | #95a5a6 | Filed by onboard |
| report | #95a5a6 | Filed by reports aggregate --push |
| status | #95a5a6 | Filed by reports status |
| recur | #95a5a6 | Opened by issue recur |
| upgrade | #95a5a6 | Filed by roll |
| reconcile | #95a5a6 | Filed by reconcile |

<!-- field: state -->
### State

<!-- field: prefix -->
#### Prefix

None

<!-- field: exclusive -->
#### Exclusive

false

<!-- field: required -->
#### Required

none

<!-- field: values -->
#### Values

| name | color | description |
| --- | --- | --- |
| needs-triage | #cc338b | Failed admission; not ready |
| status::in-progress | #428bca | The assignee started it |
| waiting | #e6e6fa | A link is unsatisfied; set by track-goals |
| blocked | #d9534f | The assignee states a blocker |
| needs-run | #ff8c00 | A breaking release reached this subscription |
