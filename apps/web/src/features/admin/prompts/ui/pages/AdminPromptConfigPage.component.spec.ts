import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { fireEvent, within } from '@testing-library/vue'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../../test/render'
import AdminPromptConfigPage from './AdminPromptConfigPage.vue'

const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('../../../shared/session/feedback/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../shared/session/feedback/toast')>()),
  useAdminToast: () => toast,
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
      expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    })

    // Group headings remain visible without redundant counts.
    expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Prompt' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Fusion' })).toBeInTheDocument()
    expect(screen.queryByText(/共 \d+ 个工作流模块/)).not.toBeInTheDocument()
    expect(screen.queryByText(/共 \d+ 个功能/)).not.toBeInTheDocument()
  })

  it('shows unsupported cutout with the pricing table placeholders and no edit action', async () => {
    renderPage()

    const row = await screen.findByRole('row', { name: /cutout/ })
    expect(within(row).getAllByText('—')).toHaveLength(4)
    expect(within(row).queryByText('未配置')).not.toBeInTheDocument()
    expect(within(row).queryByRole('button')).not.toBeInTheDocument()
  })

  it('filters groups and features by search keyword', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    })

    const searchInput = screen.getByLabelText('搜索提示词')
    await fireEvent.update(searchInput, 'Fusion')

    expect(screen.getByRole('heading', { name: 'Fusion' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Image' })).not.toBeInTheDocument()

    // Clear search
    await fireEvent.update(searchInput, '')
    expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Prompt' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Fusion' })).toBeInTheDocument()
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

  it('filters by configuration status via the status dropdown', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText('已配置').length).toBeGreaterThan(0)
    })

    // The status filter is a reka Select, so it reports `combobox`. It used to be
    // a Popover that claimed `aria-haspopup="listbox"` without implementing the
    // keyboard model. The skin geometry and the open/select behaviour are covered
    // by AdminOptionFilter's component and browser suites; reka renders its list
    // into a portal that jsdom will not open on click, so this page-level spec
    // only pins that the page wires the dropdown up under the new semantics.
    const statusTrigger = screen.getByRole('combobox', { name: '筛选提示词状态' })
    expect(statusTrigger).toHaveTextContent('全部状态')
    expect(statusTrigger).toHaveAttribute('aria-expanded', 'false')
  })

  it('renders every workflow module before any status filter is applied', async () => {
    renderPage()

    // Filtering through the Select portal is covered at browser level; this keeps
    // the unfiltered baseline honest, since the preceding test also drives search.
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    })
    expect(screen.getByRole('heading', { name: 'Image' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Prompt' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Fusion' })).toBeInTheDocument()
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
