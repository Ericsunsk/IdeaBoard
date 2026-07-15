import { beforeEach, describe, expect, it, vi } from 'vitest'
import PrimeVue from 'primevue/config'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import RewardCodeBatchPanel from './RewardCodeBatchPanel.vue'

const resource = vi.hoisted(() => ({
  generate: vi.fn(),
  list: vi.fn(),
  revoke: vi.fn(),
}))
const csv = vi.hoisted(() => ({
  create: vi.fn(() => 'secure-csv'),
  download: vi.fn(),
}))
const createIdempotencyKey = vi.hoisted(() => vi.fn())
const toast = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('./rewardCodeBatchResource', () => ({
  createAdminRewardCodeBatchResource: () => resource,
}))

vi.mock('./rewardCodeBatchCsv', () => ({
  createRewardCodeBatchCsv: csv.create,
  downloadRewardCodeBatchCsv: csv.download,
}))

vi.mock('./rewardCodeBatchIdempotency', () => ({
  createRewardCodeBatchIdempotencyKey: createIdempotencyKey,
}))

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => toast,
}))

describe('RewardCodeBatchPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    createIdempotencyKey.mockReturnValue('reward-batch:key-one')
    resource.list.mockResolvedValue({ batches: [makeBatch()], nextBeforeId: 20 })
    resource.revoke.mockResolvedValue(
      makeBatch({ validUnusedCount: 0, revokedUnusedCount: 2, revokedAt: '2026-07-16T02:00:00Z' }),
    )
  })

  it('renders Batch counts and advances keyset pagination', async () => {
    resource.list.mockImplementation(async (_campaignId: number, beforeId?: number) =>
      beforeId
        ? { batches: [makeBatch({ id: 20, codeCount: 8 })] }
        : { batches: [makeBatch()], nextBeforeId: 20 },
    )
    const { user } = renderPanel()

    expect(await screen.findByText('Batch #21')).toBeVisible()
    expect(screen.getByText('3 个')).toBeVisible()
    expect(screen.getByText('2 可用')).toBeVisible()
    expect(screen.getByText('1 已兑换')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() => expect(resource.list).toHaveBeenCalledWith(7, 20))
    expect(await screen.findByText('Batch #20')).toBeVisible()
  })

  it.each([
    {
      name: 'expired Campaign',
      campaign: { lifecycleStatus: 'active' as const, status: 'expired' as const },
      reason: '活动已过期，不能追加 Batch',
    },
    {
      name: 'depleted Campaign cap',
      campaign: { maxRedemptions: 3, redemptionCount: 3 },
      reason: '活动总兑换上限已耗尽，不能追加 Batch',
    },
    {
      name: 'fully allocated Campaign cap',
      campaign: { maxRedemptions: 3, redemptionCount: 1, validUnusedCodeCount: 2 },
      reason: '活动总兑换上限已耗尽，不能追加 Batch',
    },
  ])('disables generation for a $name', async ({ campaign, reason }) => {
    renderPanel(campaign)

    expect(await screen.findByText(reason)).toBeVisible()
    expect(screen.getByRole('button', { name: '生成 Batch' })).toBeDisabled()
  })

  it('downloads the first plaintext response immediately without rendering or retaining Codes', async () => {
    resource.generate.mockResolvedValue({
      batch: makeBatch(),
      plaintexts: ['ABCD-EFGH-JKMN-PQRS', 'BCDE-FGHJ-KMNP-QRST', 'CDEF-GHJK-MNPQ-RSTV'],
      exportUnavailable: false,
      replayed: false,
    })
    const { user } = renderPanel()
    await screen.findByText('Batch #21')

    await user.click(screen.getByRole('button', { name: '生成 Batch' }))
    const count = screen.getByRole('spinbutton', { name: 'Code 数量' })
    await user.click(count)
    await user.keyboard('{Control>}a{/Control}3')
    await user.type(screen.getByRole('textbox', { name: /生成原因/ }), '首批发行')
    await user.click(screen.getByRole('button', { name: '确认生成' }))

    await waitFor(() =>
      expect(resource.generate).toHaveBeenCalledWith(7, {
        count: 3,
        idempotencyKey: 'reward-batch:key-one',
        reason: '首批发行',
      }),
    )
    expect(csv.create).toHaveBeenCalledWith({
      codes: ['ABCD-EFGH-JKMN-PQRS', 'BCDE-FGHJ-KMNP-QRST', 'CDEF-GHJK-MNPQ-RSTV'],
      campaignName: '新用户奖励',
      creditAmount: 80,
      expiresAt: '2026-07-31T16:00:00Z',
    })
    expect(csv.download).toHaveBeenCalledWith({ campaignId: 7, batchId: 21, csv: 'secure-csv' })
    expect(screen.queryByText('ABCD-EFGH-JKMN-PQRS')).not.toBeInTheDocument()
  })

  it('revokes an unavailable replay and retries the replacement with a new idempotency key', async () => {
    createIdempotencyKey
      .mockReset()
      .mockReturnValueOnce('reward-batch:key-one')
      .mockReturnValueOnce('reward-batch:key-two')
    resource.generate
      .mockResolvedValueOnce({
        batch: makeBatch(),
        plaintexts: [],
        exportUnavailable: true,
        replayed: true,
      })
      .mockResolvedValueOnce({
        batch: makeBatch({ id: 22, codeCount: 2, validUnusedCount: 2, redeemedCount: 0 }),
        plaintexts: ['DEFG-HJKM-NPQR-STVW', 'EFGH-JKMN-PQRS-TVWX'],
        exportUnavailable: false,
        replayed: false,
      })
    const { user } = renderPanel()
    await screen.findByText('Batch #21')

    await user.click(screen.getByRole('button', { name: '生成 Batch' }))
    const count = screen.getByRole('spinbutton', { name: 'Code 数量' })
    await user.click(count)
    await user.keyboard('{Control>}a{/Control}3')
    await user.type(screen.getByRole('textbox', { name: /生成原因/ }), '重试首批发行')
    await user.click(screen.getByRole('button', { name: '确认生成' }))

    expect(await screen.findByText(/Batch #21 的明文无法再次导出/)).toBeVisible()
    expect(csv.download).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: '撤销并生成替代 Batch' }))
    await user.type(screen.getByRole('textbox', { name: /^操作原因/ }), '撤销丢失导出')
    await user.click(screen.getByRole('button', { name: '确认撤销' }))

    await waitFor(() => expect(resource.revoke).toHaveBeenCalledWith(7, 21, '撤销丢失导出'))
    expect(await screen.findByText('生成替代 Batch')).toBeVisible()
    expect(screen.getByRole('spinbutton', { name: 'Code 数量' })).toHaveValue('2')

    await user.type(screen.getByRole('textbox', { name: /生成原因/ }), '替代发行')
    await user.click(screen.getByRole('button', { name: '确认生成' }))

    await waitFor(() => expect(resource.generate).toHaveBeenCalledTimes(2))
    expect(resource.generate.mock.calls[0]?.[1].idempotencyKey).toBe('reward-batch:key-one')
    expect(resource.generate.mock.calls[1]?.[1].idempotencyKey).toBe('reward-batch:key-two')
    expect(csv.download).toHaveBeenCalledOnce()
  })

  it('recovers a lost export from refreshed Batch metadata with a new idempotency key', async () => {
    createIdempotencyKey.mockReturnValue('reward-batch:replacement-after-refresh')
    const { user } = renderPanel()
    await screen.findByText('Batch #21')

    await user.click(screen.getByRole('button', { name: 'Batch #21 导出丢失' }))
    await user.type(screen.getByRole('textbox', { name: /^操作原因/ }), '刷新后发现导出丢失')
    await user.click(screen.getByRole('button', { name: '确认撤销' }))

    await waitFor(() => expect(resource.revoke).toHaveBeenCalledWith(7, 21, '刷新后发现导出丢失'))
    expect(await screen.findByText('生成替代 Batch')).toBeVisible()
    expect(screen.getByRole('spinbutton', { name: 'Code 数量' })).toHaveValue('2')

    await user.type(screen.getByRole('textbox', { name: /生成原因/ }), '刷新后替代发行')
    await user.click(screen.getByRole('button', { name: '确认生成' }))

    await waitFor(() =>
      expect(resource.generate).toHaveBeenCalledWith(7, {
        count: 2,
        idempotencyKey: 'reward-batch:replacement-after-refresh',
        reason: '刷新后替代发行',
      }),
    )
  })
})

