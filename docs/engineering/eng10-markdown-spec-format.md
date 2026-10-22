---
id: eng10-markdown-spec-format
title: Markdown Specification Format
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: introduction -->
## Introduction

This guideline specifies the Markdown format that replaces the typed YAML specification corpus (epic GH-2578). A document in this format renders as a document on GitHub and GitLab, and it parses into the same document model the YAML front end feeds, so the traceability graph and every audit check keep working unchanged. Each heading carries a one-line HTML comment marker that says what the heading opens. Facts a reader needs stay in visible Markdown, and machine-only facts live in the markers.

We wrote the format from evidence, not from argument. A prototype (GH-2588, GH-2589) first parsed, printed, and diagnosed the format, and agent-core/pkg/spec/markdown/FINDINGS.md records what it established: round-trip laws over the 106 tracked SRDs, the inventory of shape rules, and the quality of the diagnostics. The rules this guideline describes are data. Each document type declares them in the markdown block of its entry in agent-core/docs/constitutions/design.yaml, and the engine reads its rules from nowhere else. The engine (marker grammar, shapes, declaration loader, diagnostics, and the converter for every type) lives in agent-core/pkg/spec/markdown.

This guideline is itself written in the format. Running mage specCheck on this file reports zero findings, which makes the file its first conformance test.

<!-- section: one-layer-rule -->
## The one-layer rule

A fact lives in exactly one layer. Reader-relevant facts live only in visible Markdown: headings, paragraphs, lists, tables, and the frontmatter. Machine-only facts live only in marker comments: a requirement's weight, the lifecycle status and retirement pointers of a node, and provenance references. No fact appears in both layers, so the two cannot drift apart.

The frontmatter counts as visible. GitHub renders it as a table above the document, and GitLab shows it as a code box, so identity and lifecycle fields such as id, title, status, priority, and release belong there. Everything else a reader needs is a tagged field under a heading.

We applied the rule to criterion traces (GH-2581 decision 3). A reader must see which requirements a criterion verifies, so traces are a visible field under each criterion, not a marker attribute. The same holds for use case success criteria and for test cases.

<!-- section: file-layout -->
## File layout

A document is one Markdown file at the location its type declares in location. It opens with the frontmatter, then the two license comment lines, then its nodes in order. The skeleton of an SRD follows.

```markdown
---
id: srd004-declarative-ux
title: Declarative UX
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Prose under the heading.

<!-- req: R1 -->
## R1 - Contract

<!-- req: R1.1 | weight: 3 -->
### R1.1

The requirement text.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

The criterion text.

<!-- field: traces -->
#### Traces

- R1.1
```

The frontmatter lists the fields frontmatter.required names and admits only those frontmatter.allowed names. Where a type sets id_matches_filename, the id equals the file name without its extension. The license comments are plain comments, not markers, and the copyright gate checks them as it does for any Markdown file.

The migration waves (GH-2584) moved every corpus document to Markdown except design.yaml, which holds the declarations themselves, and GH-2625 pointed every link and guidance file at the Markdown paths. The code sunset (GH-2626) removed the YAML front end: each type's location in design.yaml names its Markdown path, the audit discovers only Markdown documents, and a .yaml file at a corpus document's former location matches no declared location and fails placement. A type's converts_from keeps that former YAML location for one reader only, mage convert, so a repository still on a YAML corpus can migrate. The other reader of YAML documents is history: the retired-id checks read documents at a release tag, and the external-reference checks read a parent repository at its pinned ref, and both refs may predate the migration.

<!-- section: marker-grammar -->
## Marker grammar

A marker is a single-line HTML comment directly above the heading it tags. It holds a tag and a value, then any attributes, each segment separated by a space, a vertical bar, and a space. Attribute values are JSON: strings are quoted, lists are arrays, and provenance references are arrays of objects. Table 1 gives the grammar.

Table 1: Marker grammar

| Part | Form | Example |
| --- | --- | --- |
| Marker | `<!-- <tag>: <value>[ \| <attr>: <json>]... -->` | `<!-- req: R1.1 \| weight: 3 -->` |
| Tag | one of the tags the document type declares | field, req, ac |
| Value | the node's id, or the field name for a field marker | R1.1, acceptance_criteria |
| Attribute | a name the node declares in its attributes list | status, retired_in, replaced_by |
| Attribute value | JSON in the shape declared under attributes | `["srd010 R2.1"]` |

Binding is positional. A marker binds to the next heading, and only to it; a marker followed by another marker or by the end of the document is an orphan. The tag set is closed per type: a tag the type does not declare is a marker-unknown-tag finding with the nearest declared tag as its hint. Across all declared types the tags are field, req, ac, sc, case, section, release, pattern, interface, component, decision, and article.

The issue that commissioned this guideline named the tags req, field, acceptance, and copyright. The prototype settled on ac for acceptance criteria, matching the AC1 ids, and it treats the copyright and SPDX lines as license comments rather than markers, so no tag exists for them.

<!-- section: heading-grammar -->
## Heading grammar

A document type declares its nodes. Each node declaration names a tag, a heading level, a parent, an optional id pattern, a heading form, a content kind, whether ids are unique, the attributes it admits, and the fields it requires. Table 2 lists the parts.

