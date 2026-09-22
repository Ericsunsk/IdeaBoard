import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/vue'
import { userEvent } from '@testing-library/user-event'
import { http, HttpResponse, useMswHandlers } from '../../../../../../test/msw'
import CatalogIdentityPanel from './CatalogIdentityPanel.vue'
import CatalogSpecsPanel from './CatalogSpecsPanel.vue'
import CatalogGatewayPanel from './CatalogGatewayPanel.vue'
import CatalogPreviewCard from './CatalogPreviewCard.vue'
import CatalogReferencePanel from './CatalogReferencePanel.vue'
import type { AdminModelItem } from '../../../domain/catalogData'

describe('AdminModelEditDialog subcomponents', () => {
  describe('CatalogIdentityPanel', () => {
    it('renders fields and handles input update', async () => {
      const user = userEvent.setup()
      const onUpdateId = vi.fn()
      render(CatalogIdentityPanel, {
        props: {
          modelId: '',
          modelName: 'My Model',
          modelProvider: 'Fal AI',
          modelDescription: 'Test desc',
          isEditing: false,
          idPrefixOptions: ['fal_ai/'],
          providerOptions: [{ slug: 'fal', display_name: 'Fal AI', id_prefix: 'fal_ai/' }],
          'onUpdate:modelId': onUpdateId,
        },
      })

      expect(screen.getByLabelText(/模型标识/)).toBeInTheDocument()
      const prefixBtn = screen.getByRole('button', { name: 'fal_ai/' })
      await user.click(prefixBtn)
      expect(onUpdateId).toHaveBeenCalledWith('fal_ai/')
    })
  })

  describe('CatalogSpecsPanel', () => {
    it('handles resolution presets and adding capabilities', async () => {
      const user = userEvent.setup()
      const onUpdateRes = vi.fn()
      const onUpdateCaps = vi.fn()
      render(CatalogSpecsPanel, {
        props: {
          contextOrResolution: '2048×2048',
          hosting: 'Cloudflare 托管',
          capabilitiesDraft: '垫图生图',
          'onUpdate:contextOrResolution': onUpdateRes,
          'onUpdate:capabilitiesDraft': onUpdateCaps,
        },
      })

      await user.click(screen.getByRole('button', { name: '4096×4096' }))
      expect(onUpdateRes).toHaveBeenCalledWith('4096×4096')

      await user.click(screen.getByRole('button', { name: '+ 透明通道' }))
      expect(onUpdateCaps).toHaveBeenCalledWith('垫图生图, 透明通道')
    })
  })

  describe('CatalogGatewayPanel', () => {
    it('toggles collapsible section and handles JSON formatting', async () => {
      const user = userEvent.setup()
      const onUpdateParams = vi.fn()
      render(CatalogGatewayPanel, {
        props: {
          customPath: '',
          customParams: '{"a":1}',
          initiallyOpen: true,
          'onUpdate:customParams': onUpdateParams,
        },
      })

      expect(screen.getByLabelText(/额外自定义请求参数/)).toBeInTheDocument()
      const formatBtn = screen.getByRole('button', { name: '格式化 JSON' })
      await user.click(formatBtn)
      expect(onUpdateParams).toHaveBeenCalledWith('{\n  "a": 1\n}')
    })
  })

  describe('CatalogPreviewCard', () => {
    it('renders preview model and route preview', () => {
      const sampleModel: AdminModelItem = {
        id: 'fal/flux-dev',
        name: 'FLUX.1 [dev]',
        provider: 'Fal AI',
        description: '优质生图模型',
        contextOrResolution: '2048×2048',
        capabilities: ['垫图生图'],
        hosting: 'Cloudflare 托管',
        termsUrl: '',
        docsUrl: '',
      }

      render(CatalogPreviewCard, {
        props: {
          model: sampleModel,
          modelId: 'fal/flux-dev',
          routePreview: {
            providerSlug: 'fal',
            isFirstParty: true,
            targetPath: 'fal-ai/flux-dev',
            protocol: '原生第一方',
          },
        },
      })

      expect(screen.getByText('卡片效果实时预览')).toBeInTheDocument()
      expect(screen.getByText('ID: fal/flux-dev')).toBeInTheDocument()
      expect(screen.getByText('Cloudflare AI Gateway 路由透视')).toBeInTheDocument()
      expect(screen.getAllByText('原生第一方').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('fal-ai/flux-dev')).toBeInTheDocument()
    })
  })

  describe('CatalogReferencePanel', () => {
    it('renders backend reference pricing with a non-settlement note', async () => {
      useMswHandlers(
        http.get('/api/admin/settings/ai-gateway/models-reference', () =>
          HttpResponse.json({
            code: 0,
            msg: 'ok',
            data: {
              model_id: 'openai/gpt-4o',
              provider: 'OpenAI',
              name: 'GPT-4o',
              input_usd_per_mtok: 2.5,
              output_usd_per_mtok: 10,
              context_limit: 128000,
              last_updated: '2026-01-01',
              source: 'models.dev',
              note: '参照价，非结算依据',
              found: true,
            },
          }),
        ),
      )

      render(CatalogReferencePanel, {
        props: { modelId: 'openai/gpt-4o', provider: 'OpenAI', modelName: 'GPT-4o' },
      })

      expect(await screen.findByText('$2.5 in · $10 out /1M')).toBeInTheDocument()
      expect(await screen.findByText('128K')).toBeInTheDocument()
      expect(await screen.findByText('2026-01-01')).toBeInTheDocument()
      expect(await screen.findByText(/参照价，非结算依据/)).toBeInTheDocument()
      expect(await screen.findByText(/source: models\.dev/)).toBeInTheDocument()
    })

    it('shows fal models a graceful fallback without calling the reference backend', async () => {
      useMswHandlers(
        http.get('/api/admin/settings/ai-gateway/models-reference', () =>
          HttpResponse.json({ code: 500, msg: 'must not be called', data: null }),
        ),
      )

      render(CatalogReferencePanel, {
        props: { modelId: 'fal_ai/flux-dev', provider: 'Fal AI', modelName: 'FLUX' },
      })

      expect(await screen.findByText(/fal 模型暂无 models\.dev 数据/)).toBeInTheDocument()
      // debounce 窗口后仍无请求：失败 handler 一旦命中必出 error 文案。
      await new Promise((resolve) => setTimeout(resolve, 650))
      expect(screen.queryByText(/加载参照价失败|must not be called/)).not.toBeInTheDocument()
      expect(screen.queryByText(/USD\/MTok/)).not.toBeInTheDocument()
    })

    it('renders enrichment placeholders and freshness without false deprecated warnings', async () => {
      const nowSpy = vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-09-22T00:00:00Z'))
      try {
        useMswHandlers(
          http.get('/api/admin/settings/ai-gateway/models-reference', () =>
            HttpResponse.json({
              code: 0,
              msg: 'ok',
              data: {
                model_id: 'openai/gpt-4o',
                provider: 'OpenAI',
                name: 'GPT-4o',
                input_usd_per_mtok: 2.5,
                output_usd_per_mtok: 10,
                context_limit: 128000,
                last_updated: '2026-01-01',
                source: 'models.dev',
                note: '参照价，非结算依据',
                found: true,
              },
            }),
          ),
        )

        render(CatalogReferencePanel, {
          props: { modelId: 'openai/gpt-4o', provider: 'OpenAI', modelName: 'GPT-4o' },
        })

        expect(await screen.findByText('(8 个月前)')).toBeInTheDocument()
        // 后端能力字段未到齐：各 enrichment 行一律暂无数据，不炸版。
        expect(screen.getAllByText('暂无数据').length).toBeGreaterThanOrEqual(5)
        // 无 status 数据时绝不误报 deprecated 红色预警。
        expect(screen.queryByText(/deprecated|已下线/)).not.toBeInTheDocument()
      } finally {
        nowSpy.mockRestore()
      }
    })

    it('wires enrichment rows and raises a red deprecated badge', async () => {
      useMswHandlers(
        http.get('/api/admin/settings/ai-gateway/models-reference', () =>
          HttpResponse.json({
            code: 0,
            msg: 'ok',
            data: {
              model_id: 'openai/gpt-4.1-nano',
              provider: 'OpenAI',
              name: 'GPT-4.1 nano',
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

      render(CatalogReferencePanel, {
        props: { modelId: 'openai/gpt-4.1-nano', provider: 'OpenAI', modelName: 'GPT-4.1 nano' },
      })

      // 能力行：true→✓，false→✗，全量接线不再是暂无数据。
      expect(await screen.findByText(/推理✗/)).toBeInTheDocument()
      expect(await screen.findByText(/工具调用✓/)).toBeInTheDocument()
      expect(await screen.findByText(/开放权重✗/)).toBeInTheDocument()
      expect(await screen.findByText('text / image → text')).toBeInTheDocument()
      expect(await screen.findByText('2025-04-14 / 截止 2024-04')).toBeInTheDocument()
      expect(await screen.findByText('gpt-nano')).toBeInTheDocument()
      // deprecated 红 badge：状态原文 + 已下线预警。
      const badge = await screen.findByText(/已下线预警/)
      expect(badge.textContent).toMatch(/deprecated/)
      expect(badge.className).toMatch(/red/)
      expect(screen.queryByText('暂无数据')).not.toBeInTheDocument()
    })

    it('renders beta status as a neutral badge without a deprecation warning', async () => {
      useMswHandlers(
        http.get('/api/admin/settings/ai-gateway/models-reference', () =>
          HttpResponse.json({
            code: 0,
            msg: 'ok',
            data: {
              model_id: 'openai/gpt-5-nano',
              provider: 'OpenAI',
              name: 'GPT-5 Nano',
              input_usd_per_mtok: 0.05,
              output_usd_per_mtok: 0.4,
              context_limit: 400000,
              last_updated: '2025-08-07',
              reasoning: true,
              status: 'beta',
              source: 'models.dev',
              note: '参照价，非结算依据',
              found: true,
            },
          }),
        ),
      )

      render(CatalogReferencePanel, {
        props: { modelId: 'openai/gpt-5-nano', provider: 'OpenAI', modelName: 'GPT-5 Nano' },
      })

      expect(await screen.findByText('beta')).toBeInTheDocument()
      expect(screen.queryByText(/已下线预警/)).not.toBeInTheDocument()
      // 部分缺失：模态/发布/家族行回落暂无数据，不炸版。
      expect(screen.getAllByText('暂无数据').length).toBeGreaterThanOrEqual(3)
    })

    it('explains reference misses by error code without blocking the form', async () => {
      useMswHandlers(
        http.get('/api/admin/settings/ai-gateway/models-reference', () =>
          HttpResponse.json({
            code: 0,
            msg: 'ok',
            data: {
              model_id: 'unknown/model',
              source: 'models.dev',
              note: '参照价，非结算依据',
              found: false,
              error_code: 'not_found',
            },
          }),
        ),
      )

      render(CatalogReferencePanel, {
        props: { modelId: 'unknown/model', provider: '', modelName: '' },
      })

      expect(await screen.findByText(/models\.dev 未收录该模型/)).toBeInTheDocument()
    })

    it('never writes reference data back (gateway description stays authoritative)', async () => {
      useMswHandlers(
        http.get('/api/admin/settings/ai-gateway/models-reference', () =>
          HttpResponse.json({
            code: 0,
            msg: 'ok',
            data: {
              model_id: 'openai/gpt-4o',
              provider: 'OpenAI',
              name: '上游参照名（仅展示）',
              input_usd_per_mtok: 2.5,
              output_usd_per_mtok: 10,
              context_limit: 128000,
              last_updated: '2026-01-01',
              source: 'models.dev',
              note: '参照价，非结算依据',
              found: true,
            },
          }),
        ),
      )

      const { emitted } = render(CatalogReferencePanel, {
        props: { modelId: 'openai/gpt-4o', provider: 'OpenAI', modelName: '运营手写名' },
      })

      expect(await screen.findByText('上游参照名（仅展示）')).toBeInTheDocument()
      const user = userEvent.setup()
      await user.click(screen.getByRole('button', { name: /刷新/ }))
      expect(await screen.findByText('上游参照名（仅展示）')).toBeInTheDocument()
      // 只读面板：无任何 update:* 回写事件，表单 description 永不被参照回写
      //（emitted 含 user-event 派发的原生 DOM 事件，只断言无 update: 前缀）。
      expect(Object.keys(emitted()).filter((key) => key.startsWith('update:'))).toEqual([])
    })

    it('does not let an older reference response overwrite a newer model', async () => {
      const responses: Array<(response: Response) => void> = []
      useMswHandlers(
        http.get('/api/admin/settings/ai-gateway/models-reference', ({ request }) => {
          const modelID = new URL(request.url).searchParams.get('model_id') || ''
          return new Promise<Response>((resolve) => {
            responses.push((response) => resolve(response))
            void modelID
          })
        }),
      )

      const rendered = render(CatalogReferencePanel, {
        props: { modelId: 'openai/old', provider: 'OpenAI', modelName: 'Old' },
      })
      await waitFor(() => expect(responses).toHaveLength(1))

      await rendered.rerender({ modelId: 'openai/new', provider: 'OpenAI', modelName: 'New' })
      await new Promise((resolve) => setTimeout(resolve, 450))
      await waitFor(() => expect(responses).toHaveLength(2))

      responses[1](
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            model_id: 'openai/new',
            provider: 'OpenAI',
            name: 'New Reference',
            source: 'models.dev',
            note: '参照价，非结算依据',
            found: true,
          },
        }),
      )
      await screen.findByText('New Reference')

      responses[0](
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            model_id: 'openai/old',
            provider: 'OpenAI',
            name: 'Old Reference',
            source: 'models.dev',
            note: '参照价，非结算依据',
            found: true,
          },
        }),
      )
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(screen.queryByText('Old Reference')).not.toBeInTheDocument()
      expect(screen.getByText('New Reference')).toBeInTheDocument()
    })
  })
})
