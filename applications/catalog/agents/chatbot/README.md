# chatbot (relocated)

The chatbot agent is no longer maintained in the applications/catalog catalog. The mesh
was extracted to a standalone example, and the chatbot program now lives, as the
single canonical copy, at:

    applications/chatbot-mesh/agents/chatbot/

The catalog copy here had diverged from the example (its machine, rest, and UX
config forked, and its UI still targeted the removed provisioner), so it was
removed to end the duplication (GH-511). The example's integrations and Helm
chart consume the canonical copy, and the shipped-UI reproducibility gate
(mage uiDist) keeps its served bundle in step with source.

The applications/catalog rel09 mesh specifications (srd014, srd015, rel09.*) are the
historical record of the mesh's development here before extraction; see
applications/chatbot-mesh/docs for the canonical specifications.

This relocation follows the library membership rule: chatbot is application
composition with one consumer, not an independently reusable library member.
Therefore its canonical program stays with chatbot-mesh. If a future reusable
chat role emerges with multiple consumers and satisfies the library obligations
(family SRD, conformance, parameterization, and portable closure), it must be
promoted deliberately rather than copied back here.