Table 2: Node declaration

| Part | Meaning |
| --- | --- |
| tag | the marker tag that opens the node |
| level | the heading depth, 2 to 6 |
| parent | document, another node kind, or field:<name> for a node under a field |
| id_pattern | a regular expression over the marker value |
| id_extends_parent | the id starts with the parent's id and a dot, as R1.1 under R1 |
| heading | id (the heading is the id), id_title (the id, or the id, a spaced hyphen, and a title), or free |
| content | none (only tagged children), prose (text, no untagged list or table), or any |
| unique_id | no two nodes of the kind share an id |
| attributes | the marker attributes the node admits |
| required_fields | the fields every node of the kind carries |
| fields | for a field slot (tag field): every field the slot holds, in declared order |
| flat | table: a list every record of which is flat prints as a table; only a list some record of which holds structure or a paragraph prints as records |

A node's parent is the node open one level up, or field:<name> when that node is a field. A heading at the wrong depth, or under a parent its declaration does not name, is heading-misplaced. The document as a whole declares the fields it requires (required_fields) and the minimum count of each node kind (required_nodes); a status listed in exempt_status, such as relocated or retired, lifts the content requirements from a stub.

A node kind whose tag is field is a field slot: the place where fields open, one heading level and one parent. A slot whose parent is field:<name> holds nested fields, the keys of that field's value. The next section but one declares them (GH-2651).

<!-- section: field-shapes -->
## Field shapes

A field is a node opened by a field marker; its name is the marker value. A typed field declares where it may appear and the shape of its content. A field with no shape is passthrough: its content takes the form its value needs (Table 7). Passthrough does not mean undeclared: a slot that lists its fields admits only those, in that order (Declared fields and order). Table 3 lists the shapes.

Table 3: Field shapes

| Shape | Content | Constraints |
| --- | --- | --- |
| prose | Markdown text, read back verbatim: paragraphs, block quotes, and fenced blocks such as Mermaid | no heading, marker, or setext underline; not one list, table, or yaml fence on its own |
| code | one fenced block in the language the field declares (code: bash) | the field declares code |
| list | one unordered list with hyphen markers | item_pattern on each item, when declared |
| table | one GFM table | columns, when declared, exactly and in order |
| scalar | one line holding a YAML flow scalar (form: scalar) | the field marker carries the form |
| typed table | one GFM table whose cells are YAML flow values (form: typed table) | the field marker carries the form |
| keyed table | a typed table whose first column, id, holds each record's key (form: keyed table) | the field marker carries the form |
| tree | nested lists: bullets for map entries, numbers for list elements (form: tree) | the field marker carries the form |
| none | the line None, for an empty list or map (form: empty list, form: empty map) | the field marker carries the form |
| empty | nothing; the field only holds child nodes | none |
| yaml | one fenced yaml block, the reported last resort | none |

A list or table field with no entries still has its shape, so an empty index is not a finding. A table field that declares no columns accepts any header, which the index documents need because their columns differ by module.

How each YAML value becomes one of these shapes is the job of the YAML mapping, two sections on. The scalar, typed table, and tree shapes are the typed forms, specified in the section after it. The fenced yaml block is the last resort: every one the printer still writes is reported (GH-2638).

<!-- section: declared-fields -->
## Declared fields and order

The constitution declares every field a document holds, so the audit can check that each one is there and in its place (GH-2651). A field slot lists its fields in order (fields), and that list is both the slot's membership and its order. The document slot lists a type's top-level fields; a record's slot lists the fields its records carry; a slot under field:<name> lists the keys of that field's value, which print as nested fields.

A nested field is a field like any other. It prints as a heading one level below its parent field, with a field marker naming its key, and its value converts by Table 7 under that heading: prose, a list, a table, a typed form, or nested fields again when a slot is declared under it. Records may sit under a nested field as they sit under a top-level one: a node kind whose parent is field:<name> holds the records of that field's value, a map keyed by id or a list, each record a heading titled by its id. A list of records whose content a table cannot hold, records with nested values or long text, is declared the same way: a record kind under the field, each record a tagged heading titled by its id, and a slot listing the fields its records carry, so the Markdown parses through its tags down to the data (GH-2663). A list that is flat in one document and rich in another declares the kind with flat: table: every document prints a table while its records are flat (the plain table for strings, the typed table otherwise, with the field's link columns) and records once one holds a paragraph, so one declaration gives one rule for every document and both forms read back as the same list (GH-2674). A record kind belongs to its field at one level, so the same key can hold records in one place and text or a map in another, and a value that is not a list of the kind's records keeps its usual conversion. Headings stop at level 6. A field at level 6 holds its value in one form, a tree when it is structured, and that value's keys are data of the field rather than fields.

Two checks hold a document to its slots. A field a slot does not list is field-undeclared; the fix is to declare it in the slot, in its place, or to move the content into a declared field. A field that appears after a field its slot lists later is field-out-of-order; the fix is to move it. Both run only on slots that list their fields, so a type turned strict when its declaration listed them; every slot in design.yaml now lists its fields, and a test in agent-core/pkg/spec/markdown holds it to that (GH-2660), so a field a document adds without declaring it is a finding. A field rule's in names the slots the field may appear in, and every slot it names that lists fields lists it.

