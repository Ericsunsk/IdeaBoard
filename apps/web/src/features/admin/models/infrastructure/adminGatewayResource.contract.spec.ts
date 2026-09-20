import { describe, expect, it } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import {
  createAdminGatewayResource,
} from './adminGatewayResource'

describe('Admin gateway resource', () => {
  it('lists per-feature model mappings', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/ai-gateway/models', () =>
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

  it('surfaces save validation errors', async () => {
    useMswHandlers(
      http.put('/api/admin/settings/ai-gateway/models/:feature', () =>
        HttpResponse.json({ code: 400, msg: '模型必须是 provider/model 格式且不含空白' }),
      ),
    )

    await expect(
      createAdminGatewayResource().mappings.save('generation', 'bad'),
    ).rejects.toThrow()
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
