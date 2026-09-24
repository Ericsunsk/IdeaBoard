import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import AdminCreditHealthPage from './AdminCreditHealthPage.vue'

const pushMock = vi.hoisted(() => vi.fn())
const toastMocks = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }))

vi.mock('vue-router', async (importOriginal) => {
  const original = await importOriginal<typeof import('vue-router')>()
  return { ...original, useRouter: () => ({ push: pushMock }) }
})

vi.mock('../../../../shared/composables/useAppToast', () => ({
  useAppToast: () => toastMocks,
}))

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
    findingRequests = []
    findingsShouldFail = false
    findings = [makeFinding()]
    delayResolvedFindings = false
    releaseResolvedFindings = undefined
    delayNextFindingPage = false
    releaseNextFindingPage = undefined
    useMswHandlers(
      http.get('/api/control/v1/credits/reconciliation/findings', async ({ request }) => {
        if (findingsShouldFail) {
          return HttpResponse.json({ error: { code: 'unavailable', message: 'unavailable' } }, { status: 503 })
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
                    makeFinding({
                      id: 40,
                      status: 'resolved',
                      isolated: false,
                      resolved_at: '2026-07-17T03:00:00Z',
                    }),
                  ]
                : beforeId !== null
                  ? [
                      makeFinding({
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
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({
                    data: makeHealth({
            status: 'attention',
            active_run: makeRun({ id: 12, status: 'running', completed_at: undefined }),
            latest_successful_run: makeRun({ id: 11, status: 'completed' }),
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
    let shouldFail = true
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        shouldFail
          ? HttpResponse.json({ error: { code: 'unavailable', message: 'unavailable' } }, { status: 503 })
          : HttpResponse.json({ data: makeHealth() }),
      ),
    )
    const { user } = renderPage()

    expect(await screen.findByText('账本健康状态不可用')).toBeVisible()
    shouldFail = false
    await user.click(screen.getByRole('button', { name: '重新加载' }))

    expect(await screen.findByText('账本健康')).toBeVisible()
  })

  it('keeps cached health visible when a background refresh fails and offers recovery', async () => {
    let shouldFail = false
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        shouldFail
          ? HttpResponse.json({ error: { code: 'unavailable', message: 'unavailable' } }, { status: 503 })
          : HttpResponse.json({ data: makeHealth() }),
      ),
    )
    const { user, queryClient } = renderPage()
    expect(await screen.findByText('账本健康')).toBeVisible()

    shouldFail = true
    await queryClient.refetchQueries({ queryKey: ['admin', 'credits', 'health'] })
    expect(await screen.findByText('状态刷新失败，当前显示上次加载的数据。')).toBeVisible()
    expect(screen.getByText('账本健康')).toBeVisible()

    shouldFail = false
    await user.click(screen.getByRole('button', { name: '重试刷新' }))
    await waitFor(() =>
      expect(screen.queryByText('状态刷新失败，当前显示上次加载的数据。')).not.toBeInTheDocument(),
    )
  })

  it('starts a manual check with an automatic audited reason and refreshes health', async () => {
    const requests: unknown[] = []
    let health = makeHealth()
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({ data: health }),
      ),
      http.post('/api/control/v1/credits/reconciliation/runs', async ({ request }) => {
        requests.push(await request.json())
        const activeRun = makeRun({ id: 14, status: 'queued', completed_at: undefined })
        health = makeHealth({ active_run: activeRun })
        return HttpResponse.json({
                    data: activeRun,
        })
      }),
    )
    const { user, queryClient } = renderPage()

    await user.click(await screen.findByRole('button', { name: '重新检查' }))

    await waitFor(() => expect(requests).toEqual([{ reason: '管理员手动触发账本检查' }]))
    expect(await screen.findByText('检查进行中 · Run #14')).toBeVisible()
    expect(screen.getByRole('button', { name: '检查中' })).toBeDisabled()
    expect(toastMocks.success).toHaveBeenCalledWith('账本检查已启动')

    health = makeHealth({ latest_successful_run: makeRun({ id: 14 }) })
    await queryClient.refetchQueries({ queryKey: ['admin', 'credits', 'health'] })

    await waitFor(() => expect(screen.queryByText('检查进行中 · Run #14')).not.toBeInTheDocument())
    expect(screen.getByText('Run #14')).toBeVisible()
    expect(screen.getByRole('button', { name: '重新检查' })).toBeEnabled()
  })

  it('shows keyset-paginated check history and expands technical details on demand', async () => {
    const runRequests: Array<string | null> = []
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({ data: makeHealth() }),
      ),
      http.get('/api/control/v1/credits/reconciliation/runs', ({ request }) => {
        const beforeId = new URL(request.url).searchParams.get('before_id')
        runRequests.push(beforeId)
        return HttpResponse.json({
                    data: {
            runs:
              beforeId === null
                ? [
                    makeRun({
                      id: 21,
                      trigger: 'manual',
                      requested_by: 7,
                      reason: '人工复核账本',
                      finding_count: 3,
                      critical_count: 1,
                      isolated_wallet_count: 1,
                      attempt_count: 2,
                      error_message: 'provider timeout',
                    }),
                  ]
                : [makeRun({ id: 20 })],
            next_before_id: beforeId === null ? 21 : undefined,
          },
        })
      }),
      http.get('/api/control/v1/credits/reconciliation/runs/:runId', ({ params }) =>
        HttpResponse.json({
                    data: makeRun({
            id: Number(params.runId),
            trigger: 'manual',
            requested_by: 7,
            reason: '人工复核账本',
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
    expect(within(dialog).getByText('人工复核账本')).toBeVisible()
    expect(within(dialog).getByText('provider timeout')).toBeVisible()
    expect(within(dialog).getByText('2')).toBeVisible()

    const nextButton = within(dialog).getByRole('button', { name: '下一页' })
    await waitFor(() => expect(nextButton).toBeEnabled())
    await user.click(nextButton)
    await waitFor(() => expect(runRequests).toContain('21'))
    expect(
      await within(dialog).findByText(
        '0 检查发现（含业务追踪） · 0 严重 Finding · 0 Wallet 已受限',
      ),
    ).toBeVisible()
    expect(within(dialog).queryByText('人工复核账本')).not.toBeInTheDocument()
  })

  it('shows a global open finding queue and opens complete diagnostics without manual resolution actions', async () => {
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({ data: makeHealth() }),
      ),
    )
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
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({ data: makeHealth() }),
      ),
    )
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
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({ data: makeHealth() }),
      ),
    )
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
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({ data: makeHealth() }),
      ),
    )
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
    useMswHandlers(
      http.get('/api/control/v1/credits/health', () =>
        HttpResponse.json({ data: makeHealth() }),
      ),
    )
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