Table 4: A nested field, nested records, and an empty value

| Declaration (design.yaml) | Markdown | YAML |
| --- | --- | --- |
| document_field lists overview, then interfaces; overview_field: {tag: field, level: 3, parent: 'field:overview', fields: [summary, layout]} | `<!-- field: overview -->`, ## Overview, `<!-- field: summary -->`, ### Summary, then the summary text | overview: {summary: <text>, layout: ...} |
| layout_field: {tag: field, level: 4, parent: 'field:layout', fields: [status, packages]}; package: {tag: package, level: 5, parent: 'field:packages', heading: id, content: none, yaml: {from: packages, records: list, id: path}} | `<!-- field: layout -->`, ### Layout, `<!-- field: status -->`, #### Status, current, `<!-- field: packages -->`, #### Packages, `<!-- package: internal/tools/catalog/ -->`, ##### internal/tools/catalog/ | layout: {status: current, packages: [{path: internal/tools/catalog/, ...}]} |
| package_field: {tag: field, level: 6, parent: package, fields: [responsibility, dependencies]} | `<!-- field: dependencies \| form: "tree" -->`, ###### Dependencies, - may_depend_on:, then 1. internal/runtime/core indented | dependencies: {may_depend_on: [internal/runtime/core]} |
| a field holding a list of flat records | `<!-- field: workspace_state \| form: "typed table" -->`, then \| command \| strategy \| receipt \| and one row per record | workspace_state: [{command: write, strategy: ..., receipt: ...}] |
| a field whose value is an empty list | `<!-- field: declaration_mismatches \| form: "empty list" -->`, then the line None | declaration_mismatches: [] |

<!-- section: reference-links -->
## Reference links

A field can hold references to requirements and documents: an acceptance criterion's traces, a test case's traces and use case, a use case's touchpoints, the index tables. A field rule that declares links (links: true, or link_columns naming a table's columns) renders each reference in its list items, its one-line value, or those columns as Markdown links, so a reader follows a trace instead of searching for it (GH-2665). Ids written inside prose stay text.

A reference is a whole value: a document id, optionally followed by a space and items separated by a comma and a space (srd057-capability-profiles R1.1, R1.2), or a bare item of the same document (R1.1). An item is capital letters and digits with dotted parts (R1.1, AC5, S3). Each part links on its own: the document id to the document's file by a path relative to the referring document, and each item to that file's heading anchor; a bare item links to its anchor on the same page. A value with any other text, or a part whose target the converter does not know, stays text.

An anchor is GitHub's: the heading text lowercased, every character but letters, digits, underscores, hyphens, and spaces dropped, spaces turned into hyphens, and a -1, -2 suffix for each repeat of the same anchor earlier on the page. R1.1 becomes #r11 and "R1 - Budget" becomes #r1---budget. An item's anchor is the one its heading takes in its document.

Links are rendering only. The YAML tree holds the plain reference, and the parser reads a link back as its text when the text is a reference and the target is one the converter writes: a relative Markdown file for a document id (file names need not match ids, as cfg-runtime-contract lives in runtime-contract.md), and an anchor starting with the item's own for an item. The converter owns the target, so mage fmt rewrites a stale one. A link the converter would not write, or one around other text, stays literal text. The parser needs nothing beyond the document, so the corpus loader and a lone document parse as before; the printer needs the corpus index, the ids, paths, and item anchors of every corpus document, to write cross-document links, and mage fmt, mage convert, and the canonical-form check all print with it, so a moved document's links follow it on the next mage fmt and a stale one is a not-canonical finding. Without the index the printer links same-page items only.

A path written as a value is not a link and has one base: the corpus root, the directory that holds docs/. That covers the path column of every SPECIFICATIONS.md index table and every entry of ARCHITECTURE.md's related_documents, so docs/specs/software-requirements/srd001-auth.md names the same file whichever document writes it, and a path to a document outside the corpus climbs from the root (../docs/specs/semantic-models/agent-role-realizations.md from an application). The corpus audit reports index-broken-path for any such path that names no file (GH-2698). related_documents may be written as a plain list of paths or as a table of doc and purpose rows; an entry that is neither is a related-document-malformed finding, and the audit still runs every other check (GH-2697).

Table 5: Reference links, one example each

| Reference | Markdown | YAML |
| --- | --- | --- |
| a same-document item | `- [R1.1](#r11)` | traces: [R1.1] |
| a document | `[rel00.0-uc001-domain-agent-integration](../use-cases/rel00.0-uc001-domain-agent-integration.md)` | use_case: rel00.0-uc001-domain-agent-integration |
| a document and items | `[srd057-capability-profiles](../software-requirements/srd057-capability-profiles.md) [R1.1](../software-requirements/srd057-capability-profiles.md#r11), [R1.2](...#r12)` | target: srd057-capability-profiles R1.1, R1.2 |

<!-- section: yaml-mapping -->
## YAML mapping

One converter serves every document type (GH-2582, split on 2026-10-16). It maps a Markdown document to the yaml.Node tree its YAML form parses into, and back, driven only by the type's declaration. Every consumer that reads the YAML tree today reads the same tree from a Markdown document: the typed parsers (ParseSRD, ParseUseCase, ParseTestSuite, ParseRoadmap, ParseSpecIndex), the corpus charter's YAML-path checks, and the reference checks in magefiles. The typed models stay the one document model; Markdown is a second way to reach them.

