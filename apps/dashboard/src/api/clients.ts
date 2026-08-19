import { createIrClient } from '@ir/contract'
import createClient from 'openapi-fetch'
import type { paths as GatewayPaths } from '@ir/contract/gateway'
import { env, getIrToken, irBaseUrl } from './env'

export const irClient = createIrClient({
  baseUrl: irBaseUrl(),
  projectId: env.projectId,
  token: getIrToken,
})

export const gatewayClient = createClient<GatewayPaths>({
  baseUrl: env.gatewayUrl,
})

gatewayClient.use({
  onRequest({ request }) {
    request.headers.set('X-Project-ID', env.projectId)
    const token = getIrToken()
    if (token) request.headers.set('Authorization', `Bearer ${token}`)
    return request
  },
})
