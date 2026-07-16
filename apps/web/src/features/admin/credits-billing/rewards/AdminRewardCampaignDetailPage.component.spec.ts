import { beforeEach, describe, expect, it, vi } from 'vitest'
import PrimeVue from 'primevue/config'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminRewardCampaignDetailPage from './AdminRewardCampaignDetailPage.vue'

const campaignResource = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn() }))
const codeResource = vi.hoisted(() => ({ lookup: vi.fn(), revoke: vi.fn() }))
const redemptionResource = vi.hoisted(() => ({ list: vi.fn() }))
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => ({ params: { campaignId: '7' } }),
  useRouter: () => router,
}))

vi.mock('./rewardCampaignResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./rewardCampaignResource')>()),
  createAdminRewardCampaignResource: () => campaignResource,
}))
vi.mock('./rewardCodeResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./rewardCodeResource')>()),
  createAdminRewardCodeResource: () => codeResource,
}))
vi.mock('./rewardRedemptionResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./rewardRedemptionResource')>()),
  createAdminRewardRedemptionResource: () => redemptionResource,
}))
vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ request: vi.fn() }),
}))
vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => toast,
}))

describe('AdminRewardCampaignDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    campaignResource.get.mockResolvedValue(makeCampaign())
    redemptionResource.list.mockResolvedValue({ redemptions: [] })
    codeResource.lookup.mockResolvedValue({
      id: 17,
      campaignId: 7,
      campaignDisplayName: '新用户奖励',
      batchId: 3,
      maskedCodeHint: '•••• T293',
      status: 'available',
      issuedAt: '2026-07-16T01:00:00Z',
    })
    codeResource.revoke.mockResolvedValue({
      id: 17,
      campaignId: 7,
      campaignDisplayName: '新用户奖励',
      batchId: 3,
      maskedCodeHint: '•••• T293',
      status: 'revoked',
      issuedAt: '2026-07-16T01:00:00Z',
      revokedAt: '2026-07-16T02:00:00Z',
    })
  })

  it('uses exactly three detail tabs and revokes only masked exact-lookup results', async () => {
    const { user } = renderPage()

    expect(await screen.findByRole('heading', { name: '新用户奖励' })).toBeVisible()
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent)).toEqual([
      'Overview',
      'Code Batches',
      'Redemptions',
    ])

    await user.click(screen.getByRole('tab', { name: 'Code Batches' }))
    const input = screen.getByRole('textbox', { name: '精确查找兑换码' })
    await user.type(input, 'SECRET-T293')
    await user.click(screen.getByRole('button', { name: '查找' }))

    expect(await screen.findByText('•••• T293')).toBeVisible()
    expect(input).toHaveValue('')
    expect(screen.queryByText('SECRET-T293')).not.toBeInTheDocument()
    expect(codeResource.lookup).toHaveBeenCalledWith('SECRET-T293')

    await user.click(screen.getByRole('button', { name: '撤销' }))
    await user.type(screen.getByRole('textbox', { name: /^操作原因/ }), '运营撤销')
    await user.click(screen.getByRole('button', { name: '确认撤销' }))

    await waitFor(() => expect(codeResource.revoke).toHaveBeenCalledWith(17, '运营撤销'))
    expect(toast.success).toHaveBeenCalledWith('兑换码已撤销')
  })
})

function renderPage() {
  return renderWithUser(AdminRewardCampaignDetailPage, {
    global: {
      plugins: [[PrimeVue, primeVueTestOptions]],
      stubs: { RewardCodeBatchPanel: { template: '<div>Batch metadata</div>' } },
    },
  })
}

function makeCampaign() {
  return {
    id: 7,
    displayName: '新用户奖励',
    internalNote: '七月活动',
    creditAmount: 80,
    mode: 'single_use',
    lifecycleStatus: 'active',
    status: 'active',
    maxRedemptions: 100,
    redemptionCount: 12,
    issuedCodeCount: 40,
    validUnusedCodeCount: 28,
    startsAt: '2026-07-15T10:30:00Z',
    expiresAt: '2026-07-31T16:00:00Z',
    createdBy: 1,
    createdAt: '2026-07-15T10:00:00Z',
    updatedAt: '2026-07-15T10:00:00Z',
  }
}