The rendering rules above say how a document looks. The YAML mapping says where each piece lives in the tree, and it sits beside them in the declaration. Frontmatter fields are top-level scalar keys of the same name. A field maps to the record key of its name, and its value converts by Table 7, a field with nested fields to a mapping holding them in document order; a field declaration may name the keyed or outline conversion (value), an outline's id and content keys, and the heading the field prints with. A node kind maps to records by the parts in Table 6, given in the yaml block of its declaration.

Table 6: YAML mapping of a node kind

| Part | Meaning |
| --- | --- |
| from | the key, in the parent's record or at the top level, that holds the records |
| records | list (a sequence of records) or map (records keyed by id, as SRD requirement groups are) |
| id | the record key that holds the id; for map records the id is the map key |
| title | the record key whose value follows the id in an id_title heading |
| content | the record key whose value is the node's prose |
| keyed | the record may be written as a one-entry map from id to content |

Every other record key prints, in record order, under the field kind declared with the node kind as its parent, as SRD item fields sit under requirement items. A node kind whose records sit under a field names that field as its parent (field:acceptance_criteria); one whose records sit straight under a key, with no heading of their own, names the key in from and its parent node or the document as its parent, as requirement groups do.

Table 7: Value conversions

| YAML value | Markdown | Used for |
| --- | --- | --- |
| string | prose | problem, summary, purpose |
| list of strings | list | non_goals, references |
| map of strings | keyed list: each item reads name, colon, text | vision success criteria |
| list of keyed records | outline: each item reads id, colon, content, with every other record key as a nested item | goals, flow steps |
| list of flat records | table, one column per record key | touchpoints, needs_clarification, index tables |
| list of records with prose | nodes, one per record | requirement items, acceptance criteria, test cases |
| string in a code field | code | test-suite commands |
| number, boolean, null, or a string prose would misread | scalar | pattern confidence, test_case_count |
| list of records with scalar or scalar-list values | typed table | index tables with counts, road-map use_cases |
| map under a field whose slot is declared | nested fields, one heading per key | architecture overview, receipt groups |
| map of records keyed by id sharing their attributes | keyed table | semantic-model relationships, metric definitions |
| any other map or list | tree | test-case inputs and expected, implementation_status, diagrams |
| a value no other form holds exactly | fenced yaml block, reported | none in the corpus once regenerated, or each one listed |

A passthrough value, one no declaration types, takes the first conversion that parses back to the same node, in this order: a string becomes prose, a list of plain strings becomes a list, a list of flat string records sharing one key order becomes a table, and then the typed forms: a lone scalar becomes scalar, a list of records whose values are scalars or lists of scalars becomes a typed table, and any other map or list becomes a tree. A value none of these holds exactly becomes a fenced yaml block, and the converter and mage fmt report it with its field and the reason. The printer decides by printing a candidate and parsing it back, so a rendering is chosen only when it is exact. A declared field is held to its declaration instead: the printer refuses a value its declared shape or conversion cannot carry rather than fall back, so canonicalization, not a silent fallback, decides what a field holds. Record keys keep their order, so a document's key order survives as its field order.

<!-- section: typed-forms -->
## Typed forms

Prose reads back as a string, and a plain list or table reads back as strings, because that is how every document printed before GH-2638 was read; those rules do not change, so no existing document changes meaning. A value that is not a string, or that holds structure, takes a typed form, and its field marker says so with the attribute form: `<!-- field: test_case_count | form: "scalar" -->`. Every field marker admits form, whatever the node declares. A field without the attribute reads exactly as before.

Inside a typed form every value is a YAML flow value on one line. A number, boolean, or null prints bare; a string prints bare when YAML would read it back as the same string, and double-quoted otherwise, so 12 is a number and "12" a string, and "#601" keeps its hash. A list prints as a flow sequence, [srd001 R1, srd001 R2], when its items are scalars. Multi-line text is the one exception, below.

The scalar form holds one such value on one line. The typed table form is a GFM table with one column per key, in the first record's key order extended by keys later records add; a cell holds the record's value under its column, and an empty cell means the record has no such key. An empty string prints as "" so it stays distinct from an absent key, and a pipe inside a cell is escaped as \|.

The keyed table form holds records keyed by id: a map of two or more records that share their attributes, each record holding at least half of the attributes any record holds and the attributes numbering at most two more than the largest record. It prints as a typed table whose first column, id, holds the map key, and reads back as the same map. Such a map is data, a table of named things such as relationships or metric definitions, so its ids are not fields and the constitution does not list them (GH-2662).

The tree form prints a map as a bullet list, one entry per key: `- key: value` for a scalar, or `- key:` followed by the nested value indented by two spaces. It prints a list as a numbered list, `1. value` for a scalar element, or `1.` followed by the nested value. Bullets mean map entries and numbers mean list elements, so the reader and the parser tell a list of maps from a map of lists without punctuation. A multi-line string prints after `- key:` as indented Markdown text, so a Mermaid diagram under a key draws, and it reads back verbatim with the indentation removed. An empty map or list inside a tree prints as {} or []. A field whose whole value is an empty list or map prints the line None, with form: empty list or form: empty map, so the reader sees a word and the parser restores the exact empty value.

