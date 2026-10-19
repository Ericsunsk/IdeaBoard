// @vitest-environment node
import { describe, expect, test } from 'vitest'
import config from '../../vite.config'

// The browser dials presence on same-origin `/realtime`
// (`resolvePresenceServerUrl`), so the Vite dev proxy must forward it to
// the local sidecar. Without this entry every local-dev room silently stays
// local-only: the roster renders the self avatar but never any collaborator.
describe('vite dev proxy', () => {
  test('forwards the realtime upgrade to the local presence sidecar', () => {
    const proxy = config.server?.proxy as Record<string, Record<string, unknown>> | undefined
    expect(proxy?.['/realtime']).toMatchObject({
      target: 'http://localhost:6002',
      changeOrigin: true,
      ws: true,
    })
  })

  test('keeps the api upstreams on their local ports', () => {
    const proxy = config.server?.proxy as Record<string, Record<string, unknown>> | undefined
    expect(proxy?.['/api/auth']).toMatchObject({ target: 'http://localhost:6001' })
    expect(proxy?.['/api']).toMatchObject({ target: 'http://localhost:6000' })
  })
})
