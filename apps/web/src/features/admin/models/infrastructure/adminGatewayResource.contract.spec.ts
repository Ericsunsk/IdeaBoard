import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { createAdminGatewayResource } from './adminGatewayResource'

describe('Admin gateway resource', () => {
  it('lists per-feature model mappings', async () => {
    useMswHandlers(
      http.get('/api/control/v1/platform/ai-gateway/models', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: [{ feature: 'generation', model: 'openai/gpt-image-1' }],
        }),
      ),
    )

    await expect(createAdminGatewayResource().mappings.list()).resolves.toEqual([
      { feature: 'generation', model: 'openai/gpt-image-1' },
    ])
  })

  it('saves one feature mapping without touching pricing', async () => {
    let requestBody: unknown
    let requestPath = ''
    useMswHandlers(
      http.put('/api/admin/settings/ai-gateway/models/:feature', async ({ params, request }) => {
        requestPath = `/api/admin/settings/ai-gateway/models/${params.feature}`
        requestBody = await request.json()
        return HttpResponse.json({ code: 0, msg: 'ok', data: null })
      }),
    )

    await createAdminGatewayResource().mappings.save('generation', 'openai/gpt-image-1')

    expect(requestPath).toBe('/api/admin/settings/ai-gateway/models/generation')
    expect(requestBody).toEqual({ model: 'openai/gpt-image-1' })
  })

  it('saves catalogs without legacy price fields', async () => {
    let requestBody: unknown
    useMswHandlers(
      http.put('/api/admin/settings/ai-gateway/catalog', async ({ request }) => {
        requestBody = await request.json()
        return HttpResponse.json({ code: 0, msg: 'ok', data: null })
      }),
    )

    await createAdminGatewayResource().catalog.save([
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
    ])

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

  it('fetches models.dev reference snapshots through the backend', async () => {
    let requestUrl = ''
    useMswHandlers(
      http.get('/api/admin/settings/ai-gateway/models-reference', ({ request }) => {
        requestUrl = request.url
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            model_id: 'openai/gpt-4o',
            input_usd_per_mtok: 2.5,
            output_usd_per_mtok: 10,
            context_limit: 128000,
            last_updated: '2026-01-01',
            source: 'models.dev',
            note: '参照价，非结算依据',
            found: true,
          },
        })
      }),
    )

    const snapshot = await createAdminGatewayResource().catalog.getReference({
      modelId: 'openai/gpt-4o',
    })

    expect(snapshot).toMatchObject({
      modelId: 'openai/gpt-4o',
      inputUsdPerMtok: 2.5,
      outputUsdPerMtok: 10,
      contextLimit: 128000,
      lastUpdated: '2026-01-01',
      source: 'models.dev',
      note: '参照价，非结算依据',
      found: true,
    })
    expect(requestUrl).toContain('model_id=')
  })

  it('maps models.dev enrichment fields into camelCase snapshots', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/ai-gateway/models-reference', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            model_id: 'openai/gpt-4.1-nano',
            input_usd_per_mtok: 0.1,
            output_usd_per_mtok: 0.4,
            context_limit: 1047576,
            last_updated: '2025-04-14',
            reasoning: false,
            tool_call: true,
            temperature: true,
            attachment: true,
            structured_output: true,
            open_weights: false,
            modalities: { input: ['text', 'image'], output: ['text'] },
            knowledge: '2024-04',
            release_date: '2025-04-14',
            family: 'gpt-nano',
            status: 'deprecated',
            source: 'models.dev',
            note: '参照价，非结算依据',
            found: true,
          },
        }),
      ),
    )

    const snapshot = await createAdminGatewayResource().catalog.getReference({
      modelId: 'openai/gpt-4.1-nano',
    })

    expect(snapshot).toMatchObject({
      modelId: 'openai/gpt-4.1-nano',
      reasoning: false,
      toolCall: true,
      temperature: true,
      attachment: true,
      structuredOutput: true,
      openWeights: false,
      modalities: { input: ['text', 'image'], output: ['text'] },
      knowledge: '2024-04',
      releaseDate: '2025-04-14',
      family: 'gpt-nano',
      status: 'deprecated',
      found: true,
    })
    // description 永不透传：快照无 description 键。
    expect(snapshot).not.toHaveProperty('description')
  })

  it('lists 7-day observed snapshots with SKU on-sale markers and never-fabricated margins', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/ai-gateway/models-observed', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: [
            {
              model: 'openai/gpt-image-1',
              provider: 'OpenAI',
              window_days: 7,
              window_since: '2026-09-15T00:00:00Z',
              attempts_total: 100,
              completed: 92,
              failed: 8,
              cache_hits: 30,
              success_rate: 0.92,
              cache_hit_rate: 0.3,
              top_failure_stage: 'render',
              top_failure_category: 'timeout',
              top_failure_count: 5,
              has_observations: true,
              has_sku_margin: true,
              sku_margins: [
                {
                  sku: 'ai-image:gpt-image-1',
                  display_name: 'GPT Image 1',
                  feature: 'generation',
                  on_sale: true,
                  unit_credits_by_variant: { default: 100 },
                  actual_cost_credits: null,
                  has_actual_cost: false,
                  cost_source: 'unavailable',
                  cost_note: '网关实际成本暂无来源',
                  margin_credits: null,
                  margin_status: 'unknown_no_cost',
                },
              ],
            },
            {
              model: 'seedream-v2',
              provider: 'ByteDance',
              window_days: 7,
              window_since: '2026-09-15T00:00:00Z',
              attempts_total: 0,
              completed: 0,
              failed: 0,
              cache_hits: 0,
              success_rate: null,
              cache_hit_rate: null,
              top_failure_stage: '',
              top_failure_category: '',
              top_failure_count: 0,
              has_observations: false,
              has_sku_margin: false,
              sku_margins: [],
            },
          ],
        }),
      ),
    )

    const snapshots = await createAdminGatewayResource().catalog.listObserved()

    expect(snapshots).toHaveLength(2)
    expect(snapshots[0]).toMatchObject({
      model: 'openai/gpt-image-1',
      windowDays: 7,
      successRate: 0.92,
      cacheHitRate: 0.3,
      topFailureStage: 'render',
      topFailureCategory: 'timeout',
      topFailureCount: 5,
      hasObservations: true,
      hasSkuMargin: true,
    })
    expect(snapshots[0]!.skuMargins[0]).toMatchObject({
      sku: 'ai-image:gpt-image-1',
      displayName: 'GPT Image 1',
      onSale: true,
      // 成本/毛利恒 null：未编造任何数字。
      actualCostCredits: null,
      marginCredits: null,
      marginStatus: 'unknown_no_cost',
    })
    // 静默模型 rates 为 null，调用方按暂无数据渲染。
    expect(snapshots[1]).toMatchObject({
      model: 'seedream-v2',
      successRate: null,
      cacheHitRate: null,
      hasObservations: false,
    })
  })

  it('surfaces save validation errors', async () => {
    useMswHandlers(
      http.put('/api/admin/settings/ai-gateway/models/:feature', () =>
        HttpResponse.json({ code: 400, msg: '模型必须是 provider/model 格式且不含空白' }),
      ),
    )

    await expect(createAdminGatewayResource().mappings.save('generation', 'bad')).rejects.toThrow()
  })

  it('rejects gateway config updates when the HTTP 200 envelope reports an error', async () => {
    useMswHandlers(
      http.put('/api/admin/settings/ai-gateway', () =>
        HttpResponse.json({ code: 400, msg: '网关配置校验失败' }),
      ),
    )

    await expect(
      createAdminGatewayResource().connection.update({ base_url: 'https://gateway.example.com' }),
    ).rejects.toThrow()
  })

  it('resolves gateway config updates when the envelope reports success', async () => {
    useMswHandlers(
      http.put('/api/admin/settings/ai-gateway', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: null }),
      ),
    )

    await expect(
      createAdminGatewayResource().connection.update({ base_url: 'https://gateway.example.com' }),
    ).resolves.toBeUndefined()
  })
})
