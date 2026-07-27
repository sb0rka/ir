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

// Версия закреплена точно, а не диапазоном `@5`, и подпись ассетов проверяется
// браузером: это чужой код, исполняющийся на нашем origin, и «свежая версия
// приедет сама» здесь не преимущество, а способ подменить страницу.
// Хеши пересчитывать при смене версии:
//
//	curl -sfL https://unpkg.com/swagger-ui-dist@<версия>/<файл> |
//	  openssl dgst -sha384 -binary | openssl base64 -A
const (
	swaggerVersion = "5.29.5"
	swaggerCSSSRI  = "sha384-++DMKo1369T5pxDNqojF1F91bYxYiT1N7b1M15a7oCzEodfljztKlApQoH6eQSKI"
	swaggerJSSRI   = "sha384-+//OXWv2MI+XGzCNZ1tyxL1lT/whLV95IujjmbHXUgGh80zv+9B0ii6pDIO3URWN"
)

var swaggerPage = `<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <title>Investigations API</title>
  <link rel="stylesheet"
        href="https://unpkg.com/swagger-ui-dist@` + swaggerVersion + `/swagger-ui.css"
        integrity="` + swaggerCSSSRI + `"
        crossorigin="anonymous">
</head>
<body>
  <div id="ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@` + swaggerVersion + `/swagger-ui-bundle.js"
          integrity="` + swaggerJSSRI + `"
          crossorigin="anonymous"></script>
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
