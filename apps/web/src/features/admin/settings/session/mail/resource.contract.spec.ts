import { describe, expect, it } from 'vitest'
import { HttpResponse, http, useMswHandlers } from '../../../../../test/msw'
import { createAdminMailConfigResource } from './resource'

const summaryPayload = {
  configured: true,
  api_key_configured: true,
  enabled: true,
  from_name: 'IdeaBoard',
  from_email: 'noreply@example.com',
  reply_to: 'support@example.com',
  config_version: 2,
  readiness: 'ready',
  updated_at: '2026-08-05T00:00:00Z',
  last_tested_at: '2026-08-05T00:01:00Z',
  last_test_error_code: null,
}

// Mail config boundary contract (#809 leftovers): the full flow (read,
// save, clear, test) goes through the Control Plane mail-setting
// operations with the contract envelope (`{ data }`). Mutations carry a
// header-only Idempotency-Key (mirrored into the
// body `reason` field). This spec pins the adapter half: control URLs,
// headers, bodies, and control error-code branches.
describe('mail/resource', () => {
  it('reads the summary through the control mail setting', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json({ data: summaryPayload }),
      ),
    )

    const summary = await createAdminMailConfigResource().get()

    expect(summary).toMatchObject({
      from_email: 'noreply@example.com',
      readiness: 'ready',
      config_version: 2,
    })
  })

  it('updates through control with idempotency headers', async () => {
    let seen: { method: string; body: unknown; headers: Record<string, string> } | undefined
    useMswHandlers(
      http.put('/api/control/v1/platform/settings/mail', async ({ request }) => {
        seen = {
          method: request.method,
          body: await request.json(),
          headers: {
            idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
          },
        }
        return HttpResponse.json({ data: { ...summaryPayload, readiness: 'not_tested' } })
      }),
    )

    const updated = await createAdminMailConfigResource().update({
      from_name: 'IdeaBoard Ops',
      from_email: 'noreply@example.com',
    })

    expect(seen?.method).toBe('PUT')
    expect(seen?.body).toEqual({
      from_name: 'IdeaBoard Ops',
      from_email: 'noreply@example.com',
    })
    expect(seen?.headers.idempotencyKey).toMatch(/.+/)
    expect(updated.readiness).toBe('not_tested')
  })

  it('passes enabled through on update and reads it back on get', async () => {
    let seenBody: unknown
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json({ data: summaryPayload }),
      ),
      http.put('/api/control/v1/platform/settings/mail', async ({ request }) => {
        seenBody = await request.json()
        return HttpResponse.json({ data: { ...summaryPayload, enabled: false } })
      }),
    )

    const resource = createAdminMailConfigResource()
    const summary = await resource.get()
    expect(summary.enabled).toBe(true)

    const updated = await resource.update({
      from_name: 'IdeaBoard',
      from_email: 'noreply@example.com',
      enabled: false,
    })

    expect(seenBody).toEqual({
      from_name: 'IdeaBoard',
      from_email: 'noreply@example.com',
      enabled: false,
    })
    expect(updated.enabled).toBe(false)
  })

  it('mints a fresh idempotency key per attempt', async () => {
    const keys: Array<string | null> = []
    useMswHandlers(
      http.put('/api/control/v1/platform/settings/mail', ({ request }) => {
        keys.push(request.headers.get('Idempotency-Key'))
        return HttpResponse.json({ data: summaryPayload })
      }),
    )

    const resource = createAdminMailConfigResource()
    await resource.update({ from_name: 'IdeaBoard', from_email: 'noreply@example.com' })
    await resource.update({ from_name: 'IdeaBoard', from_email: 'noreply@example.com' })

    expect(keys).toHaveLength(2)
    expect(keys[0]).toMatch(/.+/)
    expect(keys[0]).not.toBe(keys[1])
  })

  it('clears through control delete', async () => {
    let seen: { method: string } | undefined
    useMswHandlers(
      http.delete('/api/control/v1/platform/settings/mail', ({ request }) => {
        seen = {
          method: request.method,
        }
        return HttpResponse.json({ data: { deleted: true } })
      }),
    )

    await createAdminMailConfigResource().clear()

    expect(seen?.method).toBe('DELETE')
  })

  it('tests through control', async () => {
    let seen: { method: string; body: unknown; idempotencyKey: string } | undefined
    useMswHandlers(
      http.post('/api/control/v1/platform/settings/mail/test', async ({ request }) => {
        seen = {
          method: request.method,
          body: await request.json(),
          idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
        }
        return HttpResponse.json({ data: { sent: true } })
      }),
    )

    await createAdminMailConfigResource().test('ops@example.com')

    expect(seen?.method).toBe('POST')
    expect(seen?.body).toEqual({ recipient: 'ops@example.com' })
    // Mail probes use a header-only Idempotency-Key: the same request
    // replays without resending; a different body conflicts server-side.
    expect(seen?.idempotencyKey).toMatch(/.+/)
  })

  it('sends update/test mutations with idempotency headers', async () => {
    let seenUpdate: { body: unknown; headers: Record<string, string> } | undefined
    let seenTest: { body: unknown; headers: Record<string, string> } | undefined
    useMswHandlers(
      http.put('/api/control/v1/platform/settings/mail', async ({ request }) => {
        seenUpdate = {
          body: await request.json(),
          headers: {
            idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
          },
        }
        return HttpResponse.json({ data: summaryPayload })
      }),
      http.post('/api/control/v1/platform/settings/mail/test', async ({ request }) => {
        seenTest = {
          body: await request.json(),
          headers: {
            idempotencyKey: request.headers.get('Idempotency-Key') ?? '',
          },
        }
        return HttpResponse.json({ data: { sent: true } })
      }),
    )
    const resource = createAdminMailConfigResource()

    await resource.update({ from_name: 'IdeaBoard', from_email: 'noreply@example.com' })
    expect(seenUpdate?.body).toEqual({
      from_name: 'IdeaBoard',
      from_email: 'noreply@example.com',
    })
    expect(seenUpdate?.headers.idempotencyKey).toMatch(/.+/)

    await resource.test('ops@example.com')
    expect(seenTest?.body).toEqual({ recipient: 'ops@example.com' })
    expect(seenTest?.headers.idempotencyKey).toMatch(/.+/)
  })

  it('maps read 403/404/503 to operator copy', async () => {
    const resource = createAdminMailConfigResource()

    useMswHandlers(
      http.get('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json(
          { error: { code: 'forbidden', message: 'denied', request_id: 'r' } },
          { status: 403 },
        ),
      ),
    )
    await expect(resource.get()).rejects.toThrow('权限不足，无法操作邮件配置')

    useMswHandlers(
      http.get('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json(
          { error: { code: 'control_not_found', message: 'missing', request_id: 'r' } },
          { status: 404 },
        ),
      ),
    )
    await expect(resource.get()).rejects.toThrow('邮件配置不存在')

    useMswHandlers(
      http.get('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json(
          { error: { code: 'service_unavailable', message: 'down', request_id: 'r' } },
          { status: 503 },
        ),
      ),
    )
    await expect(resource.get()).rejects.toThrow('邮件服务暂不可用，请稍后重试')
  })

  it('maps control error codes to operator copy', async () => {
    useMswHandlers(
      http.put('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json(
          { error: { code: 'control_not_found', message: 'missing', request_id: 'r' } },
          { status: 400 },
        ),
      ),
      http.delete('/api/control/v1/platform/settings/mail', () =>
        HttpResponse.json(
          { error: { code: 'idempotency_conflict', message: 'reused', request_id: 'r' } },
          { status: 409 },
        ),
      ),
    )

    const resource = createAdminMailConfigResource()
    await expect(
      resource.update({ from_name: 'IdeaBoard', from_email: 'noreply@example.com' }),
    ).rejects.toThrow('邮件配置不存在')
    await expect(resource.clear()).rejects.toThrow('操作已提交，请勿重复操作')
  })
})
