import createClient from "openapi-fetch";

import type { paths, components } from "./investigations.d.ts";

/**
 * Типизированный клиент Investigations API.
 *
 * Пути, параметры и тела запросов проверяются компилятором против OpenAPI:
 * опечатка в пути или лишнее поле в теле — ошибка сборки, а не 422 в рантайме.
 */
export function createIrClient(options: {
  baseUrl: string;
  /** Токен платформы. Функция, а не строка, — чтобы переживать refresh. */
  token: () => string | null;
}) {
  const client = createClient<paths>({ baseUrl: options.baseUrl });

  client.use({
    onRequest({ request }) {
      const token = options.token();
      if (token) {
        request.headers.set("Authorization", `Bearer ${token}`);
      }
      return request;
    },
  });

  return client;
}

/** Конверт ошибки: тело неуспешного ответа у всех ручек одинаковое. */
export type ApiError = components["schemas"]["ErrorResponse"]["error"];

export type Investigation = components["schemas"]["Investigation"];
export type Event = components["schemas"]["Event"];
export type Entity = components["schemas"]["Entity"];
export type Graph = components["schemas"]["Graph"];
export type GraphNode = components["schemas"]["GraphNode"];
export type Edge = components["schemas"]["Edge"];
export type Artifact = components["schemas"]["Artifact"];
export type Alert = components["schemas"]["Alert"];

export type { paths, components };