function renderPanel(campaign: Record<string, unknown> = {}) {
  return renderWithUser(RewardCodeBatchPanel, {
    props: { campaign: makeCampaign(campaign) },
    global: {
      plugins: [[PrimeVue, primeVueTestOptions]],
      stubs: { NavArrowLeft: true, NavArrowRight: true },
    },
  })
}

function makeCampaign(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    displayName: '新用户奖励',
    internalNote: '七月活动',
    creditAmount: 80,
    mode: 'single_use' as const,
    lifecycleStatus: 'active' as const,
    status: 'active' as const,
    maxRedemptions: 0,
    redemptionCount: 1,
    issuedCodeCount: 3,
    validUnusedCodeCount: 2,
    startsAt: '2026-07-15T10:30:00Z',
    expiresAt: '2026-07-31T16:00:00Z',
    createdBy: 1,
    createdAt: '2026-07-15T10:00:00Z',
    updatedAt: '2026-07-15T10:00:00Z',
    ...overrides,
  }
}

function makeBatch(overrides: Record<string, unknown> = {}) {
  return {
    id: 21,
    campaignId: 7,
    idempotencyKey: 'reward-batch:key-one',
    codeCount: 3,
    validUnusedCount: 2,
    redeemedCount: 1,
    revokedUnusedCount: 0,
    issuedBy: 1,
    issuedAt: '2026-07-16T01:00:00Z',
    ...overrides,
  }
}
