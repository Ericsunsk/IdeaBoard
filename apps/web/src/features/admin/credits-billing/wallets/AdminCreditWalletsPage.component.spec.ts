import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminCreditWalletsPage from './AdminCreditWalletsPage.vue'

const confirmRequestMock = vi.hoisted(() => vi.fn())
const toastMocks = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
}))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ request: confirmRequestMock }),
}))

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => toastMocks,
}))

describe('AdminCreditWalletsPage', () => {
  let walletRequestCount: number
  let ledgerRequestCount: number
  let commandRequests: Array<{
    kind: 'grant' | 'deduction' | 'refund'
    body: Record<string, unknown>
  }>

  beforeEach(() => {
    confirmRequestMock.mockReset()
    vi.clearAllMocks()
    walletRequestCount = 0
    ledgerRequestCount = 0
    commandRequests = []
    useMswHandlers(
      http.get('/api/admin/users', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { total: 0, active_count: 0, banned_count: 0, admin_count: 0, list: [] },
        }),
      ),
      http.get('/api/admin/credits/wallets/12', () => {
        walletRequestCount += 1
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { user_id: 12, status: 'active', available_credits: 1200, reserved_credits: 25 },
        })
      }),
      http.get('/api/admin/credits/wallets/12/ledger', () => {
        ledgerRequestCount += 1
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            transactions: [makeCharge()],
          },
        })
      }),
      makeCommandHandler('grant', '/api/admin/credits/grants'),
      makeCommandHandler('deduction', '/api/admin/credits/deductions'),
      makeCommandHandler('refund', '/api/admin/credits/refunds'),
    )
  })

  it('opens a Wallet by user ID and exposes Refund only on a refundable Charge row', async () => {
    const { user } = renderPage()
    const userIdInput = screen.getByRole('spinbutton', { name: '用户 ID' })
    await user.type(userIdInput, '12')
    await user.click(screen.getByRole('button', { name: '打开 Wallet' }))

    expect(await screen.findByText('1,200 Ideas')).toBeVisible()
    expect(screen.getByText('冻结 25 Ideas')).toBeVisible()
    expect(screen.getByText('已退 5 Ideas · 可退 20 Ideas')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '退款' }))
    expect(screen.getByText('原 Charge')).toBeVisible()
    expect(screen.getByText('剩余可退：20 Ideas')).toBeVisible()
    expect(screen.getByText('charge-1')).toBeVisible()
  })

  it('confirms auditable Grant, Deduction, and Refund commands and refreshes Wallet data', async () => {
    const { user } = renderPage()
    await openWallet(user)

    await submitCommand(user, {
      openButton: '赠送',
      amount: '40',
      reason: '活动补偿',
      confirmTitle: '确认赠送Idea',
    })
    await executeLatestConfirmation()

    expect(commandRequests[0]).toMatchObject({
      kind: 'grant',
      body: {
        user_id: 12,
        amount_credits: 40,
        reason: '活动补偿',
      },
    })
    expect(commandRequests[0]?.body.idempotency_key).toMatch(/^admin-ui:grant:/)
    await expectWalletInvalidation(2)

    await submitCommand(user, {
      openButton: '扣减',
      amount: '7',
      reason: '误发回收',
      confirmTitle: '确认扣减Idea',
    })
    await executeLatestConfirmation()

    expect(commandRequests[1]).toMatchObject({
      kind: 'deduction',
      body: {
        user_id: 12,
        amount_credits: 7,
        reason: '误发回收',
      },
    })
    expect(commandRequests[1]?.body.idempotency_key).toMatch(/^admin-ui:deduction:/)
    await expectWalletInvalidation(3)

    await submitCommand(user, {
      openButton: '退款',
      amount: '3',
      reason: '任务失败退还',
      confirmTitle: '确认退款Idea',
      submitButton: '核对退款',
    })
    await executeLatestConfirmation()

    expect(commandRequests[2]).toMatchObject({
      kind: 'refund',
      body: {
        user_id: 12,
        charge_transaction_id: 'charge-1',
        amount_credits: 3,
        reason: '任务失败退还',
      },
    })
    expect(commandRequests[2]?.body.idempotency_key).toMatch(/^admin-ui:refund:/)
    await expectWalletInvalidation(4)

    expect(toastMocks.success.mock.calls.map(([message]) => message)).toEqual([
      '赠送成功',
      '扣减成功',
      '退款成功',
    ])
    expect(toastMocks.error).not.toHaveBeenCalled()
  })

  function makeCommandHandler(kind: 'grant' | 'deduction' | 'refund', path: string) {
    return http.post(path, async ({ request }) => {
      commandRequests.push({
        kind,
        body: (await request.json()) as Record<string, unknown>,
      })
      return HttpResponse.json({
        code: 0,
        msg: 'ok',
        data: { transaction_id: `${kind}-transaction`, replayed: false },
      })
    })
  }

  async function expectWalletInvalidation(expectedRequestCount: number) {
    await waitFor(() => {
      expect(walletRequestCount).toBeGreaterThanOrEqual(expectedRequestCount)
      expect(ledgerRequestCount).toBeGreaterThanOrEqual(expectedRequestCount)
    })
  }
})

async function openWallet(user: ReturnType<typeof renderPage>['user']) {
  await user.type(screen.getByRole('spinbutton', { name: '用户 ID' }), '12')
  await user.click(screen.getByRole('button', { name: '打开 Wallet' }))
  expect(await screen.findByText('1,200 Ideas')).toBeVisible()
}

async function submitCommand(
  user: ReturnType<typeof renderPage>['user'],
  input: {
    openButton: '赠送' | '扣减' | '退款'
    amount: string
    reason: string
    confirmTitle: string
    submitButton?: '下一步：确认' | '核对退款'
  },
) {
  await user.click(screen.getByRole('button', { name: input.openButton }))
  await user.type(screen.getByRole('spinbutton', { name: 'Idea数量' }), input.amount)
  await user.type(screen.getByRole('textbox', { name: /^操作原因/ }), input.reason)
  await user.click(screen.getByRole('button', { name: input.submitButton ?? '下一步：确认' }))

  expect(confirmRequestMock).toHaveBeenLastCalledWith(
    expect.objectContaining({
      title: input.confirmTitle,
      message: expect.stringContaining(input.reason),
    }),
  )
}

async function executeLatestConfirmation() {
  const request = confirmRequestMock.mock.calls.at(-1)?.[0]
  expect(request).toBeDefined()
  await request.onConfirm()
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return renderWithUser(AdminCreditWalletsPage, {
    global: {
      plugins: [
        [PrimeVue, { unstyled: true }],
        [VueQueryPlugin, { queryClient }],
      ],
      stubs: { Refresh: true, NavArrowLeft: true, NavArrowRight: true },
    },
  })
}

function makeCharge() {
  return {
    sequence_id: 4,
    transaction_id: 'charge-1',
    command_type: 'capture',
    amount_credits: 25,
    actor: { type: 'system', id: 'task-worker' },
    source: 'tasking',
    reason: 'AI image completed',
    business_reference: { type: 'task', id: 'task-77' },
    related_transaction_id: 'hold-1',
    available_credits_after: 1200,
    reserved_credits_after: 25,
    created_at: '2026-07-12T08:00:00Z',
    postings: [
      { account_role: 'user_reserved', amount_credits: -25 },
      { account_role: 'system_revenue', amount_credits: 25 },
    ],
    charge: { amount_credits: 25, refunded_credits: 5, remaining_refundable_credits: 20 },
  }
}
