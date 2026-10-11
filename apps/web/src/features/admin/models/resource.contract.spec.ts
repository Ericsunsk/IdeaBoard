import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { createAdminGatewayResource } from './resource'

const reason = { operatorReason: 'gateway review' }

describe('Admin gateway resource', () => {
  it('saves catalogs through control without legacy price fields', async () => {
    let requestBody: unknown
    useMswHandlers(
      http.put('/api/control/v1/platform/ai-gateway/catalog', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({ data: {} })
      }),
    )

    await createAdminGatewayResource().catalog.save(
      [
        {
          id: 'openai/gpt-4o',
          name: 'GPT-4o',
          provider: 'OpenAI',
          description: '',
          contextOrResolution: '128K',
          capabilities: [],
          hosting: '第三方',
          termsUrl: '',
          docsUrl: '',
          supportsPromptCaching: true,
        },
      ],
      reason,
    )

    const body = requestBody as { items: Array<Record<string, unknown>> }
    expect(body.items).toHaveLength(1)
    expect(body.items[0]).toMatchObject({
      id: 'openai/gpt-4o',
      supports_prompt_caching: true,
    })
    expect(body.items[0]).not.toHaveProperty('input_cost_per_token')
    expect(body.items[0]).not.toHaveProperty('output_cost_per_token')
    expect(body.items[0]).not.toHaveProperty('cache_read_input_token_cost')
    expect(body.items[0]).not.toHaveProperty('cache_creation_input_token_cost')
  })

  it('reads pricing reference snapshots through control', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/ai-gateway/models-reference', () =>
        HttpResponse.json({
          data: {
            model_id: 'openai/gpt-4o',
            provider: 'openai',
            input_usd_per_mtok: 2.5,
            output_usd_per_mtok: 10,
            context_limit: 128000,
            source: 'models.dev',
            note: 'models.dev 参照价',
            found: true,
          },
        }),
      ),
    )

    const snapshot = await createAdminGatewayResource().catalog.getReference({
      modelId: 'openai/gpt-4o',
    })
    expect(snapshot).toMatchObject({
      modelId: 'openai/gpt-4o',
      inputUsdPerMtok: 2.5,
      contextLimit: 128000,
      source: 'models.dev',
      found: true,
    })
  })

  it('reads observed snapshots through control', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/ai-gateway/models-observed', () =>
        HttpResponse.json({
          data: [
            {
              model: 'openai/gpt-4o',
              provider: 'openai',
              window_days: 7,
              window_since: '2026-09-16T00:00:00Z',
              attempts_total: 10,
              completed: 9,
              failed: 1,
              cache_hits: 2,
              success_rate: 0.9,
              cache_hit_rate: 0.2,
              has_observations: true,
              has_sku_margin: false,
              sku_margins: [],
            },
          ],
        }),
      ),
    )

    const rows = await createAdminGatewayResource().catalog.listObserved()
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({
      model: 'openai/gpt-4o',
      windowDays: 7,
      attemptsTotal: 10,
      successRate: 0.9,
      hasObservations: true,
    })
  })

  it('previews routes, detects models, and probes connections through control', async () => {
    const seen: Array<{ path: string; body: unknown; reason: string | null }> = []
    useMswHandlers(
      http.post('/api/control/v1/platform/ai-gateway/route-preview', async ({ request }) => {
        seen.push({
          path: 'route-preview',
          body: await request.json(),
          reason: request.headers.get('X-Operator-Reason'),
        })
        return HttpResponse.json({
          data: {
            provider_slug: 'openai',
            is_first_party: false,
            target_path: '/v1/chat/completions',
            path_source: 'default',
            path_source_label: '默认路由',
          },
        })
      }),
      http.get('/api/control/v1/platform/ai-gateway/detect-models', () =>
        HttpResponse.json({
          data: {
            models: [
              {
                id: 'openai/gpt-4o',
                name: 'GPT-4o',
                provider: 'openai',
                description: '旗舰模型',
                description_enriched: true,
                context_or_resolution: '128K',
              },
            ],
            configured_providers: 2,
          },
        }),
      ),
      http.post('/api/control/v1/platform/ai-gateway/test-connection', async ({ request }) => {
        seen.push({
          path: 'test-connection',
          body: await request.json(),
          reason: request.headers.get('X-Operator-Reason'),
        })
        return HttpResponse.json({
          data: { reachable: true, status_code: 200, latency_ms: 38, message: 'ok' },
        })
      }),
    )
    const resource = createAdminGatewayResource()

    await expect(resource.catalog.previewRoute({ modelId: 'm' })).resolves.toMatchObject({
      provider_slug: 'openai',
      target_path: '/v1/chat/completions',
    })
    expect(seen[0]?.body).toEqual({ model_id: 'm' })

    await expect(resource.catalog.detect()).resolves.toMatchObject({
      models: [{ id: 'openai/gpt-4o', description: '旗舰模型' }],
      configured_providers: 2,
    })

    await expect(
      resource.connection.testConnection(
        { base_url: 'https://gateway.example.com', provider: 'openai', path: 'v1/models' },
        reason,
      ),
    ).resolves.toMatchObject({ success: true, latencyMs: 38, message: 'ok' })
    expect(seen[1]?.body).toMatchObject({
      base_url: 'https://gateway.example.com',
      provider: 'openai',
      path: 'v1/models',
    })
    expect(seen[1]?.reason).toBe('gateway review')
  })

  it('rejects gateway config updates when the envelope reports an error', async () => {
    useMswHandlers(
      http.put('/api/control/v1/platform/ai-gateway', () =>
        HttpResponse.json(
          {
            error: {
              code: 'bad_request',
              message: '网关配置校验失败',
              request_id: 'req-gateway-400',
            },
          },
          { status: 400 },
        ),
      ),
    )

    await expect(
      createAdminGatewayResource().connection.update(
        { base_url: 'https://gateway.example.com' },
        reason,
      ),
    ).rejects.toThrow()
  })

  it('resolves gateway config updates when the envelope reports success', async () => {
    useMswHandlers(
      http.put('/api/control/v1/platform/ai-gateway', () => HttpResponse.json({ data: {} })),
    )

    await expect(
      createAdminGatewayResource().connection.update(
        { base_url: 'https://gateway.example.com' },
        reason,
      ),
    ).resolves.toBeUndefined()
  })

  it('requires an operator reason before gateway writes', async () => {
    await expect(
      createAdminGatewayResource().connection.testConnection(
        { base_url: 'https://gateway.example.com', provider: 'openai', path: 'v1/models' },
        { operatorReason: '  ' },
      ),
    ).rejects.toMatchObject({ code: 'control_reason_required' })
  })
})
