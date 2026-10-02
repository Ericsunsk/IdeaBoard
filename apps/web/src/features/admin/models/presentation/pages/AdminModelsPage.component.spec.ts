import { beforeEach, describe, expect, it, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { fireEvent, within } from '@testing-library/vue'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../../test/render'
import AdminModelsPage from './AdminModelsPage.vue'

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('catalog review'))
const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))
const gatewayFakes = vi.hoisted(() => ({
  detect: vi.fn(),
  listObserved: vi.fn(),
  previewRoute: vi.fn(),
}))

vi.mock('../../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock, readReason: readReasonMock }),
}))

// Detect/observed/route-preview have no control equivalent (fail-closed in
// the resource); page UX keeps coverage through fakes while the resource
// spec pins the fail-closed contract. Everything else rides the real
// resource over MSW control handlers.
vi.mock('../../infrastructure/adminGatewayResource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../infrastructure/adminGatewayResource')>()
  return {
    ...actual,
    createAdminGatewayResource: () => {
      const real = actual.createAdminGatewayResource()
      return {
        ...real,
        catalog: {
          ...real.catalog,
          detect: gatewayFakes.detect,
          listObserved: gatewayFakes.listObserved,
          previewRoute: gatewayFakes.previewRoute,
        },
      }
    },
  }
})

vi.mock('../../../shared/lib/toast', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../shared/lib/toast')>()),
  useAdminToast: () => toast,
}))

