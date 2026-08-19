import createClient from "openapi-fetch";

import type { paths } from "./api.d.ts";

/** Creates a typed Investigations API client. */
export function createIrClient(options: {
  baseUrl: string;
  projectId: string;
  token: () => string | null;
}) {
  // Match OpenAPI form+explode:false (e.g. statuses=proposed,confirmed).
  const client = createClient<paths>({
    baseUrl: options.baseUrl,
    querySerializer: { array: { explode: false, style: "form" } },
  });

  client.use({
    onRequest({ request }) {
      request.headers.set("X-Project-ID", options.projectId);

      const token = options.token();
      if (token) {
        request.headers.set("Authorization", `Bearer ${token}`);
      }
      return request;
    },
  });

  return client;
}

export type { paths, components } from "./api.d.ts";

//   import type { components } from "@ir/contract/domains/graph";
//   type GraphEdge = components["schemas"]["GraphEdge"];
