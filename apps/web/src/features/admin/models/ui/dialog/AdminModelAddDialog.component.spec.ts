import { beforeEach, describe, expect, it, vi } from 'vitest'
import { within } from '@testing-library/vue'
import { renderWithUser, screen } from '@/test/render'
import { mockAppToast } from '@/test/toast'
import AdminModelAddDialog from './AdminModelAddDialog.vue'
import type { AdminModelItem } from '../../core/catalogData'

const gatewayFakes = vi.hoisted(() => ({
  listProviders: vi.fn(),
}))

vi.mock('../../resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../resource')>()
  return {
    ...actual,
    createAdminGatewayResource: () => ({
      catalog: { listProviders: gatewayFakes.listProviders },
    }),
  }
})

vi.mock('../../../../../core/composables/useAppToast')

describe('AdminModelAddDialog', () => {
  const mockProviders = [
    { slug: 'openai', display_name: 'OpenAI', id_prefix: 'openai/' },
    { slug: 'workers-ai', display_name: 'Workers AI', id_prefix: 'workers-ai/' },
  ]

  beforeEach(() => {
    vi.clearAllMocks()
    mockAppToast()
    gatewayFakes.listProviders.mockResolvedValue(mockProviders)
  })

  const existingModels: AdminModelItem[] = [
    {
      id: 'gpt-4o',
      name: 'GPT-4o',
      provider: 'OpenAI',
      description: '',
      contextOrResolution: '',
      capabilities: [],
      hosting: '第三方',
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

  it('loads configured providers on open', async () => {
    renderDialog({ existingModels: [] })

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: '添加模型' })).toBeInTheDocument()
    expect(gatewayFakes.listProviders).toHaveBeenCalled()
    expect(await within(dialog).findByText('OpenAI')).toBeInTheDocument()
  })

  it('imports a hand-filled bare model id with the selected provider slug', async () => {
    const importSpy = vi.fn()
    const { user } = renderDialog({ existingModels: [], onImport: importSpy })

    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('OpenAI')

    const input = within(dialog).getByPlaceholderText('例如: gpt-image-1（只需裸模型名）')
    await user.type(input, 'gpt-image-1')

    await user.click(within(dialog).getByRole('button', { name: '导入模型' }))

    expect(importSpy).toHaveBeenCalledOnce()
    const imported = importSpy.mock.calls[0]?.[0] as AdminModelItem[]
    expect(imported).toHaveLength(1)
    expect(imported[0]?.id).toBe('gpt-image-1')
    expect(imported[0]?.provider).toBe('openai')
  })

  it('rejects a repeated selected-provider prefix', async () => {
    const importSpy = vi.fn()
    const { user } = renderDialog({ existingModels: [], onImport: importSpy })

    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('OpenAI')

    const input = within(dialog).getByPlaceholderText('例如: gpt-image-1（只需裸模型名）')
    await user.type(input, 'openai/gpt-4o')

    expect(await within(dialog).findByText('模型名称不要重复所选供应商前缀')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '导入模型' })).toBeDisabled()
    expect(importSpy).not.toHaveBeenCalled()
  })

  it.each([
    ['openrouter', 'google/gemini-3-flash'],
    ['openrouter', 'openai/gpt-5-image'],
    ['fal', 'fal-ai/flux-pro/v1.1'],
  ])('imports native nested model names for %s', async (slug, id) => {
    gatewayFakes.listProviders.mockResolvedValueOnce([
      { slug, display_name: slug, id_prefix: `${slug}/` },
    ])
    const importSpy = vi.fn()
    const { user } = renderDialog({ existingModels: [], onImport: importSpy })
    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText(slug)
    await user.type(within(dialog).getByPlaceholderText('例如: gpt-image-1（只需裸模型名）'), id)
    await user.click(within(dialog).getByRole('button', { name: '导入模型' }))
    expect(importSpy).toHaveBeenCalledWith([expect.objectContaining({ provider: slug, id })])
  })

  it('blocks duplicate model ids already in the library', async () => {
    const importSpy = vi.fn()
    const { user } = renderDialog({ onImport: importSpy })

    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('OpenAI')

    const input = within(dialog).getByPlaceholderText('例如: gpt-image-1（只需裸模型名）')
    await user.type(input, 'gpt-4o')

    expect(await within(dialog).findByText('该模型标识已在库中')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '导入模型' })).toBeDisabled()
    expect(importSpy).not.toHaveBeenCalled()
  })

  it('shows an empty state when no provider is configured', async () => {
    gatewayFakes.listProviders.mockResolvedValueOnce([])
    renderDialog({ existingModels: [] })

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('网关未配置任何 Provider')).toBeInTheDocument()
  })

  it('retries when the provider list fails to load', async () => {
    gatewayFakes.listProviders.mockRejectedValueOnce(new Error('unavailable'))
    renderDialog({ existingModels: [] })

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('unavailable')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: '重试' })).toBeInTheDocument()
  })

  it('disables providers the Worker cannot adapt and shows the hint', async () => {
    gatewayFakes.listProviders.mockResolvedValueOnce([
      // eslint-disable-next-line governance/boolean-prefix -- Wire contract field (provider availability).
      { slug: 'krea', display_name: 'Krea', id_prefix: 'krea/', available: false },
      // eslint-disable-next-line governance/boolean-prefix -- Wire contract field (provider availability).
      { slug: 'openai', display_name: 'OpenAI', id_prefix: 'openai/', available: true },
    ])
    const { user } = renderDialog({ existingModels: [] })

    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('OpenAI')

    // The 缺适配器 hint sits next to the dropdown once any row is unadapted.
    expect(
      within(dialog).getByText('已在网关启用、但 Worker 暂无适配器的供应商不可选。'),
    ).toBeInTheDocument()

    // The trigger defaults to the first available provider, skipping krea.
    expect(within(dialog).getByRole('combobox', { name: '供应商（已配）' })).toHaveTextContent(
      'OpenAI',
    )

    await user.click(within(dialog).getByRole('combobox', { name: '供应商（已配）' }))
    // Reka renders the option list outside the dialog subtree, so options
    // are queried on the screen, not within the dialog.
    const kreaOption = await screen.findByRole('option', {
      name: 'Krea（缺适配器）',
    })
    expect(kreaOption).toHaveAttribute('aria-disabled', 'true')
    expect(screen.getByRole('option', { name: 'OpenAI' })).not.toHaveAttribute(
      'aria-disabled',
      'true',
    )
  })

  it('blocks import when every provider is unadapted', async () => {
    gatewayFakes.listProviders.mockResolvedValueOnce([
      // eslint-disable-next-line governance/boolean-prefix -- Wire contract field (provider availability).
      { slug: 'krea', display_name: 'Krea', id_prefix: 'krea/', available: false },
    ])
    const importSpy = vi.fn()
    const { user } = renderDialog({ existingModels: [], onImport: importSpy })

    const dialog = await screen.findByRole('dialog')
    await within(dialog).findByText('Krea（缺适配器）')

    const input = within(dialog).getByPlaceholderText('例如: gpt-image-1（只需裸模型名）')
    await user.type(input, 'v1')
    expect(within(dialog).getByRole('button', { name: '导入模型' })).toBeDisabled()
    expect(importSpy).not.toHaveBeenCalled()
  })
})