Table 8: Typed forms, one example each

| Form | YAML value | Markdown |
| --- | --- | --- |
| scalar | test_case_count: 12 | `<!-- field: test_case_count \| form: "scalar" -->`, then the line 12 |
| typed table | roadmap_summary: [{version: "00.0", use_cases_done: 2}] | header \| version \| use_cases_done \|, row \| "00.0" \| 2 \| |
| keyed table | relationships: {ToolDef_to_ToolSpec: {meaning: ..., invariant: ...}} | header \| id \| meaning \| invariant \|, row \| ToolDef_to_ToolSpec \| ... \| ... \| |
| tree | inputs: {profile: p.yaml, retries: 3, words: [read, edit]} | - profile: p.yaml, - retries: 3, - words:, then 1. read and 1. edit indented |

A key the tree form cannot print as a bare name, text whose first line would read back as a list item, and a table cell holding a newline keep the fenced yaml block, reported.

<!-- section: round-trip-laws -->
## Round-trip laws

The laws are stated on the node tree, with comments removed and after canonicalization (next section). For every tracked document of every structured type, yaml to node to Markdown to node is the identity, Markdown is a fixed point of parse then print on canonical form, and the converter is idempotent. For the types that have a typed model, the model parsed from the Markdown front end equals the model parsed from the YAML one. The laws cover every shape in Table 3, the typed forms included, and the printer holds itself to them: it keeps a rendering only after parsing it back to the same node. Two details are part of the canonical form. Parsing returns a record's keys in one order, id, title, content, attributes, then its children in document order, and canonicalization puts the YAML records in that order before printing. Strings compare without their trailing newlines, the whitespace normalization the converter reports. GH-2605 proves the laws on the real corpus. GH-2606 adds the batch converter: in agent-core, mage convert <scope> converts one document (a path from the repository root), one type, or all of them in place, verifies each conversion against the laws before it writes, and prints the report of canonical changes, dropped comments, trailing newlines, and findings. Converting a converted document changes nothing. Conversion runs one way: the inverse, mage convertBack, served rollback during the migration and left with the sunset (GH-2626).

<!-- section: comments-and-forms -->
## Comments and two-form fields

YAML comments do not survive conversion. The tracked corpus holds 270 non-license comment lines in 35 files of the structured types, and they fall into two groups. Some carry facts a reader needs, such as the note in four catalog SRDs that the agent-core SRDs they cite are platform dependencies. Others restate structure: 10 repeat the document title on its first line, and 26 are blank or divider lines. Neither group belongs in a marker, since the one-layer rule keeps markers for machine-only facts, and placing a free note inside a typed field would break its shape. The converter therefore reports every comment it does not carry, with file and line, and the corpus migration (GH-2584) turns each comment that states a fact into visible text in the same pull request, called out as a hand edit.

Some fields are written in more than one YAML form, and one Markdown form cannot parse back to two. The converter canonicalizes each document in memory, once, before it prints, and reports every change it makes: frontmatter keys move first, a record holding only its id and content takes the keyed form, any other record keeps the record form with its keys in canonical order, map records written as a list become a map, a key whose records have no heading and which holds none is dropped, a non-text id becomes text, a record without an id derives one from its title where its mapping says so, and a keyed table row splits at its declared separator. We first planned a commit that rewrote the YAML into canonical form; it would have re-emitted every SRD through a YAML encoder just before the sunset deletes the YAML, so the canonical form exists only on the way to Markdown. The typed-model equality of the round-trip laws proves it changes no value. Table 9 counts the entries canonicalization touches.

Table 9: Fields canonicalized at conversion

| Field | Forms in the corpus | Entries normalized |
| --- | --- | --- |
| SRD goals | 404 keyed, 33 records with id and text, 36 numbered strings | 69 become keyed |
| SRD requirement groups | 99 SRDs as a map, 2 as a list, 5 relocated stubs with an empty map | 2 SRDs become a map; 5 empty maps are dropped |
| SRD requirement items | 2467 keyed, 160 records with more keys | none |
| use case flow | 403 keyed, 262 records with id and text, 47 records with traces, 1 numbered string | 263 become keyed |
| use case touchpoints | 309 keyed strings, 272 records, 17 unnumbered strings | 17 numbered in a corpus commit; 326 keyed strings split into records |

The keyed touchpoint strings split at their double hyphen into target and reason, and every one of them has one. Lists inside passthrough values keep whatever forms they have, because the fenced yaml block holds any node exactly.

<!-- section: identifiers -->
## Identifiers

Identifiers follow D6 in design.yaml: an id a release contains is never renumbered or reused, and a retired node keeps its id with retired_in and replaced_by attributes pointing at its successor. The format only adds where the id sits: in the marker value, and for heading forms id and id_title also in the heading text, where heading-id-mismatch keeps the two equal.

Requirement item ids match the pattern below (GH-2581 decision 1). The two-level form R1.1 is canonical for new items. The wider forms exist because 18 ids in 3 SRDs already use them (R3.1b, R10.0.1, R3.1.1), and D6 forbids renumbering them.

