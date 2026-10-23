import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { createAdminSupportChannelsResource } from './resource'

describe('support/resource', () => {
  it('reads settings from Control Plane', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/support-channels', () =>
        HttpResponse.json({ data: { channels: [] } }),
      ),
    )
    expect(await createAdminSupportChannelsResource().get()).toEqual({ channels: [] })
  })

  it('writes settings with an idempotency key', async () => {
    let body: unknown
    let key: string | null = null
    useMswHandlers(
      http.put('/api/control/v1/platform/settings/support-channels', async ({ request }) => {
        body = await request.json()
        key = request.headers.get('Idempotency-Key')
        return HttpResponse.json({ data: body })
      }),
    )
    expect(await createAdminSupportChannelsResource().update({ channels: [] })).toEqual({
      channels: [],
    })
    expect(body).toEqual({ channels: [] })
    expect(key).toBeTruthy()
  })

  it('rejects malformed responses', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/support-channels', () =>
        HttpResponse.json({ data: {} }),
      ),
    )
    await expect(createAdminSupportChannelsResource().get()).rejects.toThrow('客户支持响应无效')
  })
})
