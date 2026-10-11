import { beforeEach, describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { fetchVisionSearchPrice } from './visionBillingResource'

const WORKSPACE_ID = 'ws_abc123'

function quoteFixture(totalCredits: number): Record<string, unknown> {
  return {
    data: {
      sku: 'ai-search:vision',
      display_name: 'Vision search',
      price_version_id: 1,
      price_version: 1,
      output_resolution: '1k',
      unit_credits: totalCredits,
      quantity: 1,
      total_credits: totalCredits,
      quoted_at: '2026-09-23T00:00:00Z',
    },
  }
}

describe('visionBillingResource', () => {
  beforeEach(() => {
    localStorage.clear()
    window.history.pushState({}, '', '/user/search')
  })

  it('reads the server-side quote for one vision search', async () => {
    const seen: Array<{ path: string; body: unknown }> = []
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/billing-quotes', async ({ request }) => {
        seen.push({ path: new URL(request.url).pathname, body: await request.json() })
        return HttpResponse.json(quoteFixture(12))
      }),
    )
    const controller = new AbortController()

    await expect(fetchVisionSearchPrice(WORKSPACE_ID, controller.signal)).resolves.toBe(12)
    expect(seen).toHaveLength(1)
    expect(seen[0]?.path).toBe('/api/v1/workspaces/ws_abc123/billing-quotes')
    expect(seen[0]?.body).toEqual({ data: { sku: 'ai-search:vision' } })
  })

  it('fails closed without a workspace selection', async () => {
    await expect(fetchVisionSearchPrice('  ')).rejects.toThrow()
  })

  it('rejects quote envelopes without a numeric total', async () => {
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/billing-quotes', () => {
        return HttpResponse.json({ data: { sku: 'ai-search:vision' } })
      }),
    )
    await expect(fetchVisionSearchPrice(WORKSPACE_ID)).rejects.toThrow()
  })

  it('surfaces a 401 quote denial without reloading the page', async () => {
    localStorage.setItem(
      'user',
      JSON.stringify({ public_id: '00000000-0000-0000-0000-000000000001' }),
    )
    useMswHandlers(
      http.post('/api/v1/workspaces/:workspaceId/billing-quotes', () => {
        return HttpResponse.json({ code: 401, msg: '' }, { status: 401 })
      }),
    )

    await expect(fetchVisionSearchPrice(WORKSPACE_ID)).rejects.toThrow()

    expect(window.location.pathname).toBe('/user/search')
    expect(localStorage.getItem('user')).toBeNull()
  })
})
