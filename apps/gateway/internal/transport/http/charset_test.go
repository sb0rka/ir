package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRespondJSONDeclaresUTF8(t *testing.T) {
	response := httptest.NewRecorder()
	respondJSON(response, http.StatusOK, map[string]string{"title": "Сработка правила корреляции"})
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type=%q", got)
	}
	if got := response.Body.String(); got != "{\"title\":\"Сработка правила корреляции\"}\n" {
		t.Fatalf("body=%q", got)
	}
}