function makeHealth(overrides: Record<string, unknown> = {}) {
  return {
    status: 'healthy',
    open_findings: { total: 0, warning: 0, error: 0, critical: 0, high_risk: 0 },
    restricted_wallet_count: 0,
    ...overrides,
  }
}

function makeRun(overrides: Record<string, unknown> = {}) {
  return {
    id: 11,
    status: 'completed',
    trigger: 'scheduled',
    finding_count: 0,
    critical_count: 0,
    isolated_wallet_count: 0,
    attempt_count: 1,
    requested_at: '2026-07-17T02:00:00Z',
    started_at: '2026-07-17T02:00:01Z',
    completed_at: '2026-07-17T02:00:04Z',
    ...overrides,
  }
}

function makeFinding(overrides: Record<string, unknown> = {}) {
  return {
    id: 41,
    fingerprint: 'a'.repeat(64),
    finding_type: 'wallet_projection_mismatch',
    severity: 'critical',
    risk: 'high',
    wallet_public_id: '00000000-0000-0000-0000-000000000012',
    wallet_display_name: 'User #12',
    subject_type: 'credit_account',
    subject_id: '17',
    details: { account_id: 17, ledger_balance: 10, projected_balance: 17 },
    status: 'open',
    isolated: true,
    occurrence_count: 3,
    first_seen_at: '2026-07-17T01:00:00Z',
    last_seen_at: '2026-07-17T02:00:00Z',
    ...overrides,
  }
}
