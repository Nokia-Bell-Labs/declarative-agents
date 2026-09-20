<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 2. Terminology

## 2.1 Classes and instances

The language distinguishes declared elements from running ones. A class refers
to a git document, while an instance represents a running mesh entity. Each
concept in this specification includes a suffix naming its side, preventing
normative text from using bare nouns.

| Class (declared, in git) | Instance (running, in the mesh) |
|---|---|
| application-profile | application-instance |
| agent-profile | agent-instance |
| machine-profile | machine-instance |

An application-profile declares the composition: agent-profiles run by the
application-instance, their cardinality, and documents each loads. An
agent-profile specifies an agent-instance's behavior: machine-profile
executed, tools selected, and documents referenced. A machine-profile defines
states, signals, and transitions a machine-instance steps through.

An agent-instance is the triple (agent-profile, identity, workspace). The
identity names the agent-instance, and the workspace denotes its operational
environment. Workspaces persist across instances, retaining their associated
identity: an agent-instance binds to exactly one workspace upon initialization
and releases it upon termination. An agent-instance binding to the same
workspace inherits the same identity. A workspace without a bound
agent-instance is an orphan workspace, a state chapter 08 explicitly permits.

## 2.2 Construction is not instantiation

Two distinct operations yield new entities, with the language maintaining
their separate names. Expansion constructs classes from classes. A template or
fragment, combined with arguments, generates a machine-profile at load time,
confined to documents. Instantiation creates instances from classes. A
profile, paired with an identity and workspace, results in a running instance.
Chapter 03 details expansion; chapter 04 outlines instantiation and subsequent
lifecycle stages.

Two compound terms name the expandable units. A stage-fragment, whose body is
`stage`, combines states and transitions into a machine-profile declaring its
own body. A machine-template, whose body is `machine`, replaces the body of a
machine-profile document carrying an expansion and nothing else.

## 2.3 Properties and attributes

Properties and attributes differ distinctly. A property is declared in a
profile document, versioned in git, and consistent across all class instances.
An attribute, however, is reported, tied to a specific running instance, and
exists only during its lifecycle, observed, not declared. Profile documents
declare only properties; attributes are communicated via the runtime's
reporting surface. This separation ensures conformance remains checkable.
Document and processor targets interact with properties, while runtime and
population targets deal with attributes.

## 2.4 Drafting rules

Normative statements are in `language.yaml`, referencing RFC 2119 levels. They
use class, instance terms (section 2.1), and compound terms (section 2.2).
Bare nouns like "application", "agent", and "machine" are banned in normative
text. The language gate enforces this—violations fail `mage audit` before
evidence runs.

## 2.5 Decision record

Three decisions were gated on this chapter.

**The language is named Libretto.** A libretto is a declared text that a
performance realizes. The document is the class, the performance is the
instance, and the same libretto supports many performances. This is the
language's exact model, and the name is distinctive enough to serve as the
specification's title and search token. The directory keeps its descriptive
name (`application-dsl-spec/`), statement identifiers their `R-` prefix, and
`language.id` remains `application-dsl`; only the title uses the new name.

**Section 2.1's concept names are final.** This includes the class/instance
table, the triple (agent-profile, identity, workspace), and the compound terms
stage-fragment and machine-template, collectively forming the vocabulary used
in all subsequent chapters and every constitution.

**The `instantiate:` keyname is now `expand:`.** Section 2.2 defines this
keyname's operation as expansion—constructing one class from another, whether
splicing a stage-fragment or applying a machine-template. Retaining
"instantiate" for a non-instantiation operation, as defined in the terminology
chapter, would make normative statements internally inconsistent. This rename
is a definitive, non-backward-compatible schema change, aligning with the
repository's approach. In-repo documents update immediately, consumer
repositories transition during their next pin update with a migration note.
The rename is implemented as a distinct unit, contingent on this record, and
`expand:` is uniformly adopted in this specification.
