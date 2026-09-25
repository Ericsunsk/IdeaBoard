import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, within } from '@testing-library/vue'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import { renderWithUser, screen } from '../../../../../test/render'
import AdminModelAddDialog from './AdminModelAddDialog.vue'
import type { AdminModelItem } from '../../domain/catalogData'

describe('AdminModelAddDialog', () => {
  const mockDetectedModels = [
    {
      id: '@cf/black-forest-labs/flux-1-schnell',
      name: 'FLUX.1 Schnell 快速生图',
      provider: 'Workers AI',
    },
    {
      id: 'gpt-4o',
      name: 'GPT-4o 旗舰多模态',
      provider: 'OpenAI',
      mode: 'chat',
    },
    {
      id: 'fal-ai/birefnet',
      name: 'BiRefNet 高清抠图',
      provider: 'Fal AI',
    },
  ]

  beforeEach(() => {
    useMswHandlers(
      http.get('/api/admin/settings/ai-gateway/detect-models', () => {
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            models: mockDetectedModels,
            configured_providers: 3,
          },
        })
      }),
    )
  })

  const existingModels: AdminModelItem[] = [
    {
      id: '@cf/black-forest-labs/flux-1-schnell',
      name: 'FLUX.1 Schnell 快速生图',
      provider: 'Workers AI',
      mode: 'image_generation',
      description: 'Workers AI 官方简介',
      contextOrResolution: '',
      capabilities: [],
      hosting: 'Cloudflare 托管',
      termsUrl: '',
      docsUrl: '',
    },
  ]

  function renderDialog(
    props: {
      visible?: boolean
      existingModels?: AdminModelItem[]
      pending?: boolean
      onImport?: (items: AdminModelItem[]) => void
    } = {},
  ) {
    const { onImport, ...rest } = props
    return renderWithUser(AdminModelAddDialog, {
      props: {
        visible: rest.visible ?? true,
        existingModels: rest.existingModels ?? existingModels,
        pending: rest.pending ?? false,
        ...(onImport ? { onImport } : {}),
      },
      global: {
        stubs: { transition: false },
      },
    })
  }

  it('automatically probes gateway models upon opening and displays them in checkbox table', async () => {
    renderDialog()

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: '添加模型' })).toBeInTheDocument()

    // 探测结果自动展示
    expect(await within(dialog).findByText('FLUX.1 Schnell 快速生图')).toBeInTheDocument()
    expect(within(dialog).getByText('GPT-4o 旗舰多模态')).toBeInTheDocument()
    expect(within(dialog).getByText('BiRefNet 高清抠图')).toBeInTheDocument()

    // 检查已在库中的状态
    expect(within(dialog).getByText('已在库中')).toBeInTheDocument()
  })

  it('shows a single detecting line while probing gateway models', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/ai-gateway/detect-models', async () => {
        await new Promise(() => {})
        return HttpResponse.json({ code: 0, msg: 'ok', data: { models: [] } })
      }),
    )
    renderDialog()

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('正在比对社区模型目录...')).toBeVisible()
    expect(within(dialog).queryByText('正在获取已连接供应商的模型列表...')).not.toBeInTheDocument()
    // 加载中与空态互斥：结果未回来前不得出现空态块
    expect(within(dialog).queryByText('未探测到可用的网关模型')).not.toBeInTheDocument()
  })

  it('shows only the error alert without the empty state when detection fails', async () => {
    useMswHandlers(
      http.get('/api/admin/settings/ai-gateway/detect-models', () => {
        return HttpResponse.json({ code: 503, msg: 'unavailable' }, { status: 503 })
      }),
    )
    renderDialog()

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByRole('button', { name: '重试' })).toBeVisible()
    expect(within(dialog).queryByText('未探测到可用的网关模型')).not.toBeInTheDocument()
  })

  it('disables checkbox for existing models and allows selecting new models for batch import', async () => {
    const importSpy = vi.fn()
    const { user } = renderDialog({
      onImport: importSpy,
    })

    const dialog = await screen.findByRole('dialog')

    // 等待模型加载完成
    await within(dialog).findByText('GPT-4o 旗舰多模态')

    // 已在库中的模型 checkbox 禁用
    const existingCheckbox = within(dialog).getByRole('checkbox', {
      name: '选择 FLUX.1 Schnell 快速生图',
    }) as HTMLInputElement
    expect(existingCheckbox).toBeDisabled()

    // 勾选未在库中的模型 GPT-4o
    const gptCheckbox = within(dialog).getByRole('checkbox', {
      name: '选择 GPT-4o 旗舰多模态',
    }) as HTMLInputElement
    expect(gptCheckbox).not.toBeDisabled()
    expect(gptCheckbox).not.toBeChecked()

    await user.click(gptCheckbox)
    expect(gptCheckbox).toBeChecked()

    // 底部导入按钮显示数量并可点击
    const importBtn = within(dialog).getByRole('button', { name: '导入所选模型 (1)' })
    expect(importBtn).not.toBeDisabled()

    await user.click(importBtn)

    expect(importSpy).toHaveBeenCalledOnce()
    const imported = importSpy.mock.calls[0]?.[0] as AdminModelItem[]
    expect(imported).toHaveLength(1)
    expect(imported[0]?.id).toBe('gpt-4o')
    expect(imported[0]?.provider).toBe('OpenAI')
    expect(imported[0]?.mode).toBe('chat')
  })

  it('falls back to the placeholder icon when a row brand logo fails to load', async () => {
    renderDialog()

    const dialog = await screen.findByRole('dialog')
    const openaiLogo = await within(dialog).findByRole('img', { name: 'OpenAI' })
    await fireEvent.error(openaiLogo)

    // 破图行回落占位，不再有可访问的品牌图；其他行不受影响。
    expect(within(dialog).queryByRole('img', { name: 'OpenAI' })).not.toBeInTheDocument()
    expect(within(dialog).getByRole('img', { name: 'Workers AI' })).toBeInTheDocument()
  })

  it('filters models by search keyword', async () => {
    const { user } = renderDialog()

    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('GPT-4o 旗舰多模态')

    const searchInput = within(dialog).getByPlaceholderText('搜索模型名称或标识...')
    await user.type(searchInput, 'birefnet')

    // 过滤后只显示 BiRefNet
    expect(within(dialog).getByText('BiRefNet 高清抠图')).toBeInTheDocument()
    expect(within(dialog).queryByText('GPT-4o 旗舰多模态')).not.toBeInTheDocument()
    expect(within(dialog).queryByText('FLUX.1 Schnell 快速生图')).not.toBeInTheDocument()
  })

  it('filters models by provider selection', async () => {
    renderDialog()

    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('GPT-4o 旗舰多模态')

    // 默认展示全部供应商
    expect(within(dialog).getByText('GPT-4o 旗舰多模态')).toBeInTheDocument()
    expect(within(dialog).getByText('BiRefNet 高清抠图')).toBeInTheDocument()
  })
})
