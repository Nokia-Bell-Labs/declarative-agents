module github.com/Nokia-Bell-Labs/declarative-agents/applications/agent-architecture

go 1.26.5

require (
	github.com/Nokia-Bell-Labs/declarative-agents/magefiles v0.0.0-00010101000000-000000000000
	github.com/magefile/mage v1.17.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/yuin/goldmark v1.4.13 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
)

replace github.com/Nokia-Bell-Labs/declarative-agents/magefiles => ../../magefiles

tool golang.org/x/tools/cmd/present
