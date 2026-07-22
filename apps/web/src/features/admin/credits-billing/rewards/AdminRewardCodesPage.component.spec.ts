import { beforeEach, describe, expect, it, vi } from 'vitest'
import PrimeVue from 'primevue/config'
import { fireEvent } from '@testing-library/vue'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminRewardCodesPage from './AdminRewardCodesPage.vue'

const resource = vi.hoisted(() => ({
  create: vi.fn(),
  delete: vi.fn(),
  search: vi.fn(),
  setEnabled: vi.fn(),
  update: vi.fn(),
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
const confirm = vi.hoisted(() => ({ request: vi.fn() }))

vi.mock('./rewardCodeAdminResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./rewardCodeAdminResource')>()),
  createAdminRewardCodeResource: () => resource,
}))

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => toast,
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => confirm,
}))

describe('AdminRewardCodesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resource.search.mockResolvedValue({ rewardCodes: [makeRewardCode()], total: 21, nextBeforeId: 6 })
    resource.create.mockResolvedValue({
      rewardCode: makeRewardCode({ id: 8 }),
      plaintext: 'ABCD12342026',
      replayed: false,
    })
    resource.update.mockResolvedValue({
      rewardCode: makeRewardCode({ displayName: '八月奖励', maskedCodeHint: '•••• WXYZ' }),
      plaintext: 'NEWC-ODE1-WXYZ',
      replayed: false,
    })
    resource.setEnabled.mockResolvedValue(makeRewardCode({ enabled: false, status: 'disabled' }))
    resource.delete.mockResolvedValue(undefined)
  })

  it('renders the canonical RewardCode list, filtered total, and pages with the keyset cursor', async () => {
    const { user } = renderPage()

    expect(await screen.findByText('新用户奖励')).toBeVisible()
    expect(screen.getByText('•••• 2026')).toBeVisible()
    expect(screen.getByText('80 Ideas')).toBeVisible()
    expect(screen.getByText('12 / 100')).toBeVisible()
    expect(screen.getByText('共 21 个兑换码')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '下一页' }))
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ beforeId: 6 }),
      ),
    )
  })

  it('sends a complete Code through the search request body without changing the route', async () => {
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    const search = screen.getByRole('textbox', { name: '搜索兑换码' })
    await user.type(search, 'ABCD-1234-2026')
    await user.keyboard('{Enter}')

    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ query: 'ABCD-1234-2026', beforeId: undefined }),
      ),
    )
    expect(window.location.search).toBe('')
  })

  it('searches by a fuzzy name and clears the applied query from the clear action', async () => {
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    const search = screen.getByRole('textbox', { name: '搜索兑换码' })
    await user.type(search, '新用')
    await user.keyboard('{Enter}')
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ query: '新用', beforeId: undefined }),
      ),
    )

    await user.click(screen.getByRole('button', { name: '清空搜索' }))
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ query: '', beforeId: undefined }),
      ),
    )
    expect(search).toHaveValue('')
  })

  it('filters by effective status and clears the keyset page before requesting', async () => {
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '筛选兑换码状态' }))
    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: '已过期' }))

    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: 'expired', beforeId: undefined }),
      ),
    )
  })

  it('keeps the newest result when an older search response resolves late', async () => {
    let resolveOld: (value: unknown) => void = () => undefined
    const oldRequest = new Promise((resolve) => {
      resolveOld = resolve
    })
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    resource.search.mockImplementationOnce(() => oldRequest)
    const search = screen.getByRole('textbox', { name: '搜索兑换码' })
    await user.type(search, '旧兑换码')
    await user.keyboard('{Enter}')
    resource.search.mockResolvedValueOnce({
      rewardCodes: [makeRewardCode({ displayName: '最新兑换码' })],
      total: 1,
    })
    await user.clear(search)
    await user.type(search, '最新兑换码')
    await user.keyboard('{Enter}')
    expect(await screen.findByText('最新兑换码')).toBeVisible()

    resolveOld({ rewardCodes: [makeRewardCode({ displayName: '旧兑换码' })], total: 1 })
    await Promise.resolve()
    expect(screen.queryByText('旧兑换码')).not.toBeInTheDocument()
  })

  it('distinguishes initial errors from a failed background refresh and recovers', async () => {
    resource.search.mockRejectedValueOnce(new Error('network unavailable'))
    const { user } = renderPage()
    expect(await screen.findByText('兑换码加载失败，请重试。')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('新用户奖励')).toBeVisible()

    resource.search.mockRejectedValueOnce(new Error('network unavailable'))
    await user.click(screen.getByRole('button', { name: '刷新兑换码' }))
    expect(await screen.findByText('刷新兑换码失败，正在显示上次成功的数据。')).toBeVisible()
    expect(screen.getByText('新用户奖励')).toBeVisible()
  })

  it('creates from the focused Dialog and shows the one-time plaintext result', async () => {
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getAllByRole('button', { name: '创建兑换码' }).at(-1)!)
    await user.type(screen.getByRole('textbox', { name: '名称' }), '八月奖励')
    resource.search.mockReturnValueOnce(new Promise(() => {}))
    await user.click(screen.getAllByRole('button', { name: '创建兑换码' }).at(-1)!)

    await waitFor(() => expect(resource.create).toHaveBeenCalledTimes(1))
    expect(resource.create).toHaveBeenCalledWith(
      expect.objectContaining({
        displayName: '八月奖励',
        creditAmount: 100,
        maxRedemptions: 0,
        idempotencyKey: expect.any(String),
        code: expect.stringMatching(/^[0-9A-HJKMNP-TV-Z]{4}(?:-[0-9A-HJKMNP-TV-Z]{4}){2}$/),
      }),
    )
    const submittedCode = resource.create.mock.calls[0]![0].code
    expect(await screen.findByText('ABCD12342026')).toBeVisible()
    expect(screen.getByText('关闭后将无法再次查看完整 Code。')).toBeVisible()

    const writeText = vi
      .fn()
      .mockRejectedValueOnce(new Error('clipboard denied'))
      .mockResolvedValueOnce(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    await user.click(screen.getByRole('button', { name: '复制' }))
    expect(await screen.findByText('复制失败，请手动选择并复制 Code。')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '复制' }))
    expect(screen.getByRole('button', { name: '已复制' })).toBeVisible()
    expect(screen.queryByText('复制失败，请手动选择并复制 Code。')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '我已保存' }))
    await waitFor(() => expect(screen.queryByText('ABCD12342026')).not.toBeInTheDocument())
    await user.click(screen.getByRole('button', { name: '创建兑换码' }))
    expect(screen.queryByText('ABCD12342026')).not.toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: 'Code' })).not.toHaveValue(submittedCode)

    await user.clear(screen.getByRole('textbox', { name: 'Code' }))
    await user.type(screen.getByRole('textbox', { name: 'Code' }), 'OLD-CODE-1234')
    await user.click(screen.getByRole('button', { name: '取消' }))
    await user.click(screen.getByRole('button', { name: '创建兑换码' }))
    expect(screen.getByRole('textbox', { name: 'Code' })).not.toHaveValue('OLD-CODE-1234')
  })

  it('uses native form submission and retries an ambiguous create with the same idempotency key', async () => {
    resource.create.mockRejectedValueOnce(new Error('network unavailable')).mockResolvedValueOnce({
      rewardCode: makeRewardCode({ id: 8 }),
      plaintext: '',
      replayed: true,
    })
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getAllByRole('button', { name: '创建兑换码' }).at(-1)!)
    const nameInput = screen.getByRole('textbox', { name: '名称' })
    const codeInput = screen.getByRole('textbox', { name: 'Code' })
    expect(nameInput).toBeRequired()
    expect(codeInput).toBeRequired()
    expect(codeInput).toHaveAttribute('autocomplete', 'off')
    expect(codeInput).toHaveAttribute('spellcheck', 'false')

    await user.type(nameInput, '重试奖励')
    screen.getByRole('button', { name: '重新生成' }).focus()
    await user.keyboard('{Enter}')
    expect(resource.create).not.toHaveBeenCalled()

    const submittedCode = (codeInput as HTMLInputElement).value
    const form = nameInput.closest('form')
    expect(form).not.toBeNull()
    await fireEvent.submit(form!)
    await waitFor(() => expect(resource.create).toHaveBeenCalledTimes(1))

    await fireEvent.submit(form!)
    await waitFor(() => expect(resource.create).toHaveBeenCalledTimes(2))
    expect(resource.create.mock.calls[1]?.[0]).toEqual(resource.create.mock.calls[0]?.[0])
    expect(await screen.findByText(submittedCode)).toBeVisible()
  })

  it('atomically edits future rules and rotates when a new Code is entered', async () => {
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '编辑' }))

    expect(screen.getByRole('dialog', { name: '编辑兑换码' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: '名称' })).toHaveValue('新用户奖励')
    expect(screen.getByRole('textbox', { name: 'Code' })).toHaveValue('')
    expect(screen.getByText('0 表示无限；当前已兑换 12 次')).toBeVisible()

    await user.type(screen.getByRole('textbox', { name: 'Code' }), 'NEWC-ODE1-WXYZ')
    expect(screen.getByText('保存后旧 Code 将立即失效，新 Code 将立即生效。')).toBeVisible()
    await user.clear(screen.getByRole('textbox', { name: '名称' }))
    await user.type(screen.getByRole('textbox', { name: '名称' }), '八月奖励')
    await user.click(screen.getByRole('button', { name: '保存更改' }))

    await waitFor(() => expect(resource.update).toHaveBeenCalledTimes(1))
    expect(resource.update).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        displayName: '八月奖励',
        creditAmount: 80,
        maxRedemptions: 100,
        code: 'NEWC-ODE1-WXYZ',
      }),
    )
    expect(await screen.findByRole('dialog', { name: '保存新 Code' })).toBeVisible()
    expect(screen.getByText('NEWC-ODE1-WXYZ')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '我已保存' }))
    expect(screen.queryByText('NEWC-ODE1-WXYZ')).not.toBeInTheDocument()
  })

  it('opens and loads redemption records for the selected RewardCode through the HTTP seam', async () => {
    let requestedRewardCodeId = ''
    useMswHandlers(
      http.get('/api/admin/rewards/codes/:rewardCodeId/redemptions', ({ params, request }) => {
        requestedRewardCodeId = String(params.rewardCodeId)
        expect(new URL(request.url).searchParams.get('limit')).toBe('20')
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { redemptions: [makeRedemptionSummary()] },
        })
      }),
    )
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '兑换记录' }))

    expect(screen.getByRole('dialog', { name: '新用户奖励 · 兑换记录' })).toBeVisible()
    expect(await screen.findByText('用户 #23')).toBeVisible()
    expect(requestedRewardCodeId).toBe('7')
  })

  it('omits a blank Code from rule-only updates and does not show plaintext', async () => {
    resource.update.mockResolvedValueOnce({ rewardCode: makeRewardCode() })
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '编辑' }))
    await user.click(screen.getByRole('button', { name: '保存更改' }))

    await waitFor(() => expect(resource.update).toHaveBeenCalledTimes(1))
    expect(resource.update).toHaveBeenCalledWith(7, {
      displayName: '新用户奖励',
      creditAmount: 80,
      maxRedemptions: 100,
      expiresAt: '',
    })
    expect(screen.queryByRole('dialog', { name: '保存新 Code' })).not.toBeInTheDocument()
  })

  it('keeps the form and row unchanged when an atomic rule and Code update fails', async () => {
    resource.update.mockRejectedValueOnce(new Error('更新冲突'))
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '编辑' }))
    await user.clear(screen.getByRole('textbox', { name: '名称' }))
    await user.type(screen.getByRole('textbox', { name: '名称' }), '不会保存')
    await user.type(screen.getByRole('textbox', { name: 'Code' }), 'NEWC-ODE1-WXYZ')
    await user.click(screen.getByRole('button', { name: '保存更改' }))

    expect(await screen.findByText('更新冲突')).toBeVisible()
    expect(screen.getByRole('dialog', { name: '编辑兑换码' })).toBeVisible()
    expect(screen.getByText('新用户奖励')).toBeVisible()
    expect(screen.queryByRole('dialog', { name: '保存新 Code' })).not.toBeInTheDocument()
  })

  it('keeps the enabled intent stable while pending and after a failed Toggle', async () => {
    let rejectToggle: (error: Error) => void = () => undefined
    resource.setEnabled.mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectToggle = reject
      }),
    )
    const { user } = renderPage()
    await screen.findByText('新用户奖励')
    const toggle = screen.getByRole('switch', { name: '新用户奖励启用状态' })

    expect(toggle).toBeChecked()
    await user.click(toggle)
    expect(screen.getByRole('switch', { name: '新用户奖励启用状态' })).toBeChecked()
    expect(screen.getByRole('switch', { name: '新用户奖励启用状态' })).toBeDisabled()
    rejectToggle(new Error('network unavailable'))

    await waitFor(() =>
      expect(screen.getByRole('switch', { name: '新用户奖励启用状态' })).toBeEnabled(),
    )
    expect(screen.getByRole('switch', { name: '新用户奖励启用状态' })).toBeChecked()
    expect(toast.error).toHaveBeenCalledWith('network unavailable')
  })

  it('confirms deletion and preserves a row when the backend rejects a redeemed Code', async () => {
    resource.delete.mockRejectedValueOnce(
      Object.assign(new Error('已有兑换记录，只能停用'), { status: 409 }),
    )
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '删除' }))
    expect(confirm.request).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '删除兑换码',
        confirmLabel: '确认删除',
        intent: 'danger',
      }),
    )

    await confirm.request.mock.calls[0]![0].onConfirm()
    expect(resource.delete).toHaveBeenCalledWith(7)
    expect(screen.getByText('新用户奖励')).toBeVisible()
    expect(toast.error).toHaveBeenCalledWith('已有兑换记录，只能停用')
  })
})

function renderPage() {
  return renderWithUser(AdminRewardCodesPage, {
    global: { plugins: [[PrimeVue, primeVueTestOptions]] },
  })
}

function makeRewardCode(overrides: Record<string, unknown> = {}) {
  return {
    id: 7,
    displayName: '新用户奖励',
    maskedCodeHint: '•••• 2026',
    creditAmount: 80,
    maxRedemptions: 100,
    redemptionCount: 12,
    enabled: true,
    status: 'enabled',
    createdBy: 1,
    createdAt: '2026-07-20T10:00:00Z',
    updatedAt: '2026-07-20T10:00:00Z',
    ...overrides,
  }
}

function makeRedemptionSummary(overrides: Record<string, unknown> = {}) {
  return {
    id: 51,
    reward_code_id: 7,
    user_id: 23,
    credited_amount: 80,
    redeemed_at: '2026-07-22T03:04:05Z',
    ...overrides,
  }
}
