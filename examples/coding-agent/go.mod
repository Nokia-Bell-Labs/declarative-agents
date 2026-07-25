module github.com/Nokia-Bell-Labs/declarative-agents/examples/coding-agent

go 1.26.3

require (
	github.com/magefile/mage v1.17.2
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/Nokia-Bell-Labs/declarative-agents/magefiles v0.0.0-00010101000000-000000000000

replace github.com/Nokia-Bell-Labs/declarative-agents/magefiles => ../../magefiles
