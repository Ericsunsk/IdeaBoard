import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '@/test/msw'
import { mockAppToast, type AppToastMocks } from '@/test/toast'
import { renderWithUser, screen, waitFor, within } from '@/test/render'
import AdminCreditHealthPage from './AdminCreditHealthPage.vue'
import {
  createCreditHealthHandlers,
  createCreditHealthMswState,
  makeCreditFinding,
  makeCreditHealth,
  makeReconciliationRun,
} from '../../session/health/testing'

const pushMock = vi.hoisted(() => vi.fn())

let toastMocks: AppToastMocks

vi.mock('vue-router', async (importOriginal) => {
  const original = await importOriginal<typeof import('vue-router')>()
  return { ...original, useRouter: () => ({ push: pushMock }) }
})

vi.mock('../../../../../core/composables/useAppToast')

describe('AdminCreditHealthPage', () => {
  let findingRequests: Array<{ status: string | null; beforeId: string | null }>
  let findingsShouldFail: boolean
  let findings: Record<string, unknown>[]
  let delayResolvedFindings: boolean
  let releaseResolvedFindings: (() => void) | undefined
  let delayNextFindingPage: boolean
  let releaseNextFindingPage: (() => void) | undefined

  beforeEach(() => {
    vi.clearAllMocks()
    toastMocks = mockAppToast()
    findingRequests = []
    findingsShouldFail = false
    findings = [makeCreditFinding()]
    delayResolvedFindings = false
    releaseResolvedFindings = undefined
    delayNextFindingPage = false
    releaseNextFindingPage = undefined
    useMswHandlers(
      http.get('/api/control/v1/credits/reconciliation/findings', async ({ request }) => {
        if (findingsShouldFail) {
          return HttpResponse.json(
            { error: { code: 'unavailable', message: 'unavailable' } },
            { status: 503 },
          )
        }
        const query = new URL(request.url).searchParams
        const status = query.get('status')
        const beforeId = query.get('before_id')
        findingRequests.push({ status, beforeId })
        if (status === 'resolved' && delayResolvedFindings) {
          await new Promise<void>((resolve) => {
            releaseResolvedFindings = resolve
          })
        }
        if (beforeId !== null && delayNextFindingPage) {
          await new Promise<void>((resolve) => {
            releaseNextFindingPage = resolve
          })
        }
        return HttpResponse.json({
          data: {
            findings:
              status === 'resolved'
                ? [
                    makeCreditFinding({
                      id: 40,
                      status: 'resolved',
                      isolated: false,
                      resolved_at: '2026-07-17T03:00:00Z',
                    }),
                  ]
                : beforeId !== null
                  ? [
                      makeCreditFinding({
                        id: 39,
                        finding_type: 'stale_hold',
                        severity: 'warning',
                        risk: 'low',
                        isolated: false,
                        subject_type: 'hold',
                        subject_id: 'hold-39',
                      }),
                    ]
                  : findings,
            next_before_id: beforeId === null ? 40 : undefined,
          },
        })
      }),
    )
  })

  it('shows one operational health panel while an active Run preserves the trusted result', async () => {
    useMswHandlers(
      ...createCreditHealthHandlers(
        createCreditHealthMswState({
          health: makeCreditHealth({
            status: 'attention',
            active_run: makeReconciliationRun({
              id: 12,
              status: 'running',
              completed_at: undefined,
            }),
            latest_successful_run: makeReconciliationRun({ id: 11, status: 'completed' }),
            open_findings: { total: 2, warning: 1, error: 1, critical: 0, high_risk: 0 },
          }),
        }),
      ),
    )
    renderPage()

    expect(await screen.findByText('需要关注')).toBeVisible()
    expect(screen.getByText('检查进行中 · Run #12')).toBeVisible()
    expect(screen.getByText('Run #11')).toBeVisible()
    expect(screen.getByText('待处理异常')).toBeVisible()
    expect(screen.getByRole('button', { name: '检查中' })).toBeDisabled()
    expect(screen.queryByText(/健康分数/)).not.toBeInTheDocument()
    expect(document.querySelector('canvas')).toBeNull()
  })

  it('recovers from an initial load failure', async () => {
    const state = createCreditHealthMswState({
      healthError: {
        status: 503,
        body: { error: { code: 'unavailable', message: 'unavailable' } },
      },
    })
    useMswHandlers(...createCreditHealthHandlers(state))
    const { user } = renderPage()

    expect(await screen.findByText('账本健康状态不可用')).toBeVisible()
    state.healthError = null
    await user.click(screen.getByRole('button', { name: '重新加载' }))

    expect(await screen.findByText('账本健康')).toBeVisible()
  })

  it('keeps cached health visible when a background refresh fails and offers recovery', async () => {
    const state = createCreditHealthMswState()
    useMswHandlers(...createCreditHealthHandlers(state))
    const { user, queryClient } = renderPage()
    expect(await screen.findByText('账本健康')).toBeVisible()

    state.healthError = {
      status: 503,
      body: { error: { code: 'unavailable', message: 'unavailable' } },
    }
    await queryClient.refetchQueries({ queryKey: ['admin', 'credits', 'health'] })
    expect(await screen.findByText('状态刷新失败，当前显示上次加载的数据。')).toBeVisible()
    expect(screen.getByText('账本健康')).toBeVisible()

    state.healthError = null
    await user.click(screen.getByRole('button', { name: '重试刷新' }))
    await waitFor(() =>
      expect(screen.queryByText('状态刷新失败，当前显示上次加载的数据。')).not.toBeInTheDocument(),
    )
  })

  it('starts a manual check without a request body and refreshes health', async () => {
    let health = makeCreditHealth()
    const state = createCreditHealthMswState({
      getHealth: () => health,
      onRunStarted: (activeRun) => {
        health = makeCreditHealth({ active_run: activeRun })
      },
    })
    useMswHandlers(...createCreditHealthHandlers(state))
    const { user, queryClient } = renderPage()

    await user.click(await screen.findByRole('button', { name: '重新检查' }))

    await waitFor(() => expect(state.startRequests).toEqual(['']))
    expect(await screen.findByText('检查进行中 · Run #14')).toBeVisible()
    expect(screen.getByRole('button', { name: '检查中' })).toBeDisabled()
    expect(toastMocks.success).toHaveBeenCalledWith('账本检查已启动')

    health = makeCreditHealth({ latest_successful_run: makeReconciliationRun({ id: 14 }) })
    await queryClient.refetchQueries({ queryKey: ['admin', 'credits', 'health'] })

    await waitFor(() => expect(screen.queryByText('检查进行中 · Run #14')).not.toBeInTheDocument())
    expect(screen.getByText('Run #14')).toBeVisible()
    expect(screen.getByRole('button', { name: '重新检查' })).toBeEnabled()
  })

  it('shows keyset-paginated check history and expands technical details on demand', async () => {
    const state = createCreditHealthMswState({
      getRunsPage: (beforeId) => ({
        runs:
          beforeId === null
            ? [
                makeReconciliationRun({
                  id: 21,
                  trigger: 'manual',
                  requested_by: 7,

                  finding_count: 3,
                  critical_count: 1,
                  isolated_wallet_count: 1,
                  attempt_count: 2,
                  error_message: 'provider timeout',
                }),
              ]
            : [makeReconciliationRun({ id: 20 })],
        next_before_id: beforeId === null ? 21 : undefined,
      }),
    })
    useMswHandlers(
      ...createCreditHealthHandlers(state),
      http.get('/api/control/v1/credits/reconciliation/runs/:runId', ({ params }) =>
        HttpResponse.json({
          data: makeReconciliationRun({
            id: Number(params.runId),
            trigger: 'manual',
            requested_by: 7,

            finding_count: 3,
            critical_count: 1,
            isolated_wallet_count: 1,
            attempt_count: 2,
            error_message: 'provider timeout',
          }),
        }),
      ),
    )
    const { user } = renderPage()

    await user.click(await screen.findByRole('button', { name: '检查记录' }))
    const dialog = await screen.findByRole('dialog', { name: '检查记录' })
    expect(within(dialog).getByRole('columnheader', { name: '时间' })).toBeVisible()
    expect(within(dialog).getByRole('columnheader', { name: '状态' })).toBeVisible()
    expect(within(dialog).getByRole('columnheader', { name: 'Trigger' })).toBeVisible()
    expect(within(dialog).getByRole('columnheader', { name: '结果' })).toBeVisible()
    expect(within(dialog).getByRole('columnheader', { name: '耗时' })).toBeVisible()
    expect(await within(dialog).findByText('人工触发')).toBeVisible()
    expect(
      within(dialog).getByText('3 检查发现（含业务追踪） · 1 严重 Finding · 1 Wallet 已受限'),
    ).toBeVisible()

    const runRow = within(dialog).getByRole('row', { name: /人工触发/ })
    await user.click(within(runRow).getByRole('button'))
    expect(await within(dialog).findByText('技术 ID')).toBeVisible()
    expect(within(dialog).getByText('Run #21')).toBeVisible()
    expect(within(dialog).getByText('provider timeout')).toBeVisible()
    expect(within(dialog).getByText('2')).toBeVisible()

    const nextButton = within(dialog).getByRole('button', { name: '下一页' })
    await waitFor(() => expect(nextButton).toBeEnabled())
    await user.click(nextButton)
    await waitFor(() => expect(state.runRequests).toContain('21'))
    expect(
      await within(dialog).findByText(
        '0 检查发现（含业务追踪） · 0 严重 Finding · 0 Wallet 已受限',
      ),
    ).toBeVisible()
  })

  it('shows a global open finding queue and opens complete diagnostics without manual resolution actions', async () => {
    useMswHandlers(...createCreditHealthHandlers(createCreditHealthMswState()))
    const { user } = renderPage()

    expect(await screen.findByText('Wallet 余额投影不一致')).toBeVisible()
    expect(findingRequests).toContainEqual({ status: 'open', beforeId: null })
    expect(screen.getByText('严重 · 高风险 · Wallet 已限制')).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '异常摘要' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '影响对象' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '风险' })).toBeVisible()
    expect(screen.getByRole('columnheader', { name: '最近发现' })).toBeVisible()

    await user.click(screen.getByRole('button', { name: '查看Wallet 余额投影不一致诊断' }))
    expect(await screen.findByText('Fingerprint')).toBeVisible()
    expect(screen.getByText('a'.repeat(64))).toBeVisible()
    expect(screen.getByText('account_id')).toBeVisible()
    expect(screen.getAllByText('17')).toHaveLength(2)
    expect(screen.getByText('首次发现')).toBeVisible()
    expect(screen.getByText('出现次数')).toBeVisible()
    expect(screen.queryByRole('button', { name: /忽略|标记解决/ })).not.toBeInTheDocument()

    expect(screen.getAllByText('Wallet · User #12')).toHaveLength(2)
    expect(
      screen.queryByRole('button', { name: /打开 Wallet|Wallet · User/ }),
    ).not.toBeInTheDocument()
    expect(pushMock).not.toHaveBeenCalled()
  })

  it('supports only open and resolved views and resets keyset pagination on view changes', async () => {
    useMswHandlers(...createCreditHealthHandlers(createCreditHealthMswState()))
    const { user } = renderPage()

    expect(await screen.findByText('Wallet 余额投影不一致')).toBeVisible()
    expect(screen.queryByRole('button', { name: '全部状态' })).not.toBeInTheDocument()
    const nextButton = screen.getByRole('button', { name: '下一页' })
    await waitFor(() => expect(nextButton).toBeEnabled())
    await user.click(nextButton)
    await waitFor(() => expect(findingRequests).toContainEqual({ status: 'open', beforeId: '40' }))

    await user.click(screen.getByRole('button', { name: '已解决' }))
    await waitFor(() =>
      expect(findingRequests).toContainEqual({ status: 'resolved', beforeId: null }),
    )
    expect(await screen.findByText('严重 · 高风险 · 未限制')).toBeVisible()
  })

  it('does not leak open rows while the resolved view is loading', async () => {
    delayResolvedFindings = true
    useMswHandlers(...createCreditHealthHandlers(createCreditHealthMswState()))
    const { user } = renderPage()

    expect(await screen.findByText('Wallet 余额投影不一致')).toBeVisible()
    await user.click(screen.getByRole('button', { name: '已解决' }))
    await waitFor(() =>
      expect(findingRequests).toContainEqual({ status: 'resolved', beforeId: null }),
    )
    expect(screen.queryByText('Wallet 余额投影不一致')).not.toBeInTheDocument()

    releaseResolvedFindings?.()
    expect(await screen.findByText('Wallet 余额投影不一致')).toBeVisible()
  })

  it('does not display previous-page rows while the next keyset page is loading', async () => {
    delayNextFindingPage = true
    useMswHandlers(...createCreditHealthHandlers(createCreditHealthMswState()))
    const { user } = renderPage()

    expect(await screen.findByText('Wallet 余额投影不一致')).toBeVisible()
    const nextButton = screen.getByRole('button', { name: '下一页' })
    await waitFor(() => expect(nextButton).toBeEnabled())
    await user.click(nextButton)
    await waitFor(() => expect(findingRequests).toContainEqual({ status: 'open', beforeId: '40' }))
    expect(screen.queryByText('Wallet 余额投影不一致')).not.toBeInTheDocument()

    releaseNextFindingPage?.()
    expect(await screen.findByText('Hold 长时间未结算')).toBeVisible()
  })

  it('renders finding empty state and recovers from an initial queue error', async () => {
    findingsShouldFail = true
    useMswHandlers(...createCreditHealthHandlers(createCreditHealthMswState()))
    const { user } = renderPage()

    expect(await screen.findByText('异常队列加载失败。')).toBeVisible()
    findingsShouldFail = false
    findings = []
    await user.click(screen.getByRole('button', { name: '重新加载异常队列' }))
    expect(await screen.findByText('暂无待处理异常')).toBeVisible()
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return {
    ...renderWithUser(AdminCreditHealthPage, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
        stubs: {
          CheckCircle: true,
          Clock: true,
          Refresh: true,
          WarningTriangle: true,
          XmarkCircle: true,
        },
      },
    }),
    queryClient,
  }
}
