module github.com/sb0rka/ir/apps/investigations

go 1.25.5

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/jsonschema-go v0.4.3
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.9.2
	github.com/modelcontextprotocol/go-sdk v1.7.0
	github.com/oapi-codegen/runtime v1.6.0
	github.com/sb0rka/ir/packages/common v0.0.0
	github.com/sb0rka/ir/packages/contract v0.0.0
	github.com/sb0rka/sb0rka/packages/core v0.0.2
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/getkin/kin-openapi v0.145.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)

replace github.com/sb0rka/ir/packages/common => ../../packages/common

replace github.com/sb0rka/ir/packages/contract => ../../packages/contract
