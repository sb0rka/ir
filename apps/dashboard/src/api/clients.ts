import { createIrClient } from '@ir/contract'
import createClient from 'openapi-fetch'
import type { paths as GatewayPaths } from '@ir/contract/gateway'
import { authorizedFetch, getAccessToken } from './auth'
import { env, getProjectId, irBaseUrl } from './env'

export const irClient = createIrClient({
  baseUrl: irBaseUrl(),
  projectId: getProjectId,
  token: getAccessToken,
  fetch: authorizedFetch,
})

export const gatewayClient = createClient<GatewayPaths>({
  baseUrl: env.gatewayUrl,
  fetch: authorizedFetch,
})

gatewayClient.use({
  onRequest({ request }) {
    const projectId = getProjectId()
    if (projectId && !request.headers.has('X-Project-ID')) {
      request.headers.set('X-Project-ID', projectId)
    }
    const token = getAccessToken()
    if (token) request.headers.set('Authorization', `Bearer ${token}`)
    return request
  },
})
