import { beforeEach, describe, expect, it, vi } from 'vitest'
import PrimeVue from 'primevue/config'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminRewardCampaignsPage from './AdminRewardCampaignsPage.vue'

const routerPush = vi.hoisted(() => vi.fn())
const resource = vi.hoisted(() => ({ create: vi.fn(), list: vi.fn() }))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRouter: () => ({ push: routerPush }),
}))

vi.mock('./rewardCampaignResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./rewardCampaignResource')>()),
  createAdminRewardCampaignResource: () => resource,
}))

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => toast,
}))

describe('AdminRewardCampaignsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    routerPush.mockResolvedValue(undefined)
    resource.list.mockResolvedValue({ campaigns: [makeCampaign()], nextBeforeId: 6 })
    resource.create.mockResolvedValue(
      makeCampaign({ id: 8, lifecycleStatus: 'draft', status: 'draft' }),
    )
  })

  it('renders the filtered Campaign table and opens the independent detail route', async () => {
    const { user } = renderPage()

    expect(await screen.findByText('待开始')).toBeVisible()
    expect(screen.getByText('已激活')).toBeVisible()
    expect(screen.getByText('2026/07/15 18:30 - 2026/08/01 00:00')).toBeVisible()
    expect(screen.getByText('80 Ideas')).toBeVisible()
    expect(screen.getByText('12 / 100')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '管理' }))
    expect(routerPush).toHaveBeenCalledWith('/admin/credits/reward-campaigns/7')
  })

  it('resets filters before searching and advances keyset pagination', async () => {
    resource.list
      .mockResolvedValueOnce({ campaigns: [makeCampaign()], nextBeforeId: 6 })
      .mockResolvedValueOnce({ campaigns: [makeCampaign({ id: 6 })] })
      .mockResolvedValueOnce({ campaigns: [makeCampaign({ id: 5, displayName: '八月活动' })] })
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() => expect(resource.list).toHaveBeenLastCalledWith(6, expect.any(Object)))

    const search = screen.getByPlaceholderText('搜索活动名称')
    await user.type(search, '八月活动{Enter}')
    await waitFor(() =>
      expect(resource.list).toHaveBeenLastCalledWith(undefined, {
        search: '八月活动',
        mode: undefined,
        status: undefined,
      }),
    )
  })

  it('passes Asia/Shanghai fields to Draft creation then opens the detail', async () => {
    const { user } = renderPage()
    await screen.findByText('待开始')

    await user.type(screen.getByRole('textbox', { name: '展示名称' }), '八月活动')
    await user.type(screen.getByRole('spinbutton', { name: 'Idea 数量' }), '50')
    await user.type(screen.getByRole('spinbutton', { name: '总兑换上限' }), '20')
    await user.type(screen.getByLabelText('开始时间（Asia/Shanghai）'), '2026-08-01T09:00')
    await user.type(screen.getByLabelText('结束时间（Asia/Shanghai）'), '2026-08-10T18:00')
    await user.type(screen.getByRole('textbox', { name: /创建原因/ }), '创建八月活动')
    await user.click(screen.getByRole('button', { name: '创建公共 Draft' }))

    expect(resource.create).toHaveBeenCalledWith(
      expect.objectContaining({
        displayName: '八月活动',
        mode: 'public',
        startsAt: '2026-08-01T09:00',
        expiresAt: '2026-08-10T18:00',
        reason: '创建八月活动',
      }),
    )
    await waitFor(() =>
      expect(routerPush).toHaveBeenCalledWith('/admin/credits/reward-campaigns/8'),
    )
  })
})

function renderPage() {
  return renderWithUser(AdminRewardCampaignsPage, {
    global: { plugins: [[PrimeVue, primeVueTestOptions]] },
  })
}

function makeCampaign(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    displayName: '新用户奖励',
    internalNote: '七月活动',
    creditAmount: 80,
    mode: 'public',
    lifecycleStatus: 'active',
    status: 'scheduled',
    maxRedemptions: 100,
    redemptionCount: 12,
    issuedCodeCount: 40,
    validUnusedCodeCount: 28,
    startsAt: '2026-07-15T10:30:00Z',
    expiresAt: '2026-07-31T16:00:00Z',
    createdBy: 1,
    createdAt: '2026-07-15T10:00:00Z',
    updatedAt: '2026-07-15T10:00:00Z',
    ...overrides,
  }
}
