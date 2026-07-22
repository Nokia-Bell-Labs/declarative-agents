# REST conformance fixtures

This directory is the canonical owner of the REST profile fixtures. The seven
`ollama-*` files, including `openapi/ollama.yaml`, are mirrored under
`agent-core/internal/tools/rest/testdata/ollama_profile` so agent-core's
package-local OpenAPI tests remain hermetic.

`TestOllamaRESTFixtureMirrorMatchesCanonicalProfile` compares every mirrored
file byte-for-byte. Update the canonical files first, copy the same bytes into
the agent-core mirror, and run the conformance test.
