import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent } from '@testing-library/vue'
import { i18n } from '@/core/i18n'
import { loadLocaleMessages } from '@/core/i18n/userLocale'
import { renderWithUser, screen, waitFor } from '@/test/render'
import { mockAppToast, expectToastFeedback, type AppToastMocks } from '@/test/toast'
import AdminRewardCodesPage from './AdminRewardCodesPage.vue'
import { REWARD_CODES_MESSAGES } from './messages'
const resource = vi.hoisted(() => ({
  create: vi.fn(),
  delete: vi.fn(),
  search: vi.fn(),
  setEnabled: vi.fn(),
  update: vi.fn(),
}))
const confirm = vi.hoisted(() => ({
  confirm: vi.fn().mockResolvedValue(true),
}))

let toast: AppToastMocks
const redemptionsResource = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
}))

vi.mock('../../session/rewards/resource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../session/rewards/resource')>()),
  createAdminRewardCodeResource: () => resource,
}))

vi.mock('../../session/rewards/rewardCodeRedemptionResource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../session/rewards/rewardCodeRedemptionResource')>()),
  createRewardCodeRedemptionResource: () => redemptionsResource,
}))

vi.mock('../../../../../core/composables/useAppToast')

// Route clipboard access to the spec-controlled API so success and failure stay deterministic.
vi.mock('../../../../../core/composables/useAppClipboard', () => ({
  useAppClipboard: () => ({
    copyText: (text: string) => {
      const clipboard = (
        globalThis as {
          navigator?: { clipboard?: { writeText?: (value: string) => Promise<void> } }
        }
      ).navigator?.clipboard
      if (!text || !clipboard?.writeText) return Promise.resolve(false)
      return clipboard.writeText(text).then(
        () => true,
        () => false,
      )
    },
  }),
}))

vi.mock('@/core/composables/useConfirmDialog', () => ({
  useConfirm: () => confirm,
}))