```text
^R[0-9]+(\.[0-9]+){1,2}[a-z]?$
```

A converter assigns new numbers only where the YAML had none: 17 unnumbered use case touchpoints in 3 files and 1 unnumbered flow step. Those are new ids, so no existing id changes.

<!-- section: diagnostics -->
## Diagnostics

An agent authoring a document iterates write, audit, fix, so every failure is a named finding that says how to fix it. Diagnostics come in tiers, and the run collects every finding rather than stopping at the first. Table 10 gives the tiers.

Table 10: Diagnostic tiers

| Tier | Covers | Handling |
| --- | --- | --- |
| 0 | canonical form: whitespace, spacing, marker spelling, table alignment | the formatter fixes it; never reported |
| 1 | grammar: document type, frontmatter, markers | named finding |
| 2 | structure: headings, nodes, ids, fields, required content | named finding |
| 3 | shapes: field shapes, list item patterns, table columns, attribute shapes | named finding |
| 4 | prose containment | named finding |

Parsing stops at a finding of tiers 1 to 3; prose-containment findings come back with the parsed tree, since they concern wording rather than structure. Each finding carries its name, tier, file, line, what the rule expected, what the document has, the rule path inside design.yaml (document_types.<type>.markdown.<rule>), and a fix hint. The names are stable: tools and agents match on them, so renaming one is a breaking change. Table 11 is the complete catalog.

Table 11: Finding catalog

| Tier | Name | Reported when |
| --- | --- | --- |
| 1 | document-type-unknown | no declared Markdown location matches the file's path |
| 1 | frontmatter-missing | the file does not open with a closed frontmatter block |
| 1 | frontmatter-invalid | the frontmatter is not a YAML mapping |
| 1 | frontmatter-missing-key | a field frontmatter.required names is absent |
| 1 | frontmatter-unknown-key | a frontmatter field is not in frontmatter.allowed |
| 1 | frontmatter-id-mismatch | the id differs from the file name where the type requires them equal |
| 1 | marker-malformed | a marker lacks the tag-colon-value form or an attribute is malformed |
| 1 | marker-unknown-tag | the tag is not one the type declares |
| 1 | marker-unknown-attribute | the attribute is not one the node declares |
| 1 | marker-orphan | no heading follows the marker |
| 2 | heading-untagged | a heading has no marker above it |
| 2 | heading-misplaced | the heading depth or parent does not fit any node declaration |
| 2 | id-malformed | the id does not match the node's id pattern |
| 2 | id-outside-parent | the id does not extend its parent's id |
| 2 | heading-id-mismatch | the heading text disagrees with the marker id |
| 2 | duplicate-id | an id the type declares unique appears twice |
| 2 | field-disallowed | a typed field appears under a node its declaration does not list |
| 2 | field-duplicate | a node holds the same field twice |
| 2 | content-untagged | content sits under a node whose content kind is none, or before any heading |
| 2 | untagged-structure | a prose node holds a list or table that belongs in its own field |
| 2 | missing-content | a prose node is empty |
| 2 | missing-required-field | a document or node lacks a field it requires |
| 2 | missing-required-node | the document holds fewer nodes of a kind than required |
| 2 | field-undeclared | a field marker names a field its slot's fields list does not hold |
| 2 | field-out-of-order | a field appears after a field its slot's fields list puts later |
| 3 | field-shape-mismatch | a typed field's content is not its declared shape |
| 3 | list-item-pattern | a list item does not match the field's item pattern |
| 3 | table-columns-mismatch | a table's header differs from the declared columns |
| 3 | attribute-shape | an attribute value is not JSON of its declared shape |
| 4 | prose-containment | a normative keyword appears in prose outside the normative nodes |

The audit reads a Markdown corpus document through the same declarations, which agent-core embeds from design.yaml so a binary, an image, or a consumer repository reads them without locating the file (GH-2614). It reports each diagnostic under the audit check markdown-format, with the finding name as its check id and in its message. One more audit check, not-canonical, reports a Markdown document that differs from its formatted form, naming the first line that differs with what it reads and what the canonical form has, so a nested field written as a tree, a flat record list written as a tree, a keyed table written as a tree, a missing form attribute, a misspelled generated heading, or a reference link with the wrong target is found and named; mage fmt fixes every one (GH-2678). The migration window had a second, dual-format-document, for a document present as both .yaml and .md; the sunset removed it with the YAML front end (GH-2626). A document with a finding of tiers 1 to 3 is left out of the corpus and the rest loads; when a reference to it then dangles, the load error names the format finding behind it.

The graph and semantic checks the audit already runs (retired-id-missing, uncovered-req-item, unresolved-clarification, invalid-priority, and the rest) are unchanged. They run over the same model, so their names and coverage carry over to the Markdown corpus.

<!-- section: prose-containment -->
## Prose containment

Nothing normative exists only in prose. A normative keyword in a paragraph or list outside the nodes a type declares normative is a prose-containment finding: the obligation either moves into a requirement item, or the text is reworded as description. Code blocks and tables are not prose, so they do not trip the check. The keyword list is data, declared per type in prose_containment.keywords, and it holds the RFC 2119 keywords in capitals plus the lowercase forms below.

