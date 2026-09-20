<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

# 2. Terminology

## 2.1 Classes and instances

The language separates what is declared from what runs. A class is a document
in git; an instance is a running entity in the mesh. Every concept in this
specification carries the suffix that names which side it is on, and normative
text never uses the bare noun.

| Class (declared, in git) | Instance (running, in the mesh) |
|---|---|
| application-profile | application-instance |
| agent-profile | agent-instance |
| machine-profile | machine-instance |

An application-profile declares the composition: which agent-profiles the
application-instance runs, their cardinality, and the documents each one
loads. An agent-profile declares one agent-instance's behaviour: the
machine-profile it executes, the tools it selects, and the documents it
references. A machine-profile declares the states, signals, and transitions a
machine-instance steps through.

An agent-instance is the triple (agent-profile, identity, workspace). The
identity names who the agent-instance is; the workspace is where it works.
Workspaces persist across instances and carry identity: an agent-instance
binds exactly one workspace when it starts and releases it on exit, and a
later agent-instance that binds the same workspace resumes the same identity.
A workspace with no bound agent-instance is an orphan workspace, which is a
legal state (chapter 08).

## 2.2 Construction is not instantiation

Two different operations produce new things, and the language keeps their
names apart. Expansion is class-to-class construction: a template or fragment
plus arguments produces a machine-profile, entirely at load time, entirely
among documents. Instantiation is class-to-instance: a profile plus an
identity and a workspace produces a running instance. Chapter 03 specifies
expansion; chapter 04 specifies instantiation and the rest of the lifecycle.

Two compound terms name the expandable units. A stage-fragment is a unit
whose body is `stage`: it splices states and transitions into a
machine-profile that declares its own body. A machine-template is a unit
whose body is `machine`: it replaces the body of a machine-profile document
that carries an expansion and nothing else.

## 2.3 Properties and attributes

A property is declared: it lives in a profile document, is versioned in git,
and is the same for every instance of the class. An attribute is reported: it
belongs to one running instance, exists only while the instance does, and is
observed rather than declared. A profile document declares properties only;
attributes reach observers through the runtime's reporting surface. The split
keeps conformance checkable — document and processor targets see properties,
runtime and population targets see attributes.

## 2.4 Drafting rules

Normative statements live in `language.yaml` and cite RFC 2119 levels.
Statement text uses the class and instance terms of section 2.1 and the
compound terms of section 2.2; the bare nouns "application", "agent", and
"machine" are banned in normative text, and the language gate enforces the
ban mechanically — a statement that uses a bare noun fails `mage audit`
before any evidence runs.

## 2.5 Decision record

Three decisions were gated on this chapter.

**The language is named Libretto.** A libretto is a declared text that a
performance realizes: the document is the class, the performance is the
instance, and the same libretto supports many performances. That is this
language's model exactly, and the name is distinctive enough to serve as the
specification's title and search token. The directory keeps its descriptive
name (`application-dsl-spec/`), statement identifiers keep their `R-` prefix,
and `language.id` stays `application-dsl`; only the title carries the coin.

**The concept names of section 2.1 are final.** The class/instance table, the
triple (agent-profile, identity, workspace), and the compound terms
stage-fragment and machine-template are the vocabulary every later chapter
and every constitution uses.

**The `instantiate:` keyname is renamed to `expand:`.** Under section 2.2 the
operation the keyname triggers is expansion — class-to-class construction —
for both of its uses, splicing a stage-fragment and applying a
machine-template. Keeping a keyname that says "instantiate" for an operation
the terminology chapter defines as not-instantiation would make every
normative sentence about it self-contradictory. The rename is a hard schema
revision with no alias, matching how this repository tightens declaration
schemas: in-repo documents migrate in the same change, and consumer
repositories migrate at their next pin bump with a migration note. The rename
pass is a separate unit gated on this record, and this specification writes
`expand:` throughout.
