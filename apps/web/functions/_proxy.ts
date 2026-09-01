interface ProxyEnv {
  IDEABOARD_API_ORIGIN?: string
}

interface PagesContext {
  request: Request
  env: ProxyEnv
}

const DEFAULT_API_ORIGIN = 'https://api.ideaboard.art'

function apiOrigin(env: ProxyEnv): URL {
  const configured = (env.IDEABOARD_API_ORIGIN || DEFAULT_API_ORIGIN).trim()
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
  return value.startsWith(origin.origin) ? `${publicOrigin}${value.slice(origin.origin.length)}` : value
}

export async function proxyRequest({ request, env }: PagesContext): Promise<Response> {
  const origin = apiOrigin(env)
  const incoming = new URL(request.url)
  const target = new URL(`${origin.origin}${incoming.pathname}${incoming.search}`)
  const headers = new Headers(request.headers)
  headers.delete('host')
  headers.set('x-forwarded-host', incoming.host)
  headers.set('x-forwarded-proto', incoming.protocol.slice(0, -1))

  const response = await fetch(target, {
    method: request.method,
    headers,
    body: request.method === 'GET' || request.method === 'HEAD' ? undefined : request.body,
    redirect: 'manual',
  })

  const responseHeaders = new Headers(response.headers)
  const publicOrigin = incoming.origin
  const location = responseHeaders.get('location')
  if (location) responseHeaders.set('location', rewriteLocation(location, origin, publicOrigin))

  const getSetCookie = (
    response.headers as Headers & { getSetCookie?: () => string[] }
  ).getSetCookie
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
