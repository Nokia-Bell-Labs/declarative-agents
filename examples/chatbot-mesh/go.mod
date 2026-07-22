module github.com/Nokia-Bell-Labs/declarative-agents/examples/chatbot-mesh

go 1.26.3

require (
	github.com/magefile/mage v1.17.2
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/Nokia-Bell-Labs/declarative-agents/magefiles v0.0.0

replace github.com/Nokia-Bell-Labs/declarative-agents/magefiles => ../../magefiles
