module github.com/Nokia-Bell-Labs/declarative-agents/agent-core/magefiles

go 1.26.3

require (
	github.com/Nokia-Bell-Labs/declarative-agents/agent-core v0.0.0
	github.com/magefile/mage v1.17.2
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dominikbraun/graph v0.23.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/Nokia-Bell-Labs/declarative-agents/agent-core v0.0.0 => ../
