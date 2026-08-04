module github.com/Nokia-Bell-Labs/declarative-agents/applications/prose-editor

go 1.26.5

require (
	github.com/Nokia-Bell-Labs/declarative-agents/magefiles v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/magefile/mage v1.17.2 // indirect

replace github.com/Nokia-Bell-Labs/declarative-agents/magefiles => ../../magefiles

tool github.com/magefile/mage
