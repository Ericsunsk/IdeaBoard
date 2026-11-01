import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { createAdminGatewayResource } from './resource'

const options = {}

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
      options,
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

  it.each([
    ['openai', 'gpt-image-1'],
    ['fal', 'fal-ai/flux-pro'],
  ])('previews %s nested names with the explicit provider', async (provider, modelId) => {
    let payload: unknown
    useMswHandlers(
      http.post('/api/control/v1/platform/ai-gateway/route-preview', async ({ request }) => {
        payload = await request.json()
        return HttpResponse.json({
          // eslint-disable-next-line governance/boolean-prefix -- Wire contract field.
          data: { provider, valid: true },
        })
      }),
    )
    await createAdminGatewayResource().catalog.previewRoute({ provider, modelId })
    expect(payload).toEqual({ model_id: `${provider}/${modelId}` })
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

  it('previews routes and lists configured providers through control', async () => {
    const seen: Array<{ path: string; body: unknown }> = []
    useMswHandlers(
      http.post('/api/control/v1/platform/ai-gateway/route-preview', async ({ request }) => {
        seen.push({
          path: 'route-preview',
          body: await request.json(),
        })
        return HttpResponse.json({
          data: {
            provider: 'openai',
            model: 'openai/gpt-image-1',
            // eslint-disable-next-line governance/boolean-prefix -- Wire contract field.
            valid: true,
            reachable: true,
            status_code: 200,
            latency_ms: 38,
            message: 'Worker 干跑正常 (38ms, HTTP 200)',
          },
        })
      }),
      http.get('/api/control/v1/platform/ai-gateway/providers', () =>
        HttpResponse.json({
          data: [
            { slug: 'openai', display_name: 'OpenAI', id_prefix: 'openai/' },
            { slug: 'workers-ai', display_name: 'Workers AI', id_prefix: 'workers-ai/' },
          ],
        }),
      ),
    )
    const resource = createAdminGatewayResource()

    await expect(resource.catalog.previewRoute({ modelId: 'm' })).resolves.toMatchObject({
      provider: 'openai',
      // eslint-disable-next-line governance/boolean-prefix -- Wire contract field.
      valid: true,
    })
    expect(seen[0]?.body).toEqual({ model_id: 'm' })

    // 裸名在此拼接 provider（仅传输）：后端仍消费组合 model_id。
    await expect(
      resource.catalog.previewRoute({ modelId: 'gpt-image-1', provider: 'openai' }),
    ).resolves.toMatchObject({
      provider: 'openai',
      // eslint-disable-next-line governance/boolean-prefix -- Wire contract field.
      valid: true,
    })
    expect(seen[1]?.body).toEqual({ model_id: 'openai/gpt-image-1' })

    await expect(resource.catalog.listProviders()).resolves.toMatchObject([
      { slug: 'openai', display_name: 'OpenAI' },
      { slug: 'workers-ai', display_name: 'Workers AI' },
    ])
  })

  it('passes custom domain update payload with explicit identity through', async () => {
    // 自定义域名透传显式身份。
    let requestBody: unknown
    useMswHandlers(
      http.put('/api/control/v1/platform/ai-gateway', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({ data: {} })
      }),
    )

    await expect(
      createAdminGatewayResource().connection.update(
        {
          base_url: 'https://gateway.example.com',
          token: 'tok',
          account_id: '3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c',
          gateway_id: 'my-gateway',
        },
        options,
      ),
    ).resolves.toBeUndefined()
    expect(requestBody).toMatchObject({
      base_url: 'https://gateway.example.com',
      token: 'tok',
      account_id: '3bf89a1c2d3e4f5a6b7c8d9e0f1a2b3c',
      gateway_id: 'my-gateway',
    })
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
        options,
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
        options,
      ),
    ).resolves.toBeUndefined()
  })
})
