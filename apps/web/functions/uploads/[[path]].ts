import { proxyRequest } from '../_proxy'

export const onRequest = (context: { request: Request; env: { IDEABOARD_API_ORIGIN?: string } }) =>
  proxyRequest(context)