describe('AdminModelsPage', () => {
  // 有状态的目录 mock：mutation 后列表从服务端重取，mock 必须记住写入。
  let storedCatalog: ReturnType<typeof wireCatalogModels> = []

  beforeEach(() => {
    confirmMock.mockReset()
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReset()
    readReasonMock.mockReturnValue('catalog review')
    toast.success.mockReset()
    toast.warning.mockReset()
    toast.error.mockReset()
    storedCatalog = wireCatalogModels()
    gatewayFakes.detect.mockReset()
    gatewayFakes.listObserved.mockReset()
    gatewayFakes.previewRoute.mockReset()
    gatewayFakes.detect.mockResolvedValue({
      models: [
        {
          id: 'anthropic/claude-image-v1',
          name: 'Claude Image V1',
          provider: 'Anthropic',
        },
      ],
      configured_providers: 1,
    })
    gatewayFakes.listObserved.mockResolvedValue([makeObservedSnapshot()])
    gatewayFakes.previewRoute.mockResolvedValue({
      provider_slug: 'custom-test',
      is_first_party: false,
      target_path: 'v1/images/generations',
      path_source: 'generations',
    })

    useMswHandlers(
      http.get('/api/control/v1/platform/ai-gateway/catalog', () => {
        // MSW 模拟的是线路格式（snake_case），与真实后端一致。
        return HttpResponse.json({ data: storedCatalog })
      }),
      http.put('/api/control/v1/platform/ai-gateway/catalog', async ({ request }) => {
        const body = (await request.json()) as { items?: typeof storedCatalog }
        storedCatalog = body.items ?? storedCatalog
        return HttpResponse.json({ data: {} })
      }),
      http.get('/api/control/v1/platform/ai-gateway', () => {
        return HttpResponse.json({
          data: { account_id: 'cf-acc-1', gateway_id: 'gw-1', token_configured: true },
        })
      }),
      http.put('/api/control/v1/platform/ai-gateway', () => {
        return HttpResponse.json({ data: {} })
      }),
      http.get('/api/control/v1/platform/ai-gateway/providers', () =>
        HttpResponse.json({
          data: [
            { slug: 'fal', display_name: 'Fal AI', id_prefix: 'fal_ai/' },
            { slug: 'openai', display_name: 'OpenAI', id_prefix: 'openai/' },
            { slug: 'google-vertex-ai', display_name: 'Google Vertex AI', id_prefix: 'vertex_ai/' },
          ],
        }),
      ),
    )
  })

  it('renders Cloudflare-style models layout with search, filters, and model cards', async () => {
    renderModelsPage()

    expect(screen.getByRole('textbox', { name: '搜索模型' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '筛选模型类型' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '筛选提供商' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '筛选托管方式' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '刷新模型' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新增网关' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '添加模型' })).toBeInTheDocument()

    expect(await screen.findByText('我们找到了 2 个模型')).toBeInTheDocument()
    expect(screen.getByText('seedream-v2-pro')).toBeInTheDocument()
    expect(screen.getByText('gpt-image-1')).toBeInTheDocument()
  })

  it('renders brand logos for recognized models and shows community mode tag', async () => {
    renderModelsPage()
    await screen.findByText('我们找到了 2 个模型')

    // 验证 OpenAI 模型卡片渲染了对应品牌 Logo
    const openaiImg = screen.getByRole('img', { name: 'OpenAI' })
    expect(openaiImg).toBeInTheDocument()

    // mode 原样透传展示（无站内分类体系）
    expect(screen.getAllByText('image_generation')).toHaveLength(2)
  })

  it('hides a broken brand logo instead of showing a broken image', async () => {
    renderModelsPage()
    await screen.findByText('我们找到了 2 个模型')

    const openaiImg = screen.getByRole('img', { name: 'OpenAI' })
    expect(openaiImg).toBeInTheDocument()
    await fireEvent.error(openaiImg)

    // 卡片品牌图加载失败即隐藏，回落 Cube 占位。
    expect(screen.queryByRole('img', { name: 'OpenAI' })).not.toBeInTheDocument()
  })

  it('filters models dynamically based on keyword and can clear filters', async () => {
    renderModelsPage()

    const searchInput = screen.getByRole('textbox', { name: '搜索模型' })
    await fireEvent.update(searchInput, 'seedream')

    expect(await screen.findByText('我们找到了 1 个模型')).toBeInTheDocument()
    expect(screen.getByText('seedream-v2-pro')).toBeInTheDocument()
    expect(screen.queryByText('gpt-image-1')).not.toBeInTheDocument()

    const clearButton = screen.getByRole('button', { name: '清空搜索' })
    await fireEvent.click(clearButton)

    expect(await screen.findByText('我们找到了 2 个模型')).toBeInTheDocument()
    expect(screen.getByText('gpt-image-1')).toBeInTheDocument()
  })

  it('shows empty state when no models match search criteria', async () => {
    renderModelsPage()

    const searchInput = screen.getByRole('textbox', { name: '搜索模型' })
    await fireEvent.update(searchInput, 'non-existent-model-xyz')

    expect(screen.getByText('未找到匹配的模型')).toBeInTheDocument()
    expect(
      screen.getByText('未找到符合当前条件的模型。请尝试调整搜索关键词或重置筛选器。'),
    ).toBeInTheDocument()

    const resetButton = screen.getByRole('button', { name: '清空筛选条件' })
    await fireEvent.click(resetButton)

    expect(await screen.findByText('我们找到了 2 个模型')).toBeInTheDocument()
  })

  it('supports copying model ID', async () => {
    renderModelsPage()

    await screen.findByText('我们找到了 2 个模型')
    const copyButtons = screen.getAllByRole('button', { name: /复制 .* 标识/ })
    expect(copyButtons.length).toBeGreaterThan(0)
    await fireEvent.click(copyButtons[0]!)

    expect(await screen.findByText('已复制')).toBeInTheDocument()
  })

  it('opens add model dialog and successfully imports a detected model', async () => {
    let savedList: unknown = null
    useMswHandlers(
      http.put('/api/control/v1/platform/ai-gateway/catalog', async ({ request }) => {
        savedList = await request.json()
        storedCatalog = (savedList as { items: typeof storedCatalog }).items ?? storedCatalog
        return HttpResponse.json({ data: {} })
      }),
    )

    const { user } = renderModelsPage()
    await user.click(screen.getByRole('button', { name: '添加模型' }))

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: '添加模型' })).toBeInTheDocument()
    await waitFor(() => expect(dialog.textContent).toMatch(/Claude/))
    console.log(
      'CLAUDE-HTML:',
      JSON.stringify(dialog.innerHTML?.match(/.{0,400}Claude Image V1.{0,200}/s)?.[0]),
    )

    // 自动探测出模型并展示（行内文本经 Portal 渲染，用角色查询代替文本匹配）
    const claudeCheckbox = await within(dialog).findByRole('checkbox', {
      name: '选择 Claude Image V1',
    })
    await user.click(claudeCheckbox)

    await user.click(within(dialog).getByRole('button', { name: /导入所选模型/ }))

    await waitFor(() => expect(savedList).not.toBeNull())
    expect(toast.success).toHaveBeenCalledWith('已成功导入 1 个模型到模型库')
    expect(screen.getByText('Claude Image V1')).toBeInTheDocument()
  })

  it('opens an empty edit dialog for manual entry from the add dialog', async () => {
    const { user } = renderModelsPage()
    await user.click(screen.getByRole('button', { name: '添加模型' }))

    const addDialog = screen.getByRole('dialog')
    expect(within(addDialog).getByRole('heading', { name: '添加模型' })).toBeInTheDocument()
    await user.click(within(addDialog).getByRole('button', { name: '手动新增' }))

    // 手动新增打开 EditDialog 空表单（探测不到的 Provider 在此入库）
    const editDialog = await screen.findByRole('dialog')
    expect(within(editDialog).getByRole('heading', { name: '新增模型' })).toBeInTheDocument()
  })

  it('edits an existing model and updates the list', async () => {
    const { user } = renderModelsPage()
    const editBtn = await screen.findByRole('button', { name: '编辑 seedream-v2-pro' })
    await user.click(editBtn)

    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: '编辑模型' })).toBeInTheDocument()

    const nameInput = within(dialog).getByLabelText(/显示名称/)
    await user.clear(nameInput)
    await user.type(nameInput, 'seedream-v2-ultra')

    await user.click(within(dialog).getByRole('button', { name: '保存修改' }))

    await waitFor(() => expect(screen.getByText('seedream-v2-ultra')).toBeInTheDocument())
    expect(toast.success).toHaveBeenCalledWith('模型修改已保存')
  })

  it('confirms and deletes a model', async () => {
    let savedList: unknown = null
    useMswHandlers(
      http.put('/api/control/v1/platform/ai-gateway/catalog', async ({ request }) => {
        savedList = await request.json()
        return HttpResponse.json({ data: {} })
      }),
    )

    const { user } = renderModelsPage()
    const deleteBtn = await screen.findByRole('button', { name: '删除 seedream-v2-pro' })
    await user.click(deleteBtn)

    expect(confirmMock).toHaveBeenCalledOnce()

    await waitFor(() => expect(savedList).not.toBeNull())
    expect(toast.success).toHaveBeenCalledWith('已删除模型「seedream-v2-pro」')
  })

  it('triggers refresh to sync models and reports status', async () => {
    const { user } = renderModelsPage()
    const refreshBtn = screen.getByRole('button', { name: '刷新模型' })
    await user.click(refreshBtn)

    await waitFor(() => {
      expect(toast.success).toHaveBeenCalled()
    })
  })

  it('renders observed stats on cards and graceful fallback for unobserved models', async () => {
    renderModelsPage()
    await screen.findByText('我们找到了 2 个模型')

    // 有观测的卡片：成功率/失败分类/cache 命中率/在售标记。
    expect(await screen.findByText('92%')).toBeInTheDocument()
    expect(screen.getByText('render / timeout · 5 次')).toBeInTheDocument()
    expect(screen.getByText('GPT Image 1 · 在售')).toBeInTheDocument()
    // margin/cost 恒 null：暂无数据，不编造。
    expect(screen.getByText('成本 / 毛利')).toBeInTheDocument()
    // 无观测的卡片（seedream-v2 缺席接口）：优雅缺省。
    expect(screen.getByText(/暂无 7 天观测数据/)).toBeInTheDocument()
  })

  it('opens add gateway dialog when clicking 新增网关 button', async () => {
    const { user } = renderModelsPage()
    const addGatewayBtn = screen.getByRole('button', { name: '新增网关' })
    await user.click(addGatewayBtn)

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('heading', { name: '添加网关' })).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/网关 Base URL/)).toBeInTheDocument()
    expect(within(dialog).getByLabelText(/API 密钥/)).toBeInTheDocument()
  })
})

