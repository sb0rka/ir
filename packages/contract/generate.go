// Package contract содержит DTO и серверные интерфейсы, сгенерированные из
// OpenAPI-спеки в api/. Файлы *.gen.go редактировать нельзя — правь спеку
// и запускай `task generate-go`.
package contract

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=oapi-codegen.yaml ../../api/bundle.yaml
