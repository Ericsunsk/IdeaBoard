interface ProxyEnv {
  IDEABOARD_API_ORIGIN: string
}

interface PagesContext {
  request: Request
  env: ProxyEnv
}

function apiOrigin(env: ProxyEnv): URL {
  const configured = env.IDEABOARD_API_ORIGIN?.trim()
  if (!configured) throw new Error('IDEABOARD_API_ORIGIN is required')
  const origin = new URL(configured)
  if (origin.protocol !== 'https:') throw new Error('IDEABOARD_API_ORIGIN must use HTTPS')
  origin.pathname = ''
  origin.search = ''
  origin.hash = ''
  return origin
}

function rewriteSetCookie(value: string): string {
  return value.replace(/;\s*domain=[^;]*/gi, '')
}

function rewriteLocation(value: string, origin: URL, publicOrigin: string): string {
  return value.startsWith(origin.origin)
    ? `${publicOrigin}${value.slice(origin.origin.length)}`
    : value
}

export async function proxyRequest({ request, env }: PagesContext): Promise<Response> {
  const origin = apiOrigin(env)
  const incoming = new URL(request.url)
  const target = new URL(`${origin.origin}${incoming.pathname}${incoming.search}`)
  const headers = new Headers(request.headers)
  // Better Auth validates the browser Origin against the canonical PUBLIC_ORIGIN,
  // so auth requests must retain the public origin while ordinary API routes
  // use the upstream origin for the API CORS policy.
  const isAuthRequest =
    incoming.pathname === '/api/auth' || incoming.pathname.startsWith('/api/auth/')
  if (!isAuthRequest && headers.get('origin') === incoming.origin) {
    headers.set('origin', origin.origin)
  }
  headers.set('x-forwarded-host', incoming.host)
  headers.set('x-forwarded-proto', incoming.protocol.slice(0, -1))

  const response = await fetch(target, {
    method: request.method,
    headers,
    body: request.method === 'GET' || request.method === 'HEAD' ? null : request.body,
    redirect: 'manual',
  })

  // Cloudflare binds the upgraded socket to this response. Reconstructing it
  // would discard the WebSocket connection.
  if (response.status === 101) return response

  const responseHeaders = new Headers(response.headers)
  const publicOrigin = incoming.origin
  const location = responseHeaders.get('location')
  if (location) responseHeaders.set('location', rewriteLocation(location, origin, publicOrigin))

  const getSetCookie = (response.headers as Headers & { getSetCookie?: () => string[] })
    .getSetCookie
  const cookies = getSetCookie ? getSetCookie.call(response.headers) : []
  if (cookies.length > 0) {
    responseHeaders.delete('set-cookie')
    for (const cookie of cookies) responseHeaders.append('set-cookie', rewriteSetCookie(cookie))
  }

  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers: responseHeaders,
  })
}
