---
id: states
title: Issue states
---

<!-- field: version -->
## Version

1

<!-- field: rows -->
## Rows

| from | to | actor | assignee | requires | kind |
| --- | --- | --- | --- | --- | --- |
| open | needs-triage | lint | none |  |  |
| open | ready | lint | none |  |  |
| open | taken | pop | set |  |  |
| needs-triage | ready | lint | none |  |  |
| needs-triage | taken | lint | none |  |  |
| ready | taken | pop | set |  |  |
| open | open | assign | set |  |  |
| ready | ready | assign | set |  |  |
| taken | ready | person | clear |  |  |
| taken | in-progress | person | none |  |  |
| in-progress | taken | person | none |  |  |
| any-open | +waiting | track-goals | none |  |  |
| any-open | -waiting | track-goals | none |  |  |
| taken, in-progress | +blocked | person | none |  |  |
| taken, in-progress | -blocked | person | none |  |  |
| any-open | +needs-run | handoff | none | type::recurring |  |
| any-open | closed | merge | none |  |  |
| any-open | closed | track-goals | none |  | sprint-goal |
| any-open | closed | person | none |  |  |
| closed | open | person | none |  |  |