function makeObservedSnapshot() {
  return {
    model: 'openai/gpt-image-1',
    provider: 'OpenAI',
    windowDays: 7,
    windowSince: '2026-09-15T00:00:00Z',
    attemptsTotal: 100,
    completed: 92,
    failed: 8,
    cacheHits: 30,
    successRate: 0.92,
    cacheHitRate: 0.3,
    topFailureStage: 'render',
    topFailureCategory: 'timeout',
    topFailureCount: 5,
    hasObservations: true,
    hasSkuMargin: true,
    skuMargins: [
      {
        sku: 'ai-image:gpt-image-1',
        displayName: 'GPT Image 1',
        feature: 'generation',
        onSale: true,
        unitCreditsByVariant: { default: 100 },
        actualCostCredits: null,
        hasActualCost: false,
        costSource: 'unavailable',
        costNote: '网关实际成本暂无来源',
        marginCredits: null,
        marginStatus: 'unknown_no_cost',
      },
    ],
  }
}

function renderModelsPage() {
  // Fresh client per render: the catalog query caches by key, so sharing one
  // across tests would serve stale lists and hide refetch behavior.
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return renderWithUser(AdminModelsPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: { 'router-link': { template: '<a v-bind="$attrs"><slot /></a>' } },
    },
  })
}

function wireCatalogModels() {
  return [
    {
      id: 'seedream-v2',
      name: 'seedream-v2-pro',
      provider: 'ByteDance',
      mode: 'image_generation',
      description: 'seed model',
      context_or_resolution: '2048×2048',
      capabilities: ['高精细节'],
      hosting: 'Cloudflare 托管',
      terms_url: '',
      docs_url: '',
    },
    {
      id: 'openai/gpt-image-1',
      name: 'gpt-image-1',
      provider: 'OpenAI',
      mode: 'image_generation',
      description: 'seed model',
      context_or_resolution: '4096×4096',
      capabilities: [],
      hosting: '第三方',
      terms_url: '',
      docs_url: '',
    },
  ]
}
