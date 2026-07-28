// Package model — доменные типы. Зависимостей от транспорта и стора нет.
package model

// SubjectRoles — тенант субъекта и его роли SOC. Вокабуляр ролей свой:
// с платформенными owner/admin/editor/viewer он не совпадает.
type SubjectRoles struct {
	ProjectID string
	Roles     []string
}
