---
id: labels
title: Label schema
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: purpose -->
## Purpose

Defines the labels a program creates in its tracker group so every issue can carry the five axes of the process: plane, type, kind, component, and state. The scripts create missing labels from this file and read it to check admission. Values for plane, type, kind, op, and state are fixed by srd001; component values come from the program. When a newer scrum-master adds an axis or a value, scrum-master doctor adds it to the file and keeps everything the file already holds (srd043 R4.2).

<!-- field: schema -->
## Schema

<!-- schema_entry: version -->
### version

<!-- field: type -->
#### Type

integer

<!-- field: description -->
#### Description

Schema version of this file; 1.

<!-- schema_entry: axes -->
### axes

<!-- field: type -->
#### Type

mapping of axis name to axis

<!-- field: description -->
#### Description

One entry per axis: plane, type, kind, component, op, state. Each axis has prefix, exclusive, required, and values.

<!-- schema_entry: axes.<axis>.prefix -->
### axes.<axis>.prefix

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

The label prefix, written with the scoped-label separator (plane::, type::, kind::, component::). The state axis has no prefix; its labels are listed whole.

<!-- schema_entry: axes.<axis>.exclusive -->
### axes.<axis>.exclusive

<!-- field: type -->
#### Type

boolean

<!-- field: description -->
#### Description

True when an issue may carry at most one value of the axis. True for plane, type, kind, component, and op. False for state, because the overlay states of srd041 R1–R32 combine.

<!-- schema_entry: axes.<axis>.required -->
### axes.<axis>.required

<!-- field: type -->
#### Type

string

<!-- field: description -->
#### Description

Which issues must carry a value: all (plane, component except on a sprint issue), data (kind, on plane::data issues), or none (type, op, state).

<!-- schema_entry: axes.<axis>.values -->
### axes.<axis>.values

<!-- field: type -->
#### Type

list of value

<!-- field: description -->
#### Description

Each value has name, color as a hex string, and description, the tracker's label description. Component values are not listed here: lint and track-goals derive them from the components the subsystem SRDs declare (srd001 R21.1), and the scripts create a component label the first time a subsystem SRD names it.

<!-- field: config_file_layout -->
## Config file layout

```markdown
---
id: labels
title: Label schema
---

<!-- field: config_files -->
## Config files

control/labels.md in the program repository

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

| name | color | description | SRD | sprint | exit decision | label schema | state machine |
| --- | --- | --- | --- | --- | --- | --- | --- |
| management | #5843ad | Changes intent - road-map release | None | None | None |  |  |
| control | #8e44ad | Changes a rule - policy files |  |  |  | None | None |
| data | #1f75cb | Work |  |  |  |  |  |

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
| tracking | #6699cc | Task list of member issues; never popped |
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

| name | color | description | not a change |
| --- | --- | --- | --- |
| build | #428bca | Builds or changes software |  |
| experiment | #5cb85c | Produces a measurement |  |
| research | #69d100 | Produces a written result |  |
| spec | #8e7cc3 | Changes requirements or design documents under docs/ |  |
| review | #f0ad4e | Responds to another person's artifact |  |
| consume | #ff8c00 | Adapts to a released artifact |  |
| handoff | #ad8d43 | Decides what an arriving artifact means |  |
| release | #34495e | Publishes a version |  |
| defect | #d9534f | An artifact is wrong |  |
| request | #d10069 | An artifact lacks something |  |
| question | #a8d695 | Needs an answer | None |

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
| file | #95a5a6 | Filed by scrum-master issue file |
| plan | #95a5a6 | Filed by scrum-master plan push |
| goals | #95a5a6 | Filed by scrum-master goals push |
| onboard | #95a5a6 | Filed by scrum-master onboard |
| report | #95a5a6 | Filed by scrum-master reports aggregate --push |
| status | #95a5a6 | Filed by scrum-master reports status |
| recur | #95a5a6 | Opened by scrum-master issue recur |

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
```
