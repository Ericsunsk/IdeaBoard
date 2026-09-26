import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { fireEvent } from '@testing-library/vue'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminPromptConfigPage from './AdminPromptConfigPage.vue'

const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('../../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../shared/lib/toast')>()),
  useAdminToast: () => toast,
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({
    confirm: vi.fn().mockResolvedValue(true),
    readReason: () => 'prompt review',
  }),
}))

describe('AdminPromptConfigPage', () => {
  let storedPrompts: Array<{ feature_slug: string; prompt: string; updated_at?: string }> = []

  beforeEach(() => {
    toast.success.mockReset()
    toast.warning.mockReset()
    toast.error.mockReset()
    storedPrompts = [
      {
        feature_slug: 'render',
        prompt: 'Masterpiece, best quality, {{prompt}}',
        updated_at: '2026-03-30T00:00:00Z',
      },
      { feature_slug: 'variations', prompt: '', updated_at: '2026-03-30T00:00:00Z' },
      {
        feature_slug: 'inpaint',
        prompt: 'Inpaint prompt: {{prompt}}',
        updated_at: '2026-03-30T00:00:00Z',
      },
      { feature_slug: 'outpaint', prompt: '', updated_at: '2026-03-30T00:00:00Z' },
      {
        feature_slug: 'generation',
        prompt: 'Generate creative prompt: {{input}}',
        updated_at: '2026-03-30T00:00:00Z',
      },
      { feature_slug: 'fusion', prompt: '', updated_at: '2026-03-30T00:00:00Z' },
    ]

    useMswHandlers(
      http.get('/api/control/v1/platform/feature-prompts', () => {
        return HttpResponse.json({
          data: { prompts: storedPrompts },
        })
      }),
      http.put('/api/control/v1/platform/feature-prompts/:feature', async ({ params, request }) => {
        const slug = String(params.feature)
        const body = (await request.json()) as { prompt: string }
        const existing = storedPrompts.find((p) => p.feature_slug === slug)
        if (existing) {
          existing.prompt = body.prompt
        } else {
          storedPrompts.push({
            feature_slug: slug,
            prompt: body.prompt,
            updated_at: new Date().toISOString(),
          })
        }
        return HttpResponse.json({
          data: { feature_slug: slug, prompt: body.prompt, updated_at: new Date().toISOString() },
        })
      }),
    )
  })

  function renderPage() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: 0 },
      },
    })
    return renderWithUser(AdminPromptConfigPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })
  }

  it('renders prompt configuration cards aligned with model management card tokens', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('共 3 个工作流模块')).toBeInTheDocument()
    })

    // Group headings and item counts should be rendered
    expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Prompt' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Fusion' })).toBeInTheDocument()
    expect(screen.getByText('共 4 个功能')).toBeInTheDocument()
    expect(screen.getAllByText('共 1 个功能')).toHaveLength(2)
  })

  it('filters groups and features by search keyword', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('共 3 个工作流模块')).toBeInTheDocument()
    })

    const searchInput = screen.getByLabelText('搜索提示词')
    await fireEvent.update(searchInput, 'Fusion')

    expect(screen.getByText('共 1 个工作流模块')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Fusion' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Image' })).not.toBeInTheDocument()

    // Clear search
    await fireEvent.update(searchInput, '')
    expect(screen.getByText('共 3 个工作流模块')).toBeInTheDocument()
  })

  it('opens editor dialog, inserts variable, and saves successfully', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText('已配置').length).toBeGreaterThan(0)
    })

    // Click row-level "配置" button
    const configureBtn = screen.getByRole('button', { name: '配置 Image · 草图成图 提示词' })
    await fireEvent.click(configureBtn)

    // Dialog should open
    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: '编辑 · Render' })).toBeInTheDocument()
    })

    // Dialog should be strictly dedicated to the clicked feature and not stack other features
    expect(screen.getByText('编辑 · Render')).toBeInTheDocument()
    expect(screen.queryByText('编辑 · Inpaint')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Image · 局部重绘 提示词模板')).not.toBeInTheDocument()

    // Check that variable button exists
    const variableButton = screen.getAllByRole('button', { name: '{{user_prompt}}' })[0]!
    expect(variableButton).toBeInTheDocument()

    // Insert variable
    await fireEvent.click(variableButton)

    // Save prompt
    const saveButtons = screen.getAllByRole('button', { name: /保存/ })
    expect(saveButtons.length).toBeGreaterThan(0)
    await fireEvent.click(saveButtons[0]!)

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalled()
    })
  })

  it('filters by configuration status via popover filter', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText('已配置').length).toBeGreaterThan(0)
    })

    // Open status filter popover
    const statusBtn = screen.getByRole('button', { name: '筛选提示词状态' })
    await fireEvent.click(statusBtn)

    // Filter by "已配置"
    const configuredOpt = screen.getByRole('option', { name: '已配置' })
    await fireEvent.click(configuredOpt)

    // Fusion has 0 configured prompts, so only Image and Prompt should match
    await waitFor(() => {
      expect(screen.getByText('共 2 个工作流模块')).toBeInTheDocument()
    })
    expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Prompt' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Fusion' })).not.toBeInTheDocument()

    // Filter by "未配置"
    await fireEvent.click(statusBtn)
    const unconfiguredOpt = screen.getByRole('option', { name: '未配置' })
    await fireEvent.click(unconfiguredOpt)

    // Prompt has 1/1 configured, so Prompt should be filtered out
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Fusion' })).toBeInTheDocument()
    })
    expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Prompt' })).not.toBeInTheDocument()
  })

  it('renders table columns and opens editor from row action button', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText('已配置').length).toBeGreaterThan(0)
    })

    // Table headers should be present
    expect(screen.getAllByText('Service').length).toBeGreaterThan(0)
    expect(screen.getAllByText('Status').length).toBeGreaterThan(0)
    expect(screen.getAllByText('可用变量').length).toBeGreaterThan(0)
    expect(screen.getAllByText('系统提示词预览').length).toBeGreaterThan(0)
    expect(screen.getAllByText('操作').length).toBeGreaterThan(0)

    // Check status badges
    expect(screen.getAllByText('已配置').length).toBeGreaterThan(0)
    expect(screen.getAllByText('未配置').length).toBeGreaterThan(0)

    // Click row-level action button for render feature
    const rowBtn = screen.getByRole('button', { name: '配置 Image · 草图成图 提示词' })
    await fireEvent.click(rowBtn)

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: '编辑 · Render' })).toBeInTheDocument()
    })
  })
})
