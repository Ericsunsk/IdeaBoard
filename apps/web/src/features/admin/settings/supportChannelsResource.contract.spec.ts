import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { createAdminSupportChannelsResource } from './supportChannelsResource'

describe('supportChannelsResource', () => {
  it('reads settings from Control Plane', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/support-channels', () =>
        HttpResponse.json({ data: { channels: [] } }),
      ),
    )
    expect(await createAdminSupportChannelsResource().get()).toEqual({ channels: [] })
  })

  it('writes settings with audit and idempotency headers', async () => {
    let body: unknown
    let reason: string | null = null
    let key: string | null = null
    useMswHandlers(
      http.put('/api/control/v1/platform/settings/support-channels', async ({ request }) => {
        body = await request.json()
        reason = request.headers.get('X-Operator-Reason')
        key = request.headers.get('Idempotency-Key')
        return HttpResponse.json({ data: body })
      }),
    )
    expect(
      await createAdminSupportChannelsResource().update({ channels: [] }, ' update channels '),
    ).toEqual({ channels: [] })
    expect(body).toEqual({ channels: [] })
    expect(reason).toBe('update channels')
    expect(key).toBeTruthy()
  })

  it('refuses unaudited writes and malformed responses', async () => {
    await expect(
      createAdminSupportChannelsResource().update({ channels: [] }, ' '),
    ).rejects.toThrow('需要填写操作原因')
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/support-channels', () =>
        HttpResponse.json({ data: {} }),
      ),
    )
    await expect(createAdminSupportChannelsResource().get()).rejects.toThrow('客服渠道响应无效')
  })
})