```text
MUST, MUST NOT, REQUIRED, SHALL, SHALL NOT, SHOULD, SHOULD NOT,
RECOMMENDED, MAY, OPTIONAL, shall, shall not
```

The normative nodes are the SRD requirement items, the constitution articles, and a config format's validation rules, which state what the format's validator enforces (GH-2658); everywhere else the check applies. Non-goals stay inside it (GH-2581 decision 2): a non-goal phrased as an obligation is a requirement nobody traces, or a scope statement in the wrong voice.

The check reports what the corpus migration (GH-2584) rewrites. On the printed form of every tracked document it finds 17 hits: 15 in 5 SRDs, 14 of them in non-goals, and 2 in one engineering guideline (eng02 quoting srd020 R7.4). A scan of the YAML sources had also counted 61 hits in 4 config formats and 2 in one constitution; every one of them sits inside a value that prints as a fenced yaml block, which is code, so the check does not read it. The typed forms that replace those blocks (GH-2638) hold the same data, so the check does not read a field whose marker names a form either: a rendering change does not change which text the rule governs. Prose a field holds as text is read, and four permissions that printed as code before GH-2641 were reworded as description. A normative keyword inside a nested passthrough value therefore escapes prose containment, and a reviewer of the migration reads those blocks. A nested field is not such a value: its text is prose under its own heading, so the check reads it, and declaring a type's nested fields rewords the obligations they held as description (GH-2651).

<!-- section: rules-as-data -->
## Rules as data

Every entry in document_types carries a markdown entry, and a test in agent-core/pkg/spec/markdown holds design.yaml to that. A structured type declares its rules there; a type that takes no rules says why in one line. Table 12 lists the types.

Table 12: Markdown declarations by document type

| Type | Location | Declaration |
| --- | --- | --- |
| vision | docs/VISION.md | fields; success criteria as a keyed list; 8 field slots listing 36 fields in order |
| architecture | docs/ARCHITECTURE.md | interface, component, and decision nodes; two tables; overview architectures and package records; 26 field slots listing 158 fields in order |
| specifications | docs/SPECIFICATIONS.md | overview; index tables pass through; 4 field slots listing 26 fields in order |
| roadmap | docs/road-map.md | release nodes with a status attribute; 6 field slots listing 56 fields in order |
| pattern_language | docs/pattern-language.md | pattern nodes requiring intent, problem, solution; 10 field slots listing 45 fields in order |
| engineering_guideline | docs/engineering/engNN-short-name.md | titled section nodes; 1 field slots listing 2 fields in order |
| srd | docs/specs/software-requirements/srdNNN-short-name.md | requirement groups, items, criteria; 18 field slots listing 114 fields in order |
| use_case | docs/specs/use-cases/relNN.N-ucNNN-short-name.md | success criterion nodes; flow, touchpoint, and scenario fields; 6 field slots listing 59 fields in order |
| test_suite | docs/specs/test-suites/test-relNN.N-short-name.md | test case nodes with visible traces; 5 field slots listing 69 fields in order |
| audit_register | docs/specs/audits/name-register.md | required record fields; 24 field slots listing 130 fields in order |
| audit_report | docs/specs/audits/name.md | none: native Markdown already |
| semantic_model | docs/specs/semantic-models/name.md | purpose and list fields; tool and role records; 37 field slots listing 294 fields in order |
| config_format | docs/specs/config-formats/name.md | purpose and list fields; schema and validation-rule records; 96 field slots listing 409 fields in order |
| constitution | docs/constitutions/name.md | article nodes, the normative blocks; 32 field slots listing 121 fields in order |
| migration | docs/migrations/name.md | lifecycle frontmatter; summary and evidence fields; 8 field slots listing 42 fields in order |
| guide | docs/guides/name.md | none: native Markdown already |
| stats_baseline | docs/stats/name.json | none: generated JSON |
| corpus_charter | docs/corpus-charter.yaml | none: stays YAML, an executable check plan |

A file selects its declaration by its whole path, matched at a path boundary, because several types share the name placeholder and differ only by directory. A run of N is a digit placeholder only after a lowercase letter or a dot, so VISION stays a word; the placement gate reads locations by the same rule. Audit registers take a -register suffix so they do not collide with the audit reports beside them.

Two types and one document stay outside the format by design. The corpus charter is configuration the audit executes (spec.Charter), not a document a person reads, and the stats baseline is generated output. The design constitution, agent-core/docs/constitutions/design.yaml, holds the document_types registry these declarations live in; the declaration loader, the placement and reference gates, and the converter read it, so the constitution declaration excludes it (exclude) and it stays YAML while the other constitutions convert.

<!-- section: corpus-fit -->
## Corpus fit

Every tracked document of every structured type round-trips under the round-trip laws: 369 documents, of which mage convert converts 368 (the design constitution stays YAML), from canonical YAML to Markdown and back to the same tree, with Markdown a fixed point of Format and the typed model unchanged where the type has one (TestEveryCorpusSRDRoundTrips and TestEveryCorpusDocumentRoundTrips in agent-core/pkg/spec/markdown). Table 13 gives the counts by type.

