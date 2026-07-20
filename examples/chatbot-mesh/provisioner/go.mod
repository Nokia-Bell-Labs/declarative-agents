// The chatbot-mesh provisioner is the deployment-plane values-patch service the
// provisioning panel drives (srd003 R4). It is deployment tooling, not an agent,
// so it lives with the chart and has no agent-core dependency. Standard library
// only.
module github.com/Nokia-Bell-Labs/declarative-agents/examples/chatbot-mesh/provisioner

go 1.26.3
