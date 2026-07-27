import createClient from "openapi-fetch";

import type { paths } from "./paths.gen.ts";

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

export type { paths };

// Типы доменов импортируются напрямую из ./domains/<домен>: раскладка та же,
// что в api/paths, поэтому искать не приходится.
//
//   import type { components } from "@ir/contract/domains/graph";
//   type Edge = components["schemas"]["Edge"];
