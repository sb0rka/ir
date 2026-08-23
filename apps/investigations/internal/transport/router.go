package transport

import (
	"log/slog"
	"net/http"

	"github.com/sb0rka/ir/apps/investigations/internal/config"
	"github.com/sb0rka/ir/apps/investigations/internal/transport/httperr"
	"github.com/sb0rka/ir/packages/contract/entities"
	"github.com/sb0rka/ir/packages/contract/events"
	"github.com/sb0rka/ir/packages/contract/graph"
	"github.com/sb0rka/ir/packages/contract/investigations"
	"github.com/sb0rka/ir/packages/contract/reference"
	"github.com/sb0rka/ir/packages/contract/som"
)

const baseURL = "/api/v1"

// API — интерфейс на домен, а не один общий: забытый домен виден сразу.
type API interface {
	entities.StrictServerInterface
	events.StrictServerInterface
	graph.StrictServerInterface
	investigations.StrictServerInterface
	reference.StrictServerInterface
	som.StrictServerInterface
}

type Dependencies struct {
	Cfg    config.ServerConfig
	Log    *slog.Logger
	Server API
}

func NewHandler(deps Dependencies) http.Handler {
	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", liveness)
	public.HandleFunc("GET /ping", ping)
	// Документация без авторизации: она нужна до получения токена.
	public.HandleFunc("GET /openapi.json", openAPISpec)
	public.HandleFunc("GET /swagger", swaggerUI)

	// Домены живут на отдельном mux, и он целиком закрыт авторизацией.
	// Через options.Middlewares контракта это не сделать: сгенерированная
	// обёртка сначала разбирает параметры и только потом вызывает middleware,
	// то есть неаутентифицированный запрос успевал бы получить 400 от
	// валидации вместо 401.
	api := http.NewServeMux()
	registerDomains(api, deps)
	public.Handle(baseURL+"/", authMiddleware(deps.Cfg, deps.Log)(api))

	// Логгер снаружи recover: он оборачивает ResponseWriter, а recover по этой
	// обёртке понимает, была ли уже запись в ответ.
	return chain(public,
		loggerMiddleware(deps.Log),
		recoverMiddleware(deps.Log),
		corsMiddleware(deps.Cfg),
	)
}

func registerDomains(mux *http.ServeMux, deps Dependencies) {
	onRequestError := func(w http.ResponseWriter, _ *http.Request, err error) {
		httperr.Write(w, deps.Log, httperr.BadRequest(err.Error()))
	}
	onResponseError := func(w http.ResponseWriter, _ *http.Request, err error) {
		httperr.Write(w, deps.Log, err)
	}

	entities.HandlerWithOptions(
		entities.NewStrictHandlerWithOptions(deps.Server, nil, entities.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  onRequestError,
			ResponseErrorHandlerFunc: onResponseError,
		}),
		entities.StdHTTPServerOptions{
			BaseURL: baseURL, BaseRouter: mux, ErrorHandlerFunc: onRequestError,
		})

	events.HandlerWithOptions(
		events.NewStrictHandlerWithOptions(deps.Server, nil, events.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  onRequestError,
			ResponseErrorHandlerFunc: onResponseError,
		}),
		events.StdHTTPServerOptions{
			BaseURL: baseURL, BaseRouter: mux, ErrorHandlerFunc: onRequestError,
		})

	graph.HandlerWithOptions(
		graph.NewStrictHandlerWithOptions(deps.Server, nil, graph.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  onRequestError,
			ResponseErrorHandlerFunc: onResponseError,
		}),
		graph.StdHTTPServerOptions{
			BaseURL: baseURL, BaseRouter: mux, ErrorHandlerFunc: onRequestError,
		})

	investigations.HandlerWithOptions(
		investigations.NewStrictHandlerWithOptions(deps.Server, nil, investigations.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  onRequestError,
			ResponseErrorHandlerFunc: onResponseError,
		}),
		investigations.StdHTTPServerOptions{
			BaseURL: baseURL, BaseRouter: mux, ErrorHandlerFunc: onRequestError,
		})

	reference.HandlerWithOptions(
		reference.NewStrictHandlerWithOptions(deps.Server, nil, reference.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  onRequestError,
			ResponseErrorHandlerFunc: onResponseError,
		}),
		reference.StdHTTPServerOptions{
			BaseURL: baseURL, BaseRouter: mux, ErrorHandlerFunc: onRequestError,
		})

	som.HandlerWithOptions(
		som.NewStrictHandlerWithOptions(deps.Server, nil, som.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  onRequestError,
			ResponseErrorHandlerFunc: onResponseError,
		}),
		som.StdHTTPServerOptions{
			BaseURL: baseURL, BaseRouter: mux, ErrorHandlerFunc: onRequestError,
		})

}

func liveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func ping(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("pong"))
}
