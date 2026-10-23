import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/vue'
import { userEvent } from '@testing-library/user-event'
import CatalogIdentityPanel from './CatalogIdentityPanel.vue'
import CatalogSpecsPanel from './CatalogSpecsPanel.vue'
import CatalogGatewayPanel from './CatalogGatewayPanel.vue'
import CatalogPreviewCard from './CatalogPreviewCard.vue'
import CatalogReferencePanel from './CatalogReferencePanel.vue'
import type { AdminModelItem } from '../../../core/catalogData'

// The reference lookup has no control equivalent (fail-closed in the
// resource); panel rendering is covered through a fake snapshot source
// while the resource spec pins the fail-closed contract.
const referenceFakes = vi.hoisted(() => ({
  snapshot: vi.fn(),
}))

vi.mock('../../../resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../../resource')>()
  return {
    ...actual,
    createAdminGatewayResource: () => ({
      catalog: { getReference: referenceFakes.snapshot },
    }),
  }
})

function resolveReferenceSnapshot(payload: Record<string, unknown>) {
  const item = payload as {
    model_id?: string
    provider?: string
    name?: string
    input_usd_per_mtok?: number
    output_usd_per_mtok?: number
    context_limit?: number
    last_updated?: string
    reasoning?: boolean
    tool_call?: boolean
    temperature?: boolean
    attachment?: boolean
    structured_output?: boolean
    open_weights?: boolean
    modalities?: { input?: string[]; output?: string[] }
    knowledge?: string
    release_date?: string
    family?: string
    status?: string
    source?: string
    note?: string
    found?: boolean
    error_code?: string
    message?: string
  }
  return {
    modelId: item.model_id ?? '',
    provider: item.provider,
    name: item.name,
    inputUsdPerMtok: item.input_usd_per_mtok,
    outputUsdPerMtok: item.output_usd_per_mtok,
    contextLimit: item.context_limit,
    lastUpdated: item.last_updated,
    reasoning: item.reasoning,
    toolCall: item.tool_call,
    temperature: item.temperature,
    attachment: item.attachment,
    structuredOutput: item.structured_output,
    openWeights: item.open_weights,
    modalities: item.modalities,
    knowledge: item.knowledge,
    releaseDate: item.release_date,
    family: item.family,
    status: item.status,
    source: item.source ?? '',
    note: item.note ?? '',
    found: item.found ?? false,
    errorCode: item.error_code,
    message: item.message,
  }
}

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
    beforeEach(() => {
      referenceFakes.snapshot.mockReset()
    })

    it('renders backend reference pricing with a non-settlement note', async () => {
      referenceFakes.snapshot.mockResolvedValue(
        resolveReferenceSnapshot({
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
        }),
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
      render(CatalogReferencePanel, {
        props: { modelId: 'fal_ai/flux-dev', provider: 'Fal AI', modelName: 'FLUX' },
      })

      expect(await screen.findByText(/fal 模型暂无 models\.dev 数据/)).toBeInTheDocument()
      // debounce 窗口后仍无请求：失败 handler 一旦命中必出 error 文案。
      await new Promise((resolve) => setTimeout(resolve, 650))
      expect(referenceFakes.snapshot).not.toHaveBeenCalled()
      expect(screen.queryByText(/加载参照价失败/)).not.toBeInTheDocument()
      expect(screen.queryByText(/USD\/MTok/)).not.toBeInTheDocument()
    })

    it('renders enrichment placeholders and freshness without false deprecated warnings', async () => {
      const nowSpy = vi.spyOn(Date, 'now').mockReturnValue(Date.parse('2026-09-22T00:00:00Z'))
      try {
        referenceFakes.snapshot.mockResolvedValue(
          resolveReferenceSnapshot({
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
          }),
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
      referenceFakes.snapshot.mockResolvedValue(
        resolveReferenceSnapshot({
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
        }),
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
      // 能力徽标收口 StatusBadge：supported→success，unsupported→neutral，
      // 静态属性不呼吸；✓/✗ 后缀保留区分。
      const supportedPill = (await screen.findByText(/工具调用✓/)).parentElement!
      expect(supportedPill).toHaveClass('bg-success-soft', 'text-success')
      expect(supportedPill.querySelector('.status-badge-pulse')).toBeNull()
      const unsupportedPill = (await screen.findByText(/推理✗/)).parentElement!
      expect(unsupportedPill).toHaveClass('bg-muted', 'text-muted-foreground')
      expect(unsupportedPill.querySelector('.status-badge-pulse')).toBeNull()
      // deprecated 红 Pill：状态原文 + 已下线预警（danger 语义载体，满圆 Badge）。
      const badge = await screen.findByText(/已下线预警/)
      expect(badge.textContent).toMatch(/deprecated/)
      const badgeRoot = badge.parentElement!
      expect(badgeRoot).toHaveClass('bg-destructive-soft', 'text-destructive', 'rounded-pill')
      expect(badgeRoot.querySelector('.status-badge-pulse')).toBeNull()
      expect(screen.queryByText('暂无数据')).not.toBeInTheDocument()
    })

    it('renders beta status as a neutral badge without a deprecation warning', async () => {
      referenceFakes.snapshot.mockResolvedValue(
        resolveReferenceSnapshot({
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
        }),
      )

      render(CatalogReferencePanel, {
        props: { modelId: 'openai/gpt-5-nano', provider: 'OpenAI', modelName: 'GPT-5 Nano' },
      })

      expect(await screen.findByText('beta')).toBeInTheDocument()
      expect(screen.queryByText(/已下线预警/)).not.toBeInTheDocument()
      // beta 收口 neutral Pill：满圆 Badge + muted 载体，静态不呼吸。
      const betaRoot = screen.getByText('beta').parentElement!
      expect(betaRoot).toHaveClass('bg-muted', 'text-muted-foreground', 'rounded-pill')
      expect(betaRoot.querySelector('.status-badge-pulse')).toBeNull()
      // 部分缺失：模态/发布/家族行回落暂无数据，不炸版。
      expect(screen.getAllByText('暂无数据').length).toBeGreaterThanOrEqual(3)
    })

    it('explains reference misses by error code without blocking the form', async () => {
      referenceFakes.snapshot.mockResolvedValue(
        resolveReferenceSnapshot({
          model_id: 'unknown/model',
          source: 'models.dev',
          note: '参照价，非结算依据',
          found: false,
          error_code: 'not_found',
        }),
      )

      render(CatalogReferencePanel, {
        props: { modelId: 'unknown/model', provider: '', modelName: '' },
      })

      expect(await screen.findByText(/models\.dev 未收录该模型/)).toBeInTheDocument()
    })

    it('never writes reference data back (gateway description stays authoritative)', async () => {
      referenceFakes.snapshot.mockResolvedValue(
        resolveReferenceSnapshot({
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
        }),
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

    it('falls back to the placeholder when the provider logo fails to load', async () => {
      referenceFakes.snapshot.mockResolvedValue(
        resolveReferenceSnapshot({
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
        }),
      )

      render(CatalogReferencePanel, {
        props: { modelId: 'openai/gpt-4o', provider: 'OpenAI', modelName: 'GPT-4o' },
      })

      const logo = await screen.findByRole('img', { name: 'OpenAI' })
      expect(logo).toBeInTheDocument()
      await fireEvent.error(logo)
      // 破图隐藏，回落 Cube 占位，不再有可访问的品牌图。
      expect(screen.queryByRole('img', { name: 'OpenAI' })).not.toBeInTheDocument()
    })

    it('does not let an older reference response overwrite a newer model', async () => {
      let resolveOld!: (value: unknown) => void
      let resolveNew!: (value: unknown) => void
      const oldPromise = new Promise((resolve) => {
        resolveOld = resolve
      })
      const newPromise = new Promise((resolve) => {
        resolveNew = resolve
      })
      referenceFakes.snapshot.mockReturnValueOnce(oldPromise).mockReturnValueOnce(newPromise)

      const rendered = render(CatalogReferencePanel, {
        props: { modelId: 'openai/old', provider: 'OpenAI', modelName: 'Old' },
      })
      await waitFor(() => expect(referenceFakes.snapshot).toHaveBeenCalledTimes(1))

      await rendered.rerender({ modelId: 'openai/new', provider: 'OpenAI', modelName: 'New' })
      await new Promise((resolve) => setTimeout(resolve, 450))
      await waitFor(() => expect(referenceFakes.snapshot).toHaveBeenCalledTimes(2))

      resolveNew(
        resolveReferenceSnapshot({
          model_id: 'openai/new',
          provider: 'OpenAI',
          name: 'New Reference',
          source: 'models.dev',
          note: '参照价，非结算依据',
          found: true,
        }),
      )
      await screen.findByText('New Reference')

      resolveOld(
        resolveReferenceSnapshot({
          model_id: 'openai/old',
          provider: 'OpenAI',
          name: 'Old Reference',
          source: 'models.dev',
          note: '参照价，非结算依据',
          found: true,
        }),
      )
      await new Promise((resolve) => setTimeout(resolve, 0))
      expect(screen.queryByText('Old Reference')).not.toBeInTheDocument()
      expect(screen.getByText('New Reference')).toBeInTheDocument()
    })
  })
})
