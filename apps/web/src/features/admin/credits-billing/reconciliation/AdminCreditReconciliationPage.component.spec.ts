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
  let startRequests: unknown[]
  let runDetailShouldFail: boolean
  let startBusinessError: boolean
  let runsRequestCount: number
  let runResponse: Record<string, unknown>

  beforeEach(() => {
    confirmRequestMock.mockReset()
    startRequests = []
    runDetailShouldFail = false
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
    )
  })

  it('shows Run audit details without a second Finding surface and requires confirmation to start', async () => {
    const { user } = renderPage()

    expect(await screen.findByText('#7')).toBeVisible()
    await user.click(screen.getByText('#7'))

    expect(await screen.findByText('Run #7')).toBeVisible()
    expect(screen.getAllByText('月度例行对账').length).toBeGreaterThan(0)
    expect(screen.getByText('尝试 1 次 · 1 项异常 · 1 个 Wallet 受限')).toBeVisible()
    expect(screen.queryByLabelText('异常状态筛选')).not.toBeInTheDocument()
    expect(screen.queryByText('异常列表加载失败。')).not.toBeInTheDocument()

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
    expect(await screen.findByText('计划时间')).toBeVisible()
  })

  it('retries Run details through the retained Run API', async () => {
    runDetailShouldFail = true
    const { user } = renderPage()

    await user.click(await screen.findByText('#7'))
    const retryRun = await screen.findByRole('button', { name: '重试 Run 详情' })

    runDetailShouldFail = false
    await user.click(retryRun)
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: '重试 Run 详情' })).not.toBeInTheDocument()
    })
    expect(await screen.findByText('Run #7')).toBeVisible()
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
