---
id: test-rel18.0-release-annotation
title: Release 18.0 Release Annotation Test Suite
release: "18.0"
status: implemented
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: test_case_count | form: "scalar" -->
## Test case count

8

<!-- field: overview -->
## Overview

TC1 parses and traces the Release 18.0 documents. TC2 through TC5 run the
verify target's scratch-repository tests, which touch neither this
repository's refs nor a network. TC6 checks the placement: the recurring
release runbook carries the verify step between annotation and push, and
the skill counterpart is coding-skills issue 395. TC7 and TC8 run the
released check and the publish step against fake forges and scratch
repositories.

<!-- field: traces -->
## Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: srds -->
## SRDs

- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md)

<!-- field: preconditions -->
## Preconditions

- Run commands from the repository root.
- Python 3 with PyYAML, Go, Mage, and git are available.
- A planned case is not passing evidence until its command exists and passes.

<!-- field: test_cases -->
## Test cases

<!-- case: Release 18.0 documents parse and trace reciprocally | status: implemented -->
### Release 18.0 documents parse and trace reciprocally

<!-- field: id -->
#### ID

TC1

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

executable_structural

<!-- field: command -->
#### Command

```bash
python3 - <<'PY'
from pathlib import Path
import subprocess
import yaml

root = Path("applications/docs")
paths = {
    "srd": root / "specs/software-requirements/srd007-release-annotation.md",
    "use_case": root / "specs/use-cases/rel18.0-uc001-release-annotation.md",
    "suite": root / "specs/test-suites/test-rel18.0-release-annotation.md",
    "index": root / "SPECIFICATIONS.md",
    "roadmap": root / "road-map.md",
    "guideline": Path("docs/engineering/eng01-kind-test-demo-rig.md"),
}
def load(path):
    # A corpus document is Markdown; mage specYAML prints the YAML tree
    # its declaration parses it into (GH-2627).
    out = subprocess.run(["mage", "specYAML", str(path)], cwd="agent-core",
                         check=True, capture_output=True, text=True).stdout
    return yaml.safe_load(out)

docs = {name: load(path) for name, path in paths.items()}
required = {
    "srd": {"id", "title", "problem", "goals", "requirements",
            "non_goals", "acceptance_criteria"},
    "use_case": {"id", "title", "summary", "actor", "trigger", "flow",
                 "touchpoints", "success_criteria", "out_of_scope"},
    "suite": {"id", "title", "release", "traces", "preconditions",
              "test_cases"},
    "guideline": {"id", "title", "introduction", "sections", "references"},
}
for name, fields in required.items():
    assert not fields - set(docs[name]), (name, fields - set(docs[name]))

srd, use_case, suite = docs["srd"], docs["use_case"], docs["suite"]
assert use_case["test_suite"] == suite["id"]
assert use_case["id"] in suite["traces"]
assert use_case["id"] in srd["use_case_traces"]
assert suite["id"] in srd["test_suite_traces"]

item_ids = {
    item["id"]
    for group in srd["requirements"].values()
    for item in group["items"]
}
ac_traces = {t for ac in srd["acceptance_criteria"] for t in ac["traces"]}
assert item_ids <= ac_traces, sorted(item_ids - ac_traces)
ac_ids = {ac["id"] for ac in srd["acceptance_criteria"]}
case_traces = {
    t.rsplit(" ", 1)[-1]
    for case in suite["test_cases"]
    for t in case.get("traces", [])
    if t.startswith(srd["id"] + " ")
}
assert ac_ids <= case_traces, sorted(ac_ids - case_traces)
assert len(suite["test_cases"]) == suite["test_case_count"]

titles = [section["title"] for section in docs["guideline"]["sections"]]
assert "Release annotation" in titles, titles
verbs = next(s for s in docs["guideline"]["sections"]
             if s["title"] == "Standard mage verbs")["content"]
assert "| tag:verify |" in verbs, "the root verb table does not carry tag:verify"
annotation = next(s for s in docs["guideline"]["sections"]
                  if s["title"] == "Release annotation")["content"]
assert srd["id"] in annotation

index = docs["index"]
assert any(e["id"] == srd["id"] for e in index["srd_index"])
assert any(e["id"] == use_case["id"] for e in index["use_case_index"])
assert any(e["id"] == suite["id"] for e in index["test_suite_index"])
assert any(e["version"] == "18.0" for e in index["roadmap_summary"])
release = next(r for r in docs["roadmap"]["releases"] if r["version"] == "18.0")
assert any(e["id"] == use_case["id"] for e in release["use_cases"])
assert any(e["id"] == suite["id"] for e in release["test_suites"])
print("validated Release 18.0 release annotation trace closure")
PY
```

<!-- field: expected | form: "tree" -->
#### Expected

