module github.com/sb0rka/ir/apps/gateway

go 1.25.5

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/oapi-codegen/runtime v1.6.0
	github.com/sb0rka/ir/packages/common v0.0.0
	github.com/sb0rka/sb0rka/packages/core v0.0.2
)

replace github.com/sb0rka/ir/packages/common => ../../packages/common

require github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
