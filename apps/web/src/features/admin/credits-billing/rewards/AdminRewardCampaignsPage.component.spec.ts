import { beforeEach, describe, expect, it, vi } from 'vitest'
import PrimeVue from 'primevue/config'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminRewardCampaignsPage from './AdminRewardCampaignsPage.vue'
import { RewardCampaignConflictError } from './rewardCampaignResource'

const resource = vi.hoisted(() => ({
  activate: vi.fn(),
  cancel: vi.fn(),
  create: vi.fn(),
  delete: vi.fn(),
  issuePublicCode: vi.fn(),
  list: vi.fn(),
  pause: vi.fn(),
  resume: vi.fn(),
  update: vi.fn(),
}))
const confirmRequestMock = vi.hoisted(() => vi.fn())
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))

vi.mock('./rewardCampaignResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./rewardCampaignResource')>()),
  createAdminRewardCampaignResource: () => resource,
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ request: confirmRequestMock }),
}))

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => toast,
}))

describe('AdminRewardCampaignsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resource.list.mockResolvedValue({ campaigns: [makeCampaign()] })
    resource.create.mockResolvedValue(makeCampaign({ id: 8, lifecycleStatus: 'draft', status: 'draft' }))
    resource.update.mockResolvedValue(makeCampaign({ internalNote: '更新后的备注' }))
    resource.pause.mockResolvedValue(
      makeCampaign({ lifecycleStatus: 'paused', status: 'paused' }),
    )
  })

  it('shows derived and lifecycle status, Shanghai schedule, Ideas, cap, and progress', async () => {
    const { user } = renderPage()

    expect(await screen.findByText('待开始')).toBeVisible()
    expect(screen.getByText('已激活')).toBeVisible()
    expect(screen.getByText('2026/07/15 18:30 - 2026/08/01 00:00')).toBeVisible()
    expect(screen.getByText('80 Ideas')).toBeVisible()
    expect(screen.getByText('100 次')).toBeVisible()
    expect(screen.getByText('12 / 100')).toBeVisible()
    expect(
      screen.getByRole('region', { name: '奖励活动列表横向滚动区域' }),
    ).toBeVisible()

    await user.click(screen.getByRole('button', { name: '管理新用户奖励' }))
    expect(screen.getByRole('heading', { name: '新用户奖励' })).toBeVisible()
  })

  it('passes Asia/Shanghai schedule fields to Draft creation', async () => {
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
  })

  it('creates an unlimited single-use Campaign with a zero cap', async () => {
    resource.create.mockResolvedValueOnce(
      makeCampaign({
        id: 8,
        mode: 'single_use',
        lifecycleStatus: 'draft',
        status: 'draft',
        maxRedemptions: 0,
      }),
    )
    const { user } = renderPage()
    await screen.findByText('待开始')

    await user.click(screen.getByRole('button', { name: '单次码' }))
    await user.type(screen.getByRole('textbox', { name: '展示名称' }), '单次码活动')
    await user.type(screen.getByRole('spinbutton', { name: 'Idea 数量' }), '50')
    await user.type(screen.getByRole('textbox', { name: /创建原因/ }), '创建单次码活动')
    await user.click(screen.getByRole('button', { name: '创建单次码 Draft' }))

    expect(resource.create).toHaveBeenCalledWith(
      expect.objectContaining({ mode: 'single_use', maxRedemptions: 0 }),
    )
    expect(await screen.findByText('Code Batches')).toBeVisible()
  })

  it('confirms an audited update and explains frozen core rules on conflict', async () => {
    resource.update.mockRejectedValueOnce(new RewardCampaignConflictError('rules_frozen'))
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: '管理新用户奖励' }))

    const note = screen.getByRole('textbox', { name: '活动内部备注' })
    await user.clear(note)
    await user.type(note, '更新后的备注')
    await user.type(screen.getByRole('textbox', { name: /更新原因/ }), '补充运营说明')
    await user.click(screen.getByRole('button', { name: '保存活动设置' }))

    expect(confirmRequestMock).toHaveBeenCalledWith(
      expect.objectContaining({ title: '确认更新奖励活动' }),
    )
    const confirmation = confirmRequestMock.mock.calls[0]?.[0] as {
      onConfirm: () => Promise<void>
    }
    await confirmation.onConfirm()

    expect(resource.update).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ internalNote: '更新后的备注', reason: '补充运营说明' }),
    )
    expect(toast.error).toHaveBeenCalledWith('活动已发行兑换码，仅内部备注可以修改')
  })

  it('collects a reason before pausing an active lifecycle', async () => {
    const { user } = renderPage()
    await user.click(await screen.findByRole('button', { name: '管理新用户奖励' }))
    await user.click(screen.getByRole('button', { name: '暂停活动' }))

    await user.type(screen.getByRole('textbox', { name: /^操作原因/ }), '临时停止投放')
    await user.click(screen.getByRole('button', { name: '确认暂停' }))

    await waitFor(() => expect(resource.pause).toHaveBeenCalledWith(7, '临时停止投放'))
  })

  it('keeps cancellation available for an issued-but-not-activated Draft', async () => {
    resource.list.mockResolvedValue({
      campaigns: [makeCampaign({ lifecycleStatus: 'draft', status: 'draft' })],
    })
    const { user } = renderPage()

    await user.click(await screen.findByRole('button', { name: '管理新用户奖励' }))

    expect(screen.getByRole('button', { name: '激活活动' })).toBeVisible()
    expect(screen.getByRole('button', { name: '取消活动' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '删除空 Draft' })).not.toBeInTheDocument()
  })

  it('only offers deletion for an empty Draft', async () => {
    resource.list.mockResolvedValue({
      campaigns: [
        makeCampaign({
          lifecycleStatus: 'draft',
          status: 'draft',
          issuedCodeCount: 0,
          validUnusedCodeCount: 0,
        }),
      ],
    })
    const { user } = renderPage()

    await user.click(await screen.findByRole('button', { name: '管理新用户奖励' }))

    expect(screen.getByRole('button', { name: '删除空 Draft' })).toBeVisible()
    expect(screen.queryByRole('button', { name: '激活活动' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '取消活动' })).not.toBeInTheDocument()
  })
})

function renderPage() {
  return renderWithUser(AdminRewardCampaignsPage, {
    global: {
      plugins: [[PrimeVue, primeVueTestOptions]],
      stubs: {
        RewardCodeBatchPanel: {
          props: ['campaign'],
          template: '<div>Code Batches</div>',
        },
      },
    },
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
