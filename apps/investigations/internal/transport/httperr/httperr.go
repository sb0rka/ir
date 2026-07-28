// Package httperr переводит доменные ошибки в конверт §8.3 спеки.
package httperr

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type Code string

const (
	CodeUnauthorized      Code = "unauthorized"
	CodeForbidden         Code = "forbidden"
	CodeNotFound          Code = "not_found"
	CodeConflict          Code = "conflict"
	CodeValidation        Code = "validation"
	CodeSourceUnavailable Code = "source_unavailable"
	CodeNotImplemented    Code = "not_implemented"
	CodeInternal          Code = "internal"
)

// Error — доменная ошибка, знающая свой HTTP-код. Хендлеры возвращают её,
// транспорт разворачивает в конверт.
type Error struct {
	Status  int
	Code    Code
	Message string
	Details map[string]any
}

func (e *Error) Error() string { return e.Message }

func New(status int, code Code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// WithDetails возвращает копию. Мутировать приёмник нельзя: сентинелы ниже —
// общие на весь процесс, и один вызов на ErrNotFound подмешал бы свои детали
// во все последующие 404.
func (e *Error) WithDetails(details map[string]any) *Error {
	clone := *e
	clone.Details = details
	return &clone
}

var (
	ErrNotImplemented = New(http.StatusNotImplemented, CodeNotImplemented, "not implemented")
	ErrUnauthorized   = New(http.StatusUnauthorized, CodeUnauthorized, "unauthorized")
	ErrForbidden      = New(http.StatusForbidden, CodeForbidden, "forbidden")
	ErrNotFound       = New(http.StatusNotFound, CodeNotFound, "not found")
)

func BadRequest(message string) *Error {
	return New(http.StatusBadRequest, CodeValidation, message)
}

type body struct {
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type envelope struct {
	Error body `json:"error"`
}

// Write отправляет ошибку клиенту. Текст неизвестной ошибки наружу не уходит:
// он попадает в лог, клиент получает общее сообщение.
func Write(w http.ResponseWriter, log *slog.Logger, err error) {
	var domain *Error
	if !errors.As(err, &domain) {
		if log != nil {
			log.Error("unhandled_error", "error", err)
		}
		domain = New(http.StatusInternalServerError, CodeInternal, "internal server error")
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(domain.Status)
	_ = json.NewEncoder(w).Encode(envelope{Error: body{
		Code:    domain.Code,
		Message: domain.Message,
		Details: domain.Details,
	}})
}
