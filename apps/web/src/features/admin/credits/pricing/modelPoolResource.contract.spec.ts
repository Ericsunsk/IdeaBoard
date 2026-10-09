import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import {
  createAdminModelPoolResource,
  fetchAdminModelPool,
  replaceAdminModelPool,
} from './modelPoolResource'

const reason = { operatorReason: 'pool review' }
const sku = 'ai-image:generation'

function poolHandlers(state: {
  version: number
  entries: unknown[]
  putStatus?: number
  putBody?: unknown
  seen?: { body?: unknown; idempotencyKey?: string | null; operatorReason?: string | null }
}) {
  return [
    http.get('/api/control/v1/billing/catalog/:sku/pool', () => {
      return HttpResponse.json({
        data: { service: 'generation', sku, version: state.version, entries: state.entries },
      })
    }),
    http.put('/api/control/v1/billing/catalog/:sku/pool', async ({ request }) => {
      state.seen = state.seen ?? {}
      state.seen.body = await request.json()
      state.seen.idempotencyKey = request.headers.get('Idempotency-Key')
      state.seen.operatorReason = request.headers.get('X-Operator-Reason')
      if (state.putStatus && state.putStatus !== 200) {
        return HttpResponse.json(state.putBody ?? {}, { status: state.putStatus })
      }
      return HttpResponse.json({ data: state.putBody })
    }),
  ]
}

describe('Admin Model Pool resource', () => {
  it('gets the complete pool with its version', async () => {
    const state = {
      version: 2,
      entries: [
        { model_id: 'a/first', enabled: true, resolutions: ['1k', '2k'] },
        { model_id: 'b/second', enabled: false, resolutions: [] },
      ],
    }
    useMswHandlers(...poolHandlers(state))

    await expect(fetchAdminModelPool(sku)).resolves.toEqual({
      service: 'generation',
      sku,
      version: 2,
      entries: [
        { modelId: 'a/first', enabled: true, resolutions: ['1k', '2k'] },
        { modelId: 'b/second', enabled: false, resolutions: [] },
      ],
    })
  })

  it('replaces the complete pool with version, entries, and operator reason', async () => {
    const state = {
      version: 2,
      entries: [],
      putBody: {
        service: 'generation',
        sku,
        version: 3,
        changed: true,
        entries: [{ model_id: 'a/first', enabled: true, resolutions: ['1k', '2k'] }],
      },
      seen: {} as {
        body?: unknown
        idempotencyKey?: string | null
        operatorReason?: string | null
      },
    }
    useMswHandlers(...poolHandlers(state))

    await expect(
      replaceAdminModelPool(
        sku,
        { version: 2, entries: [{ modelId: 'a/first', enabled: true, resolutions: ['1k', '2k'] }] },
        reason,
      ),
    ).resolves.toEqual({
      service: 'generation',
      sku,
      version: 3,
      changed: true,
      entries: [{ modelId: 'a/first', enabled: true, resolutions: ['1k', '2k'] }],
    })
    expect(state.seen.body).toEqual({
      expected_version: 2,
      entries: [{ model_id: 'a/first', enabled: true, resolutions: ['1k', '2k'] }],
      reason: 'pool review',
    })
    expect(state.seen.idempotencyKey).toMatch(/.+/)
    expect(state.seen.operatorReason).toBe('pool review')
  })

  it('maps a version conflict to the stale-pool copy', async () => {
    const state = {
      version: 3,
      entries: [],
      putStatus: 409,
      putBody: { error: { code: 'conflict', message: 'stale', request_id: 'req-1' } },
      seen: {} as {
        body?: unknown
        idempotencyKey?: string | null
        operatorReason?: string | null
      },
    }
    useMswHandlers(...poolHandlers(state))

    const failure = await replaceAdminModelPool(sku, { version: 2, entries: [] }, reason).catch(
      (error: unknown) => error,
    )
    expect(failure).toBeInstanceOf(Error)
    expect((failure as Error & { code?: unknown }).code).toBe('conflict')
    expect((failure as Error).message).toBe('模型池已被更新，请刷新后重试')
  })

  it('replaces the pool without an operator reason while keeping the idempotency key', async () => {
    const state = {
      version: 2,
      entries: [],
      putBody: {
        service: 'generation',
        sku,
        version: 3,
        changed: true,
        entries: [],
      },
      seen: {} as {
        body?: unknown
        idempotencyKey?: string | null
        operatorReason?: string | null
      },
    }
    useMswHandlers(...poolHandlers(state))

    await expect(
      replaceAdminModelPool(sku, { version: 2, entries: [] }, { operatorReason: '  ' }),
    ).resolves.toMatchObject({ version: 3, changed: true })
    expect(state.seen.body).toEqual({ expected_version: 2, entries: [], reason: '' })
    expect(state.seen.idempotencyKey).toMatch(/.+/)
    expect(state.seen.operatorReason).toBeNull()
  })

  it('lists catalog models through the injected lister', async () => {
    const resource = createAdminModelPoolResource(async () => [{ id: 'a/first', name: 'First' }])
    await expect(resource.listCatalogModels()).resolves.toEqual([{ id: 'a/first', name: 'First' }])
  })
})