- yaml_parses: true
- design_constitution_fields_present: true
- requirement_to_acceptance_criterion_closure: true
- reciprocal_srd_use_case_suite_traces: true
- guideline_carries_the_section_and_the_verb: true
- specification_and_roadmap_indexes_present: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S1](../use-cases/rel18.0-uc001-release-annotation.md#s1)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC1](../software-requirements/srd007-release-annotation.md#ac1)

<!-- case: A missing or lightweight tag is refused by name | status: implemented -->
### A missing or lightweight tag is refused by name

<!-- field: id -->
#### ID

TC2

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

go_test

<!-- field: implemented_by -->
#### Implemented by

GH-2420

<!-- field: command -->
#### Command

```bash
cd magefiles && go test . -run TestTagVerify
```

<!-- field: expected | form: "tree" -->
#### Expected

- missing_tag_refused: true
- lightweight_tag_refused: true
- each_refusal_names_its_rule: true
- annotated_tag_passes_this_rule: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S1](../use-cases/rel18.0-uc001-release-annotation.md#s1)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC1](../software-requirements/srd007-release-annotation.md#ac1)

<!-- case: A degenerate annotation is refused | status: implemented -->
### A degenerate annotation is refused

<!-- field: id -->
#### ID

TC3

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

go_test

<!-- field: implemented_by -->
#### Implemented by

GH-2420

<!-- field: command -->
#### Command

```bash
cd magefiles && go test . -run TestTagVerify
```

<!-- field: expected | form: "tree" -->
#### Expected

- subject_equal_to_tag_name_refused: true
- empty_subject_refused: true
- under_bar_summary_refused: true
- substance_bar_stated_once: true
- real_summary_passes: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S1](../use-cases/rel18.0-uc001-release-annotation.md#s1)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC1](../software-requirements/srd007-release-annotation.md#ac1)

<!-- case: A tag off main is refused and an absent main fails by name | status: implemented -->
### A tag off main is refused and an absent main fails by name

<!-- field: id -->
#### ID

TC4

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

go_test

<!-- field: implemented_by -->
#### Implemented by

GH-2420

<!-- field: command -->
#### Command

```bash
cd magefiles && go test . -run TestTagVerify
```

<!-- field: expected | form: "tree" -->
#### Expected

- side_branch_tag_refused: true
- absent_main_is_a_named_failure: true
- main_tag_passes_this_rule: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S2](../use-cases/rel18.0-uc001-release-annotation.md#s2)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC2](../software-requirements/srd007-release-annotation.md#ac2)

<!-- case: A run mutates nothing, passing or refusing | status: implemented -->
### A run mutates nothing, passing or refusing

<!-- field: id -->
#### ID

TC5

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

go_test

<!-- field: implemented_by -->
#### Implemented by

GH-2420

<!-- field: command -->
#### Command

```bash
cd magefiles && go test . -run TestTagVerify
```

<!-- field: expected | form: "tree" -->
#### Expected

- ref_set_unchanged_across_a_pass: true
- ref_set_unchanged_across_a_refusal: true
- working_tree_unchanged: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S3](../use-cases/rel18.0-uc001-release-annotation.md#s3)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC3](../software-requirements/srd007-release-annotation.md#ac3)

<!-- case: The runbook and the skill place the check before the push | status: implemented -->
### The runbook and the skill place the check before the push

<!-- field: id -->
#### ID

TC6

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

manual

<!-- field: implemented_by -->
#### Implemented by

GH-2420

<!-- field: command -->
#### Command

```bash
gh issue view 2198 --json body --jq .body | grep -c "tag:verify"
```

<!-- field: expected | form: "tree" -->
#### Expected

- runbook_carries_the_step_between_annotation_and_push: true
- skill_counterpart_filed_and_linked: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S4](../use-cases/rel18.0-uc001-release-annotation.md#s4)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC4](../software-requirements/srd007-release-annotation.md#ac4)

<!-- case: A tag install.sh cannot install is refused by name | status: implemented -->
### A tag install.sh cannot install is refused by name

<!-- field: id -->
#### ID

TC7

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

go_test

<!-- field: implemented_by -->
#### Implemented by

GH-2861

<!-- field: command -->
#### Command

```bash
cd applications/scrum-master && go test ./internal/release/ -run CheckReleased
```

<!-- field: expected | form: "tree" -->
#### Expected

- no_release_refused: true
- draft_refused: true
- missing_asset_refused: true
- forge_unchanged: true
- gitlab_release_checked: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S5](../use-cases/rel18.0-uc001-release-annotation.md#s5)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC5](../software-requirements/srd007-release-annotation.md#ac5)

<!-- case: A publish that did not complete cannot pass as a release | status: implemented -->
### A publish that did not complete cannot pass as a release

<!-- field: id -->
#### ID

TC8

<!-- field: use_case -->
#### Use case

[rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md)

<!-- field: evidence_type -->
#### Evidence type

go_test

<!-- field: implemented_by -->
#### Implemented by

GH-2861

<!-- field: command -->
#### Command

```bash
cd magefiles && go test . -run "TestTagPublish|TestRequirePreviousReleasePublished|TestNewestReleaseTag"
```

<!-- field: expected | form: "tree" -->
#### Expected

- refuses_before_publishing: true
- publishes_then_checks: true
- failed_step_fails_the_target: true
- next_release_stops_on_an_unpublished_tag: true

<!-- field: traces -->
#### Traces

- [rel18.0-uc001-release-annotation](../use-cases/rel18.0-uc001-release-annotation.md) [S6](../use-cases/rel18.0-uc001-release-annotation.md#s6)
- [srd007-release-annotation](../software-requirements/srd007-release-annotation.md) [AC6](../software-requirements/srd007-release-annotation.md#ac6)