describe('AdminRewardCodesPage', () => {
  beforeAll(async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
  })

  afterAll(() => {
    ;(i18n.global.locale as { value: string }).value = 'en'
  })

  beforeEach(() => {
    vi.clearAllMocks()
    toast = mockAppToast()
    resource.search.mockResolvedValue({
      rewardCodes: [makeRewardCode()],
      total: 21,
      nextBeforeId: 6,
    })
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
      expect(resource.search).toHaveBeenLastCalledWith(expect.objectContaining({ beforeId: 6 })),
    )
  })

  it('keeps the table headers mounted for the empty state', async () => {
    resource.search.mockResolvedValueOnce({ rewardCodes: [], total: 0 })
    renderPage()

    await waitFor(() => expect(screen.getByText('暂无兑换码')).toBeVisible())
    expect(screen.getByRole('table', { name: '兑换码列表' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '兑换码' })).toBeVisible()
    expect(
      screen.queryByText('创建第一个兑换码后，用户即可在奖励中心兑换 Idea。'),
    ).not.toBeInTheDocument()
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

  it('clears the status filter back to all from inside the select', async () => {
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

    await user.click(screen.getByRole('combobox'))
    await user.click(await screen.findByRole('option', { name: '全部状态' }))
    await waitFor(() =>
      expect(resource.search).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: undefined, beforeId: undefined }),
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

  it('routes an initial error to toast and recovers through the toolbar refresh', async () => {
    resource.search.mockRejectedValueOnce(new Error('network unavailable'))
    const { user } = renderPage()
    // 初载行内锚点已删除：失败只走 toast，空态表格 + 工具栏刷新即重试入口。
    await expectToastFeedback(toast, 'network unavailable', screen, ['兑换码加载失败，请重试。'])
    expect(screen.queryByRole('table', { name: '兑换码列表' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '刷新兑换码' }))
    expect(await screen.findByText('新用户奖励')).toBeVisible()

    resource.search.mockRejectedValueOnce(new Error('network unavailable'))
    await user.click(screen.getByRole('button', { name: '刷新兑换码' }))
    expect(await screen.findByText('刷新兑换码失败，正在显示上次成功的数据。')).toBeVisible()
    expect(screen.getByText('新用户奖励')).toBeVisible()
  })

  it('keeps the table frame mounted while the initial collection request is pending', async () => {
    resource.search.mockReturnValueOnce(new Promise(() => {}))
    renderPage()

    expect(await screen.findByRole('table', { name: '兑换码列表' })).toBeVisible()
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
    await expectToastFeedback(toast, REWARD_CODES_MESSAGES.plaintextCopyFailed, screen, [
      REWARD_CODES_MESSAGES.plaintextCopyFailed,
    ])
    await user.click(screen.getByRole('button', { name: '复制' }))
    expect(screen.getByRole('button', { name: '已复制' })).toBeVisible()
    expect(screen.queryByText(REWARD_CODES_MESSAGES.plaintextCopyFailed)).not.toBeInTheDocument()

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

  it('uses the native date-time input for RewardCode expiry', async () => {
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '创建兑换码' }))
    const expiresAt = screen.getByPlaceholderText('永久有效')
    expect(expiresAt).toHaveAttribute('type', 'datetime-local')
    expect(expiresAt).toHaveAttribute('id', 'reward-code-expires-at')
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
    await user.click(screen.getByRole('button', { name: '保存' }))

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
    expect(confirm.confirm).not.toHaveBeenCalled()
    expect(await screen.findByRole('dialog', { name: '保存新 Code' })).toBeVisible()
    expect(screen.getByText('NEWC-ODE1-WXYZ')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '我已保存' }))
    expect(screen.queryByText('NEWC-ODE1-WXYZ')).not.toBeInTheDocument()
  })

  it('routes redemption history load failures to toast without an inline error', async () => {
    redemptionsResource.list.mockRejectedValueOnce(new Error('control unavailable'))
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '兑换记录' }))

    expect(screen.getByRole('dialog', { name: '新用户奖励 · 兑换记录' })).toBeVisible()
    await expectToastFeedback(toast, 'control unavailable', screen, ['兑换记录加载失败，请重试。'])
    expect(screen.queryByText('用户 #23')).not.toBeInTheDocument()
  })

  it('omits a blank Code from rule-only updates and does not show plaintext', async () => {
    resource.update.mockResolvedValueOnce({ rewardCode: makeRewardCode() })
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '编辑' }))
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(resource.update).toHaveBeenCalledTimes(1))
    expect(resource.update).toHaveBeenCalledWith(7, {
      displayName: '新用户奖励',
      creditAmount: 80,
      maxRedemptions: 100,
      expiresAt: '',
    })
    expect(confirm.confirm).not.toHaveBeenCalled()
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
    await user.click(screen.getByRole('button', { name: '保存' }))

    // 保存失败只走 toast（对话框保持打开可重试），不再写行内。
    // 编辑框内常驻轮换 warning alert 与本次失败无关，这里只断言失败文案无行内残留。
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('更新冲突'))
    expect(screen.queryByText('更新冲突')).not.toBeInTheDocument()
    expect(screen.getByRole('dialog', { name: '编辑兑换码' })).toBeVisible()
    expect(screen.getByRole('textbox', { name: '名称' })).toHaveValue('不会保存')
    expect(screen.getByText('新用户奖励')).toBeVisible()
    expect(screen.queryByRole('dialog', { name: '保存新 Code' })).not.toBeInTheDocument()
  })

  it('flips the switch optimistically while pending and rolls back on Toggle failure', async () => {
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
    expect(screen.getByText('可兑换')).toBeVisible()
    await user.click(toggle)
    await waitFor(() => expect(resource.setEnabled).toHaveBeenCalledWith(7, false))
    expect(confirm.confirm).not.toHaveBeenCalled()
    // 乐观 intent 同 tick 翻转：同一 DOM 节点即时 unchecked + disabled，
    // Status 胶囊仍跟服务端（可兑换），不随开关预翻。
    const pendingToggle = screen.getByRole('switch', { name: '新用户奖励启用状态' })
    expect(pendingToggle).toBe(toggle)
    expect(pendingToggle).not.toBeChecked()
    expect(pendingToggle).toBeDisabled()
    expect(screen.getByText('可兑换')).toBeVisible()
    expect(screen.queryByText('已停用')).not.toBeInTheDocument()
    rejectToggle(new Error('network unavailable'))

    await waitFor(() =>
      expect(screen.getByRole('switch', { name: '新用户奖励启用状态' })).toBeEnabled(),
    )
    const recoveredToggle = screen.getByRole('switch', { name: '新用户奖励启用状态' })
    expect(recoveredToggle).toBe(toggle)
    expect(recoveredToggle).toBeChecked()
    expect(screen.getByText('可兑换')).toBeVisible()
    await expectToastFeedback(toast, 'network unavailable')
  })

  it('keeps the same Switch DOM node across a successful Toggle without remounting', async () => {
    // 初载停用行（unchecked），后台 refetch 回填启用行，避免默认 mock 覆盖。
    let resolveToggle!: (value: unknown) => void
    resource.search
      .mockResolvedValueOnce({
        rewardCodes: [makeRewardCode({ enabled: false, status: 'disabled' })],
        total: 21,
        nextBeforeId: 6,
      })
      .mockResolvedValue({
        rewardCodes: [makeRewardCode({ enabled: true, status: 'enabled' })],
        total: 21,
        nextBeforeId: 6,
      })
    resource.setEnabled.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveToggle = resolve
      }),
    )
    const { user } = renderPage()
    await screen.findByText('新用户奖励')
    const toggle = screen.getByRole('switch', { name: '新用户奖励启用状态' })
    expect(toggle).not.toBeChecked()
    expect(screen.getByText('已停用')).toBeVisible()

    await user.click(toggle)
    await waitFor(() => expect(resource.setEnabled).toHaveBeenCalledWith(7, true))
    // 乐观链路同一 DOM 节点即时翻转 checked（不等服务端），胶囊仍跟服务端。
    const pending = screen.getByRole('switch', { name: '新用户奖励启用状态' })
    expect(pending).toBe(toggle)
    expect(pending).toBeChecked()
    expect(pending).toBeDisabled()
    expect(screen.getByText('已停用')).toBeVisible()

    resolveToggle(makeRewardCode({ enabled: true, status: 'enabled' }))
    // 成功后同一节点保持 checked，胶囊跟服务端翻为可兑换，不重建。
    const toggled = await screen.findByRole('switch', { name: '新用户奖励启用状态' })
    expect(toggled).toBe(toggle)
    await waitFor(() => expect(toggled).toBeChecked())
    await waitFor(() => expect(toggled).toBeEnabled())
    expect(await screen.findByText('可兑换')).toBeVisible()
  })

  it('pins the StatusPill width so the Switch never shifts with the status text', async () => {
    renderPage()
    await screen.findByText('新用户奖励')

    const pill = screen.getByText('可兑换').parentElement
    // SwitchRow：68px status 槽锁宽（style），胶囊同宽居中，开关固定位。
    expect(screen.getByTestId('switch-row-status-slot').style.minWidth).toBe('68px')
    expect(pill?.style.minWidth).toBe('68px')
    expect(pill).toHaveClass('justify-center')
    expect(screen.getByRole('switch', { name: '新用户奖励启用状态' })).toBeVisible()
  })

  it('confirms deletion and preserves a row when the backend rejects a redeemed Code', async () => {
    resource.delete.mockRejectedValueOnce(
      Object.assign(new Error('已有兑换记录，只能停用'), { status: 409 }),
    )
    const { user } = renderPage()
    await screen.findByText('新用户奖励')

    await user.click(screen.getByRole('button', { name: '新用户奖励 操作菜单' }))
    await user.click(await screen.findByRole('button', { name: '删除' }))
    expect(confirm.confirm).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '删除兑换码',
        confirmLabel: '确认删除',
        intent: 'danger',
      }),
    )

    await waitFor(() => expect(resource.delete).toHaveBeenCalledWith(7))
    expect(screen.getByText('新用户奖励')).toBeVisible()
    expect(toast.error).toHaveBeenCalledWith('已有兑换记录，只能停用')
  })
})

function renderPage() {
  // Fresh client per render: the list query caches by key, so sharing one
  // across tests would serve stale pages and hide `resource.search` calls.
  const queryClient = new QueryClient()
  return renderWithUser(AdminRewardCodesPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
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
    createdAt: '2026-07-20T10:00:00Z',
    updatedAt: '2026-07-20T10:00:00Z',
    ...overrides,
  }
}
