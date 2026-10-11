import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '../../../../../core/i18n'
import { loadLocaleMessages } from '../../../../../core/i18n/userLocale'
import { http, HttpResponse, useMswHandlers } from '../../../../../test/msw'
import { makeControlUserPayload } from '../../../../../test/factories'
import { mockAppToast, type AppToastMocks } from '../../../../../test/toast'
import { renderWithUser, screen, waitFor, within } from '../../../../../test/render'
import { expectDomOrder } from '../../../../../test/domOrder'
import AdminUsersPage from './AdminUsersPage.vue'

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))

let toastMocks: AppToastMocks
const userPublicID = 'usr_000000000000000000000012'
const secondUserPublicID = 'usr_000000000000000000000013'

vi.mock('../../../shared/session/feedback/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock }),
}))

vi.mock('../../../../../core/composables/useAppToast')

describe('AdminUsersPage', () => {
  beforeAll(async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
  })

  afterAll(() => {
    ;(i18n.global.locale as { value: string }).value = 'en'
  })

  let userListRequestCount: number
  let commandRequests: Array<Record<string, unknown>>

  beforeEach(() => {
    confirmMock.mockReset()
    confirmMock.mockResolvedValue(true)
    vi.clearAllMocks()
    toastMocks = mockAppToast()
    userListRequestCount = 0
    commandRequests = []
    useMswHandlers(
      http.get('/api/control/v1/users', () => {
        userListRequestCount += 1
        return HttpResponse.json({
          data: [
            {
              id: userPublicID,
              email: 'admin@example.com',
              providers: ['email'],
              display_name: 'admin_balance_user',
              avatar: '',
              role: 'user',
              status: 1,
              available_credits: 125,
              last_login_ip: '203.0.113.42',
              last_login_country: 'US',
              created_at: '2026-07-24T08:00:00Z',
              updated_at: '2026-07-24T08:00:00Z',
            },
          ],
          meta: { next_cursor: null, has_more: false },
        })
      }),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}`, () =>
        HttpResponse.json({
          data: {
            user_id: userPublicID,
            display_name: 'admin_balance_user',
            status: 'active',
            available_credits: 125,
            reserved_credits: 10,
            active_restrictions: [
              {
                restriction_id: 'restriction-1',
                user_id: userPublicID,
                display_name: 'admin_balance_user',
                type: 'ledger_integrity',
                source: 'reconciliation',
                reason: '需要人工复核',
                created_at: '2026-07-24T08:00:00Z',
                created_by: { type: 'system' },
                related_object: { type: 'run', id: 'run-1' },
              },
            ],
          },
        }),
      ),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}/ledger`, ({ request }) =>
        HttpResponse.json({
          data:
            new URL(request.url).searchParams.get('cursor') === 'cursor-page-2'
              ? [makeLedgerTransaction('charge-older', 'grant')]
              : [makeLedgerTransaction('charge-1', 'capture')],
          meta:
            new URL(request.url).searchParams.get('cursor') === 'cursor-page-2'
              ? { next_cursor: null, has_more: false }
              : { next_cursor: 'cursor-page-2', has_more: true },
        }),
      ),
      http.post('/api/control/v1/credits/grants', async ({ request }) => {
        commandRequests.push({
          body: (await request.json()) as Record<string, unknown>,
          idempotencyKey: request.headers.get('Idempotency-Key'),
          operatorReason: request.headers.get('X-Operator-Reason'),
        })
        return HttpResponse.json({
          data: {
            transaction_id: 'grant-1',
            command_type: 'grant',
            amount_credits: 20,
            replayed: false,
            created_at: '2026-07-24T08:00:00Z',
            wallet: {
              user_id: userPublicID,
              display_name: 'admin_balance_user',
              available_credits: 145,
              reserved_credits: 10,
            },
          },
        })
      }),
    )
  })

  it('opens the operational account overview from the available Idea balance', async () => {
    const { user } = renderPage()

    expect(await screen.findByRole('columnheader', { name: '账户余额' })).toBeVisible()
    const balanceTrigger = await screen.findByRole('button', {
      name: '打开 admin_balance_user 的账户详情',
    })
    expect(balanceTrigger).toHaveTextContent('125 Ideas')

    await user.click(balanceTrigger)

    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    expect(dialog).toBeVisible()
    expect(within(dialog).getByText('账户余额')).toBeVisible()
    expect(within(dialog).getByText('125 Ideas')).toBeVisible()
    expect(within(dialog).getByText('账户状态')).toBeVisible()
    expect(within(dialog).getByText('受限')).toBeVisible()
    expect(within(dialog).queryByText('无限制')).not.toBeInTheDocument()
    expect(within(dialog).getByText('需要人工复核')).toBeVisible()
  })

  it('shows the latest login country and IP in one column after Email', async () => {
    renderPage()

    const emailHeader = await screen.findByRole('columnheader', { name: 'Email' })
    const ipHeader = await screen.findByRole('columnheader', { name: 'IP' })
    expectDomOrder([emailHeader, ipHeader])
    expect(await screen.findByText('203.0.113.42')).toBeVisible()
    expect(await screen.findByText('🇺🇸')).toBeVisible()
  })

  it('keeps user columns in subject, attributes, status, time, then More-last order', async () => {
    renderPage()

    expect(await screen.findByText('203.0.113.42')).toBeVisible()
    const headers = ['用户', 'Email', 'IP', '角色', '账户余额', '状态', '加入时间', 'More'].map(
      (name) => screen.getByRole('columnheader', { name }),
    )
    expectDomOrder(headers)
  })

  it('loads more Ledger entries and refreshes the user list after confirmed Grant', async () => {
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })

    expect(
      await within(dialog).findByRole('button', { name: '退还 charge-1 的Idea' }),
    ).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: '加载更多 Ledger' }))
    expect(await within(dialog).findByText('历史赠送')).toBeVisible()

    await user.click(within(dialog).getByRole('button', { name: '赠送' }))
    await user.type(within(dialog).getByRole('spinbutton', { name: 'Idea数量' }), '20')
    await user.type(within(dialog).getByRole('textbox', { name: /操作原因/ }), '运营补偿')
    await user.click(within(dialog).getByRole('button', { name: '核对操作' }))
    await waitFor(() =>
      expect(confirmMock).toHaveBeenCalledWith(expect.objectContaining({ title: '确认赠送Idea' })),
    )

    await waitFor(() =>
      expect(commandRequests[0]).toMatchObject({
        body: {
          user_id: userPublicID,
          amount_credits: 20,
          reason: '运营补偿',
        },
        operatorReason: '运营补偿',
      }),
    )
    expect(commandRequests[0] as Record<string, unknown>).toMatchObject({
      idempotencyKey: expect.stringMatching(/^control-ui:grant:[A-Za-z0-9_-]+$/),
    })
    await waitFor(() => expect(userListRequestCount).toBeGreaterThan(1))
  })

  it('prioritizes frozen status and gives a recovery path when no restriction reason exists', async () => {
    useMswHandlers(
      http.get('/api/control/v1/users', () =>
        HttpResponse.json({
          data: [
            { ...makeControlUserPayload({ id: userPublicID, available_credits: 125 }), status: 0 },
          ],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}`, () =>
        HttpResponse.json({
          data: {
            user_id: userPublicID,
            display_name: 'admin_balance_user',
            status: 'frozen',
            available_credits: 125,
            reserved_credits: 10,
            active_restrictions: [],
          },
        }),
      ),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })

    expect(within(dialog).getByText('冻结')).toBeVisible()
    expect(
      within(dialog).getByText('用户当前已停用或正在注销，账户由身份生命周期冻结。'),
    ).toBeVisible()
  })

  it('keeps a zero balance actionable', async () => {
    useMswHandlers(
      http.get('/api/control/v1/users', () =>
        HttpResponse.json({
          data: [
            {
              id: userPublicID,
              display_name: 'admin_balance_user',
              avatar: '',
              role: 'user',
              status: 1,
              available_credits: 0,
              created_at: '2026-07-24T08:00:00Z',
              updated_at: '2026-07-24T08:00:00Z',
            },
          ],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
    )
    const { user } = renderPage()
    const balanceTrigger = await screen.findByRole('button', {
      name: '打开 admin_balance_user 的账户详情',
    })
    expect(balanceTrigger).toHaveTextContent('0 Ideas')
    expect(balanceTrigger).toBeEnabled()
    await user.click(balanceTrigger)
    expect(
      await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' }),
    ).toBeVisible()
  })

  it('confirms inline Deduction and refreshes the wallet, Ledger and table balance', async () => {
    let availableCredits = 125
    let hasDeduction = false
    useMswHandlers(
      http.get('/api/control/v1/users', () =>
        HttpResponse.json({
          data: [makeControlUserPayload({ id: userPublicID, available_credits: availableCredits })],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}`, () =>
        HttpResponse.json({
          data: {
            user_id: userPublicID,
            display_name: 'admin_balance_user',
            status: 'active',
            available_credits: availableCredits,
            reserved_credits: 0,
            active_restrictions: [],
          },
        }),
      ),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}/ledger`, () =>
        HttpResponse.json({
          data: hasDeduction
            ? [makeLedgerTransaction('deduction-1', 'deduction', '运营扣减')]
            : [makeLedgerTransaction('capture-1', 'capture')],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
      http.post('/api/control/v1/credits/deductions', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        expect(body).toMatchObject({
          user_id: userPublicID,
          amount_credits: 25,
          reason: '运营扣减',
        })
        expect(request.headers.get('Idempotency-Key')).toMatch(/^control-ui:deduction:/)
        expect(request.headers.get('X-Operator-Reason')).toBe('运营扣减')
        availableCredits = 100
        hasDeduction = true
        return HttpResponse.json({
          data: {
            transaction_id: 'deduction-1',
            command_type: 'deduction',
            amount_credits: 25,
            replayed: false,
            created_at: '2026-07-24T08:00:00Z',
            wallet: {
              user_id: userPublicID,
              display_name: 'admin_balance_user',
              available_credits: 100,
              reserved_credits: 0,
            },
          },
        })
      }),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    await user.click(within(dialog).getByRole('button', { name: '扣减' }))
    await user.type(within(dialog).getByRole('spinbutton', { name: 'Idea数量' }), '25')
    await user.type(within(dialog).getByRole('textbox', { name: /操作原因/ }), '运营扣减')
    await user.click(within(dialog).getByRole('button', { name: '核对操作' }))
    await waitFor(() =>
      expect(confirmMock).toHaveBeenCalledWith(expect.objectContaining({ title: '确认扣减Idea' })),
    )

    await waitFor(() => expect(within(dialog).getByText('100 Ideas')).toBeVisible())
    expect(await within(dialog).findByText('运营扣减')).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: '关闭' }))
    expect(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    ).toHaveTextContent('100 Ideas')
  })

  it('keeps the inline Deduction form and balances unchanged after a typed business failure', async () => {
    useMswHandlers(
      http.post('/api/control/v1/credits/deductions', () =>
        HttpResponse.json(
          { error: { code: 'billing_wallet_restricted', message: 'restricted', request_id: 'r1' } },
          { status: 403 },
        ),
      ),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    await user.click(within(dialog).getByRole('button', { name: '扣减' }))
    const amount = within(dialog).getByRole('spinbutton', { name: 'Idea数量' })
    const reason = within(dialog).getByRole('textbox', { name: /操作原因/ })
    await user.type(amount, '25')
    await user.type(reason, '受限账户扣减')
    await user.click(within(dialog).getByRole('button', { name: '核对操作' }))
    await waitFor(() =>
      expect(confirmMock).toHaveBeenCalledWith(expect.objectContaining({ title: '确认扣减Idea' })),
    )

    await waitFor(() => expect(toastMocks.error).toHaveBeenCalled())
    expect(amount).toHaveValue('25')
    expect(reason).toHaveValue('受限账户扣减')
    expect(within(dialog).getByText('125 Ideas')).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: '关闭' }))
    expect(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    ).toHaveTextContent('125 Ideas')
    expect(within(dialog).queryByText('受限账户扣减')).not.toBeInTheDocument()
  })

  it('locks the inline command controls while a Deduction is pending', async () => {
    let resolveDeduction: (() => void) | undefined
    let deductionRequests = 0
    useMswHandlers(
      http.post('/api/control/v1/credits/deductions', () => {
        deductionRequests += 1
        return new Promise((resolve) => {
          resolveDeduction = () =>
            resolve(
              HttpResponse.json({
                data: {
                  transaction_id: 'deduction-1',
                  command_type: 'deduction',
                  amount_credits: 25,
                  replayed: false,
                  created_at: '2026-07-24T08:00:00Z',
                  wallet: {
                    user_id: userPublicID,
                    display_name: 'admin_balance_user',
                    available_credits: 100,
                    reserved_credits: 0,
                  },
                },
              }),
            )
        })
      }),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    await user.click(within(dialog).getByRole('button', { name: '扣减' }))
    await user.type(within(dialog).getByRole('spinbutton', { name: 'Idea数量' }), '25')
    await user.type(within(dialog).getByRole('textbox', { name: /操作原因/ }), '等待扣减')
    await user.click(within(dialog).getByRole('button', { name: '核对操作' }))

    await waitFor(() => expect(deductionRequests).toBe(1))
    expect(within(dialog).getByRole('spinbutton', { name: 'Idea数量' })).toBeDisabled()
    expect(within(dialog).getByRole('textbox', { name: /操作原因/ })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: '核对操作' })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: '赠送' })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: '扣减' })).toBeDisabled()
    expect(within(dialog).getByRole('button', { name: '取消' })).toBeDisabled()
    expect(within(dialog).queryByRole('button', { name: '关闭' })).not.toBeInTheDocument()

    resolveDeduction?.()
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '赠送' })).toBeEnabled())
    expect(deductionRequests).toBe(1)
  })

  it('only offers Refund for an eligible Charge and validates its remaining amount', async () => {
    let refundRequests = 0
    useMswHandlers(
      http.get(`/api/control/v1/credits/wallets/${userPublicID}/ledger`, () =>
        HttpResponse.json({
          data: [
            makeLedgerTransaction('charge-eligible', 'capture', '可退款 Charge', 5),
            makeLedgerTransaction('charge-spent', 'capture', '已退完 Charge', 0),
            makeLedgerTransaction('grant-1', 'grant'),
          ],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
      http.post('/api/control/v1/credits/refunds', () => {
        refundRequests += 1
        return HttpResponse.json({ data: {} })
      }),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    expect(
      within(dialog).getByRole('button', { name: '退还 charge-eligible 的Idea' }),
    ).toBeVisible()
    expect(
      within(dialog).queryByRole('button', { name: '退还 charge-spent 的Idea' }),
    ).not.toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: '退还 charge-eligible 的Idea' }))
    expect(within(dialog).getByText(/原 Charge：charge-eligible，剩余可退 5 Ideas/)).toBeVisible()
    await user.type(within(dialog).getByRole('spinbutton', { name: 'Idea数量' }), '6')
    await user.type(within(dialog).getByRole('textbox', { name: /退款原因/ }), '超额退款')
    await user.click(within(dialog).getByRole('button', { name: '核对退款' }))
    expect(within(dialog).getByText('退款不能超过可退余额 5 Ideas')).toBeVisible()
    expect(refundRequests).toBe(0)
  })

  it('submits a linked Refund and refreshes the wallet, list and original Charge remaining amount', async () => {
    let availableCredits = 125
    let refunded = false
    useMswHandlers(
      http.get('/api/control/v1/users', () =>
        HttpResponse.json({
          data: [makeControlUserPayload({ id: userPublicID, available_credits: availableCredits })],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}`, () =>
        HttpResponse.json({
          data: {
            user_id: userPublicID,
            display_name: 'admin_balance_user',
            status: 'active',
            available_credits: availableCredits,
            reserved_credits: 0,
            active_restrictions: [],
          },
        }),
      ),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}/ledger`, () =>
        HttpResponse.json({
          data: refunded
            ? [
                makeLedgerTransaction('refund-1', 'refund', '任务取消退款'),
                makeLedgerTransaction('charge-1', 'capture', 'AI image completed', 15),
              ]
            : [makeLedgerTransaction('charge-1', 'capture', 'AI image completed', 25)],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
      http.post('/api/control/v1/credits/refunds', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        expect(body).toMatchObject({
          user_id: userPublicID,
          charge_transaction_id: 'charge-1',
          amount_credits: 10,
          reason: '任务取消退款',
        })
        expect(request.headers.get('Idempotency-Key')).toMatch(/^control-ui:refund:/)
        expect(request.headers.get('X-Operator-Reason')).toBe('任务取消退款')
        availableCredits = 135
        refunded = true
        return HttpResponse.json({
          data: {
            transaction_id: 'refund-1',
            command_type: 'refund',
            amount_credits: 10,
            replayed: false,
            created_at: '2026-07-24T08:00:00Z',
            wallet: {
              user_id: userPublicID,
              display_name: 'admin_balance_user',
              available_credits: 135,
              reserved_credits: 0,
            },
          },
        })
      }),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    await user.click(await within(dialog).findByRole('button', { name: '退还 charge-1 的Idea' }))
    await user.type(within(dialog).getByRole('spinbutton', { name: 'Idea数量' }), '10')
    await user.type(within(dialog).getByRole('textbox', { name: /退款原因/ }), '任务取消退款')
    await user.click(within(dialog).getByRole('button', { name: '核对退款' }))
    await waitFor(() =>
      expect(confirmMock).toHaveBeenCalledWith(expect.objectContaining({ title: '确认退款Idea' })),
    )

    await waitFor(() => expect(within(dialog).getByText('135 Ideas')).toBeVisible())
    const refundRow = (await within(dialog).findByText('任务取消退款')).closest('tr')
    expect(refundRow).not.toBeNull()
    expect(within(refundRow as HTMLElement).getByText('退款')).toBeVisible()
    expect(within(dialog).getByText('可退 15 Ideas')).toBeVisible()
    await user.click(within(dialog).getByRole('button', { name: '关闭' }))
    expect(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    ).toHaveTextContent('135 Ideas')
  })

  it('uses a 25-row cursor, appends without duplicates, and resets Ledger state for another user', async () => {
    const ledgerRequests: Array<{
      userId: string
      cursor: string | null
      pageSize: string | null
    }> = []
    useMswHandlers(
      http.get('/api/control/v1/users', () =>
        HttpResponse.json({
          data: [
            makeControlUserPayload({ id: userPublicID, available_credits: 125 }),
            {
              ...makeControlUserPayload({ id: userPublicID, available_credits: 55 }),
              id: secondUserPublicID,
              display_name: 'another_balance_user',
            },
          ],
          meta: { next_cursor: null, has_more: false },
        }),
      ),
      http.get('/api/control/v1/credits/wallets/:userId', ({ params }) =>
        HttpResponse.json({
          data: {
            user_id: String(params.userId),
            display_name:
              String(params.userId) === userPublicID
                ? 'admin_balance_user'
                : 'another_balance_user',
            status: 'active',
            available_credits: params.userId === userPublicID ? 125 : 55,
            reserved_credits: 0,
            active_restrictions: [],
          },
        }),
      ),
      http.get('/api/control/v1/credits/wallets/:userId/ledger', ({ params, request }) => {
        const url = new URL(request.url)
        ledgerRequests.push({
          userId: String(params.userId),
          cursor: url.searchParams.get('cursor'),
          pageSize: url.searchParams.get('page_size'),
        })
        if (params.userId !== userPublicID) {
          return HttpResponse.json({
            data: [makeLedgerTransaction('user-13-ledger', 'grant', '第二个用户流水')],
            meta: { next_cursor: null, has_more: false },
          })
        }
        const cursor = url.searchParams.get('cursor')
        return HttpResponse.json({
          data:
            cursor === 'ledger-cursor-2'
              ? [
                  makeLedgerTransaction('charge-1', 'capture'),
                  makeLedgerTransaction('charge-older', 'grant', '历史赠送'),
                ]
              : [
                  makeLedgerTransaction('charge-1', 'capture'),
                  makeLedgerTransaction('grant-current', 'grant', '当前赠送'),
                ],
          meta:
            cursor === 'ledger-cursor-2'
              ? { next_cursor: null, has_more: false }
              : { next_cursor: 'ledger-cursor-2', has_more: true },
        })
      }),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const firstDialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    await within(firstDialog).findByText('当前赠送')
    await user.click(within(firstDialog).getByRole('button', { name: '加载更多 Ledger' }))
    await within(firstDialog).findByText('历史赠送')

    expect(
      ledgerRequests
        .filter((request) => request.userId === userPublicID)
        .every((request) => request.pageSize === '25'),
    ).toBe(true)
    expect(within(firstDialog).getAllByText('AI image completed')).toHaveLength(1)

    await user.click(within(firstDialog).getByRole('button', { name: '关闭' }))
    await waitFor(() =>
      expect(
        screen.queryByRole('dialog', { name: 'admin_balance_user 的Idea账户' }),
      ).not.toBeInTheDocument(),
    )
    await user.click(screen.getByRole('button', { name: '打开 another_balance_user 的账户详情' }))
    const secondDialog = await screen.findByRole('dialog', {
      name: 'another_balance_user 的Idea账户',
    })
    expect(await within(secondDialog).findByText('第二个用户流水')).toBeVisible()
    expect(within(secondDialog).queryByText('当前赠送')).not.toBeInTheDocument()
    expect(within(secondDialog).queryByText('历史赠送')).not.toBeInTheDocument()
    expect(
      within(secondDialog).queryByRole('button', { name: '加载更多 Ledger' }),
    ).not.toBeInTheDocument()
  })

  it('reports wallet and Ledger load failures through toast only and retries loading', async () => {
    let shouldFail = true
    useMswHandlers(
      http.get(`/api/control/v1/credits/wallets/${userPublicID}`, () =>
        shouldFail
          ? HttpResponse.json(
              { error: { code: 'service_unavailable', message: 'down', request_id: 'r1' } },
              { status: 500 },
            )
          : HttpResponse.json({
              data: {
                user_id: userPublicID,
                display_name: 'admin_balance_user',
                status: 'active',
                available_credits: 125,
                reserved_credits: 0,
                active_restrictions: [],
              },
            }),
      ),
      http.get(`/api/control/v1/credits/wallets/${userPublicID}/ledger`, () =>
        shouldFail
          ? HttpResponse.json(
              { error: { code: 'service_unavailable', message: 'down', request_id: 'r2' } },
              { status: 500 },
            )
          : HttpResponse.json({
              data: [makeLedgerTransaction('recovered-ledger', 'grant', '恢复后的流水')],
              meta: { next_cursor: null, has_more: false },
            }),
      ),
    )
    const { user } = renderPage()
    await user.click(
      await screen.findByRole('button', { name: '打开 admin_balance_user 的账户详情' }),
    )
    const dialog = await screen.findByRole('dialog', { name: 'admin_balance_user 的Idea账户' })
    // Toast-only contract: load failures surface via toast, never an inline alert.
    await waitFor(() => expect(toastMocks.error).toHaveBeenCalled())
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
    expect(within(dialog).queryByText('账户或流水加载失败。请刷新后重试。')).not.toBeInTheDocument()

    shouldFail = false
    await user.click(within(dialog).getByRole('button', { name: '刷新' }))
    expect(await within(dialog).findByText('恢复后的流水')).toBeVisible()
    expect(within(dialog).queryByRole('alert')).not.toBeInTheDocument()
  })
})

function makeLedgerTransaction(
  transactionId: string,
  commandType: 'capture' | 'grant' | 'deduction' | 'refund',
  reason?: string,
  remainingRefundableCredits = 25,
) {
  return {
    sequence_id: transactionId === 'charge-1' ? 100 : 80,
    transaction_id: transactionId,
    command_type: commandType,
    amount_credits: 25,
    actor: { type: 'system', id: 'task-worker' },
    source: 'tasking',
    reason: reason ?? (transactionId === 'charge-older' ? '历史赠送' : 'AI image completed'),
    business_reference: { type: 'task', id: 'task-77' },
    available_credits_after: 125,
    reserved_credits_after: 10,
    created_at: '2026-07-24T08:00:00Z',
    postings: [],
    ...(commandType === 'capture'
      ? {
          charge: {
            amount_credits: 25,
            refunded_credits: 25 - remainingRefundableCredits,
            remaining_refundable_credits: remainingRefundableCredits,
          },
        }
      : {}),
  }
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return renderWithUser(AdminUsersPage, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      stubs: { Refresh: true, Search: true, Settings: true },
    },
  })
}
