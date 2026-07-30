module github.com/Nokia-Bell-Labs/declarative-agents/applications/chatbot-mesh

go 1.26.3

require (
	github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog v0.0.0-00010101000000-000000000000
	github.com/magefile/mage v1.17.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Nokia-Bell-Labs/declarative-agents/magefiles v0.0.0
	go.opentelemetry.io/proto/otlp v1.11.0
	google.golang.org/grpc v1.82.1
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
	github.com/yuin/goldmark v1.4.13 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260720211330-0afa2a65878a // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260720211330-0afa2a65878a // indirect
)

replace github.com/Nokia-Bell-Labs/declarative-agents/magefiles => ../../magefiles

replace github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog => ../catalog

tool golang.org/x/tools/cmd/present
