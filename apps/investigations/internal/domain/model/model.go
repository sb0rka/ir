// Package model — доменные типы. Зависимостей от транспорта и стора нет.
package model

// Вокабуляр ролей свой: с платформенными owner/admin/editor/viewer не совпадает.
type SubjectRoles struct {
	ProjectID string
	Roles     []string
}
