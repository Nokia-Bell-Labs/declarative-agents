# Specification-Driven Development

The declarative-agents repository practices specification-driven development.
Specifications are the source of truth, and code serves the specifications.
No implementation code is written before a requirements document and a use
case exist, and no implementation issue is opened before a test suite covers
its use case.

The traceability chain runs from the vision, to the architecture, to the
software requirements documents, to the use cases, to the test suites, and
finally to the code. Every requirements document traces to the vision and the
architecture. Every use case traces to a requirements document. Every test
suite validates a use case.