After the rendering work of GH-2638 the printer writes no fenced yaml block anywhere in the corpus, down from 1,715 in 229 documents. Of the values that printed as YAML, 1,572 fields now print in the tree form, 85 as typed tables, and 27 as scalars; the 61 test-suite commands print as bash fences, and 132 Mermaid blocks draw. One fenced yaml block remains by intent: a code example inside the text of agent-core ARCHITECTURE decision 5, which an earlier indented example would have turned into an untagged list. mage fmt reports any fallback fence the printer writes again.

Table 13: Corpus fit of the declarations

| Type | Documents | Round-trip | Prose-containment hits |
| --- | --- | --- | --- |
| vision | 6 | 6 | 0 |
| architecture | 6 | 6 | 0 |
| specifications | 6 | 6 | 0 |
| roadmap | 6 | 6 | 0 |
| pattern_language | 1 | 1 | 0 |
| engineering_guideline | 9 | 9 | 2 in eng02 |
| srd | 106 | 106 | 15 in 5 files |
| use_case | 110 | 110 | 0 |
| test_suite | 82 | 82 | 0 |
| audit_register | 2 | 2 | 0 |
| semantic_model | 11 | 11 | 0 |
| config_format | 9 | 9 | 0 |
| constitution | 11 | 11 | 0 |
| migration | 4 | 4 | 0 |

The corpus forced several changes to the declarations of GH-2581, and we made them rather than bend the corpus to a guess. Ids in config formats, semantic models, and migrations carry a type prefix (cfg-, sm-, migration-), and an audit register's Markdown name adds -register to its id, so those types no longer require the id to equal the file name. The specification indexes, the roadmap's per-release lists, and the architecture's related documents differ in columns, cell types, or line wrapping between modules, so they pass through instead of being typed. Some records lack a field the declaration required: 7 of 88 releases have no description, 25 of 491 use case success criteria and 1 of 757 test cases have no traces, so those fields are optional for those kinds. Ids are unique within their kind, since an architecture names one interface and one component lifecycle-exit client. Integer design-decision ids are written as text, guideline sections, which have no ids, take one derived from their title, and keyed touchpoints split at their double hyphen into id, target, and reason.

Three kinds of corpus change landed in their own commit, because no canonical form could express them without inventing a value: the 17 unnumbered touchpoints in three chatbot-mesh use cases received numbers, one config format's implementation became a one-item list, and one semantic model's two-paragraph non-goals became a two-item list. The typed parser also changed in one place: a record-form touchpoint now keeps its id label in the model string, as the keyed form does, so both forms read the same; every consumer strips the label. One gap stays open as GH-2611: ParseUseCase rejects a flow step record that carries traces, which six use cases under applications/docs use, so the round trip checks that the typed parser rejects them the same way from either form.

<!-- section: rendering -->
## Rendering

GitHub hides HTML comments and renders frontmatter as a table, so a converted document shows its headings, prose, lists, tables, and Mermaid diagrams and none of its markers. Until GH-2638 a value with no simple shape printed as a fenced yaml block, and GitHub showed its YAML source: a survey of the corpus found 1,715 such blocks in 229 documents, among them 76 Mermaid diagrams that never drew. The typed forms replace them, and a heading generated from a field name keeps its acronyms (SRD index, ID, API, UI, UX, LLM, CLI, REST, YAML, URL, JSON, HTTP, OTel). For GitLab we rely on its documentation, which states that comments are hidden and that frontmatter is shown in a box above the document (GH-2581 decision 4). That is an unverified assumption, and the verification step is named: render one converted document on a GitLab instance. The first GitLab mirror of the repository closes it.

<!-- section: authoring-loop -->
## Authoring loop

An author, human or agent, writes a document, runs mage fmt, runs mage audit, and fixes what the audit reports. mage fmt <scope> from the repository root applies Tier 0 in place, so the audit's findings concern meaning, plus not-canonical for a document edited since its last format. Every audit gate reads a corpus document as the YAML tree its declaration parses it into (GH-2614, GH-2615, GH-2616), and none reads a YAML corpus document in the working tree (GH-2626). A script that checks document fields reads them from mage specYAML <path> in agent-core, which prints a document's tree, or for a directory a mapping from each document's path to its tree (GH-2627).

The loop terminates on the hints alone. TestAuthoringLoopFollowsTheHints in agent-core/pkg/spec/markdown seeds an SRD with a finding of every tier and drives it to zero findings, one finding per round, with a fixer that may use only what a finding says: its line, its expected and found text, and its hint. A hint that stops naming what its fix needs stalls the loop and fails the test.

For one file at a time, a mage target in agent-core runs the same diagnostics, with paths relative to agent-core and separated by commas:

```text
cd agent-core
mage specCheck ../docs/engineering/eng10-markdown-spec-format.md
```

Each finding names the rule in design.yaml it enforces, so an author who disagrees with a finding knows which declaration to change, through an issue that amends this guideline.

<!-- field: references -->
## References

- agent-core/docs/constitutions/design.yaml
- docs/engineering/eng09-spec-kit-correspondence.md
- docs/engineering/eng03-declaration-standard-library.md
- agent-core/pkg/spec/markdown/FINDINGS.md
- https://www.rfc-editor.org/rfc/rfc2119
- https://docs.gitlab.com/user/markdown/
- https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2578
- https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2581
- https://github.com/Nokia-Bell-Labs/declarative-agents/issues/2597
