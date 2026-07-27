package transport

import (
	"encoding/json"
	"net/http"

	"github.com/sb0rka/ir/packages/contract/spec"
)

// Спека вкомпилирована в бинарь генератором (embedded-spec), поэтому
// документация всегда соответствует запущенной версии сервиса — расхождение
// «в UI одно, отвечает другое» невозможно.

func openAPISpec(w http.ResponseWriter, _ *http.Request) {
	doc, err := spec.GetSwagger()
	if err != nil {
		http.Error(w, "spec unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(doc)
}

func swaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerPage))
}

const swaggerPage = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <title>Investigations API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: '/openapi.json',
      dom_id: '#ui',
      persistAuthorization: true,
      tryItOutEnabled: true
    });
  </script>
</body>
</html>`
