// @vitest-environment node
import { afterEach, describe, expect, it, vi } from 'vitest'
import { onRequest as realtimeRequest } from '../../functions/realtime'
import { onRequest as apiRequest } from '../../functions/api/[[path]]'

const env = { IDEABOARD_API_ORIGIN: 'https://api.ideaboard.test' }

afterEach(() => vi.unstubAllGlobals())

describe('Pages same-origin proxy', () => {
  it('forwards the Realtime upgrade and ticket to the configured origin without replacing its socket', async () => {
    // Node cannot construct status 101; model the Cloudflare fetch response
    // with a socket binding and require the handler to return that exact object.
    const upstream = Object.defineProperties(new Response(null), {
      status: { value: 101 },
      webSocket: { value: { readyState: 1 } },
    })
    const fetchUpstream = vi.fn<typeof fetch>().mockResolvedValue(upstream)
    vi.stubGlobal('fetch', fetchUpstream)

    const response = await realtimeRequest({
      env,
      request: new Request('https://ideaboard.test/realtime?ticket=signed-ticket&room=board-1', {
        headers: {
          Upgrade: 'websocket',
          Connection: 'Upgrade',
          'Sec-WebSocket-Protocol': 'presence',
          Origin: 'https://ideaboard.test',
          Cookie: 'session=browser-session',
        },
      }),
    })

    expect(response).toBe(upstream)
    expect(fetchUpstream).toHaveBeenCalledTimes(1)
    const [target, options] = fetchUpstream.mock.calls[0]!
    expect(String(target)).toBe(
      'https://api.ideaboard.test/realtime?ticket=signed-ticket&room=board-1',
    )
    expect(options).toMatchObject({ method: 'GET', redirect: 'manual' })
    const headers = new Headers(options?.headers)
    expect(headers.get('upgrade')).toBe('websocket')
    expect(headers.get('connection')).toBe('Upgrade')
    expect(headers.get('sec-websocket-protocol')).toBe('presence')
    expect(headers.get('cookie')).toBe('session=browser-session')
    expect(headers.get('origin')).toBe(env.IDEABOARD_API_ORIGIN)
    expect(headers.get('x-forwarded-host')).toBe('ideaboard.test')
    expect(headers.get('x-forwarded-proto')).toBe('https')
  })

  it.each([401, 426])(
    'preserves upstream Realtime refusal %i without manufacturing an upgrade',
    async (status) => {
      vi.stubGlobal(
        'fetch',
        vi.fn<typeof fetch>().mockResolvedValue(
          new Response('ticket or upgrade required', {
            status,
            headers: { 'Content-Type': 'text/plain', 'Cache-Control': 'no-store' },
          }),
        ),
      )

      const response = await realtimeRequest({
        env,
        request: new Request('https://ideaboard.test/realtime'),
      })

      expect(response.status).toBe(status)
      expect(await response.text()).toBe('ticket or upgrade required')
      expect(response.headers.get('cache-control')).toBe('no-store')
      expect(response.headers.has('upgrade')).toBe(false)
    },
  )

  it('retains the auth origin and existing same-origin cookie and redirect rewrites', async () => {
    const upstreamHeaders = new Headers({
      Location: 'https://api.ideaboard.test/api/auth/callback',
    })
    upstreamHeaders.append('Set-Cookie', 'session=one; Domain=api.ideaboard.test; Path=/; Secure')
    upstreamHeaders.append('Set-Cookie', 'state=two; Domain=api.ideaboard.test; Path=/; HttpOnly')
    const fetchUpstream = vi
      .fn<typeof fetch>()
      .mockResolvedValue(new Response(null, { status: 302, headers: upstreamHeaders }))
    vi.stubGlobal('fetch', fetchUpstream)

    const response = await apiRequest({
      env,
      request: new Request('https://ideaboard.test/api/auth/sign-in', {
        headers: { Origin: 'https://ideaboard.test' },
      }),
    })

    expect(new Headers(fetchUpstream.mock.calls[0]![1]?.headers).get('origin')).toBe(
      'https://ideaboard.test',
    )
    expect(response.status).toBe(302)
    expect(response.headers.get('location')).toBe('https://ideaboard.test/api/auth/callback')
    expect(response.headers.getSetCookie()).toEqual([
      'session=one; Path=/; Secure',
      'state=two; Path=/; HttpOnly',
    ])
  })
})
