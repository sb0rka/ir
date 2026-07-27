// Package model — доменные типы. Зависимостей от транспорта и стора нет.
package model

const (
	RoleL1    = "l1"
	RoleL2    = "l2"
	RoleLead  = "lead"
	RoleAdmin = "admin"
)

// SubjectRoles — тенант субъекта и его роли SOC. Вокабуляр ролей свой:
// с платформенными owner/admin/editor/viewer он не совпадает.
type SubjectRoles struct {
	ProjectID string
	Roles     []string
}

type Source struct {
	Code      string
	Kind      string
	Title     string
	SecretRef *string
	IsEnabled bool
}

type EntityType struct {
	Code  string
	Title string
}

type RelationType struct {
	Code       string
	Title      string
	SourceKind string
	TargetKind string
	Directed   bool
}
