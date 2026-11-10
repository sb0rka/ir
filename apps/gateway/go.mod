module github.com/sb0rka/ir/apps/gateway

go 1.27.3

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/oapi-codegen/runtime v1.6.0
	github.com/sb0rka/ir/packages/common v0.0.0
	github.com/sb0rka/sb0rka/packages/core v0.0.7
)

replace github.com/sb0rka/ir/packages/common => ../../packages/common

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.9.2 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
