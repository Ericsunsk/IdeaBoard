import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import AdminCreditReconciliationPage from './AdminCreditReconciliationPage.vue'

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

describe('AdminCreditReconciliationPage', () => {
  let findingQueries: Array<{ status: string | null; beforeId: string | null }>
  let startRequests: unknown[]
  let runDetailShouldFail: boolean
  let findingsShouldFail: boolean
  let startBusinessError: boolean
  let runsRequestCount: number
  let runResponse: Record<string, unknown>

  beforeEach(() => {
    confirmRequestMock.mockReset()
    findingQueries = []
    startRequests = []
    runDetailShouldFail = false
    findingsShouldFail = false
    startBusinessError = false
    runsRequestCount = 0
    runResponse = makeRun()
    vi.clearAllMocks()
    useMswHandlers(
      http.get('/api/admin/credits/reconciliation/runs', () => {
        runsRequestCount += 1
        return HttpResponse.json({ code: 0, msg: 'ok', data: { runs: [runResponse] } })
      }),
      http.post('/api/admin/credits/reconciliation/runs', async ({ request }) => {
        startRequests.push(await request.json())
        if (startBusinessError) {
          return HttpResponse.json({ code: 503, msg: 'api key invalid' })
        }
        return HttpResponse.json({ code: 0, msg: 'ok', data: runResponse })
      }),
      http.get('/api/admin/credits/reconciliation/runs/7', () => {
        if (runDetailShouldFail) {
          return HttpResponse.json({ code: 503, msg: 'run detail unavailable' }, { status: 503 })
        }
        return HttpResponse.json({ code: 0, msg: 'ok', data: runResponse })
      }),
      http.get('/api/admin/credits/reconciliation/runs/7/findings', ({ request }) => {
        if (findingsShouldFail) {
          return HttpResponse.json({ code: 503, msg: 'findings unavailable' }, { status: 503 })
        }
        const query = new URL(request.url).searchParams
        const status = query.get('status')
        const beforeId = query.get('before_id')
        findingQueries.push({ status, beforeId })
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: {
            findings: [
              status === 'resolved'
                ? makeFinding({
                    id: 92,
                    status: 'resolved',
                    subject_id: 'refund-2',
                    details: { task_id: 'task-88', transaction_id: 'refund-2' },
                    isolated: false,
                  })
                : makeFinding(),
            ],
            next_before_id: beforeId === null ? 91 : 81,
          },
        })
      }),
    )
  })

  it('shows report status, linked anomalies, isolation state, and requires confirmation to start', async () => {
    const { user } = renderPage()

    expect(await screen.findByText('#7')).toBeVisible()
    await user.click(screen.getByText('#7'))

    expect(await screen.findByText('ledger_balance_mismatch')).toBeVisible()
    expect(screen.getByText('Wallet · User #12')).toBeVisible()
    expect(screen.getByText('credit_transaction · charge-1')).toBeVisible()
    expect(screen.getByText('task_id: task-77 · transaction_id: charge-1')).toBeVisible()
    expect(screen.getByText('已隔离')).toBeVisible()
    expect(screen.getAllByText('月度例行对账').length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: /自动修复/ })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '启动对账' }))
    expect(confirmRequestMock).not.toHaveBeenCalled()
    expect(screen.getByText('请输入操作原因')).toBeVisible()

    await user.type(screen.getByRole('textbox', { name: /^启动原因/ }), '  运营抽查  ')
    await user.click(screen.getByRole('button', { name: '启动对账' }))
    expect(confirmRequestMock).toHaveBeenCalledWith(
      expect.objectContaining({
        title: '确认启动Idea对账',
        message: expect.stringContaining('24 小时'),
      }),
    )

    const confirmation = confirmRequestMock.mock.calls[0]?.[0] as {
      onConfirm: () => Promise<void>
    }
    await confirmation.onConfirm()
    expect(startRequests).toEqual([{ reason: '运营抽查' }])
  })

  it('sends the selected finding status and renders the matching Task and Transaction details', async () => {
    const { user } = renderPage()

    await user.click(await screen.findByText('#7'))
    expect(await screen.findByText('task_id: task-77 · transaction_id: charge-1')).toBeVisible()
    expect(findingQueries).toContainEqual({ status: null, beforeId: null })

    await user.click(screen.getByRole('combobox', { name: '异常状态筛选' }))
    await user.click(await screen.findByRole('option', { name: '已解决' }))

    await waitFor(() =>
      expect(findingQueries).toContainEqual({ status: 'resolved', beforeId: null }),
    )
    expect(await screen.findByText('credit_transaction · refund-2')).toBeVisible()
    expect(screen.getByText('task_id: task-88 · transaction_id: refund-2')).toBeVisible()
    expect(screen.getAllByText('已解决')).toHaveLength(2)
    expect(screen.getByText('未隔离')).toBeVisible()
  })

  it('renders scheduled Runs without a synthetic Admin or reason', async () => {
    runResponse = {
      ...makeRun(),
      trigger: 'scheduled',
      requested_by: undefined,
      reason: undefined,
      scheduled_for: '2026-07-17T19:00:00Z',
    }
    const { user } = renderPage()

    expect(await screen.findByText('自动调度')).toBeVisible()
    expect(screen.getByText('系统')).toBeVisible()
    expect(screen.getByText('每日账本健康检查')).toBeVisible()
    await user.click(screen.getByText('#7'))
    expect(await screen.findByText(/计划时间：/)).toBeVisible()
  })

  it('resets the findings cursor when the selected status changes', async () => {
    const { user } = renderPage()

    await user.click(await screen.findByText('#7'))
    const nextButtons = await screen.findAllByRole('button', { name: '下一页' })
    expect(nextButtons).toHaveLength(2)
    await waitFor(() => expect(nextButtons[1]).toBeEnabled())

    await user.click(nextButtons[1]!)
    await waitFor(() => expect(findingQueries).toContainEqual({ status: null, beforeId: '91' }))

    await user.click(screen.getByRole('combobox', { name: '异常状态筛选' }))
    await user.click(await screen.findByRole('option', { name: '已解决' }))

    await waitFor(() =>
      expect(findingQueries).toContainEqual({ status: 'resolved', beforeId: null }),
    )
    await waitFor(() => expect(screen.getAllByRole('button', { name: '下一页' })[1]).toBeEnabled())
  })

  it('does not restore the previous status cursor when replacement findings fail', async () => {
    const { user } = renderPage()

    await user.click(await screen.findByText('#7'))
    const nextButtons = await screen.findAllByRole('button', { name: '下一页' })
    await waitFor(() => expect(nextButtons[1]).toBeEnabled())
    await user.click(nextButtons[1]!)
    await waitFor(() => expect(findingQueries).toContainEqual({ status: null, beforeId: '91' }))
    await waitFor(() => expect(nextButtons[1]).toBeEnabled())

    findingsShouldFail = true
    await user.click(screen.getByRole('combobox', { name: '异常状态筛选' }))
    await user.click(await screen.findByRole('option', { name: '已解决' }))

    expect(await screen.findByRole('button', { name: '重试异常列表' })).toBeVisible()
    expect(nextButtons[1]).toBeDisabled()
  })

  it('retries Run detail and findings through their corresponding queries', async () => {
    runDetailShouldFail = true
    findingsShouldFail = true
    const { user } = renderPage()

    await user.click(await screen.findByText('#7'))
    const retryRun = await screen.findByRole('button', { name: '重试 Run 详情' })
    const retryFindings = await screen.findByRole('button', { name: '重试异常列表' })

    runDetailShouldFail = false
    await user.click(retryRun)
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: '重试 Run 详情' })).not.toBeInTheDocument()
    })

    findingsShouldFail = false
    await user.click(retryFindings)
    expect(await screen.findByText('ledger_balance_mismatch')).toBeVisible()
    expect(screen.queryByRole('button', { name: '重试异常列表' })).not.toBeInTheDocument()
  })

  it('treats an HTTP 200 start business error as a failed mutation without refreshing reports', async () => {
    startBusinessError = true
    const { user } = renderPage()
    expect(await screen.findByText('#7')).toBeVisible()

    await user.type(screen.getByRole('textbox', { name: /^启动原因/ }), '运营抽查')
    await user.click(screen.getByRole('button', { name: '启动对账' }))
    const confirmation = confirmRequestMock.mock.calls[0]?.[0] as {
      onConfirm: () => Promise<void>
    }
    await confirmation.onConfirm()

    expect(toastMocks.error).toHaveBeenCalledWith('启动对账失败，请稍后重试')
    expect(toastMocks.success).not.toHaveBeenCalled()
    expect(runsRequestCount).toBe(1)
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return renderWithUser(AdminCreditReconciliationPage, {
    global: {
      plugins: [
        [PrimeVue, primeVueTestOptions],
        [VueQueryPlugin, { queryClient }],
      ],
      stubs: { Refresh: true, NavArrowLeft: true, NavArrowRight: true },
    },
  })
}

function makeRun() {
  return {
    id: 7,
    status: 'completed',
    trigger: 'manual',
    requested_by: 1,
    reason: '月度例行对账',
    stale_hold_after_seconds: 86400,
    finding_count: 1,
    critical_count: 1,
    isolated_wallet_count: 1,
    attempt_count: 1,
    requested_at: '2026-07-12T08:00:00Z',
    started_at: '2026-07-12T08:00:01Z',
    completed_at: '2026-07-12T08:00:04Z',
  }
}

function makeFinding(overrides: Record<string, unknown> = {}) {
  return {
    id: 91,
    fingerprint: 'a'.repeat(64),
    finding_type: 'ledger_balance_mismatch',
    severity: 'critical',
    risk: 'high',
    wallet_user_id: 12,
    subject_type: 'credit_transaction',
    subject_id: 'charge-1',
    details: { task_id: 'task-77', transaction_id: 'charge-1' },
    status: 'open',
    isolated: true,
    occurrence_count: 1,
    first_seen_at: '2026-07-12T08:00:03Z',
    last_seen_at: '2026-07-12T08:00:03Z',
    ...overrides,
  }
}
