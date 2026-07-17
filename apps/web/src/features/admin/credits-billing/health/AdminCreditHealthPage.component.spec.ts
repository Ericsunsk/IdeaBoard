import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { primeVueTestOptions } from '../../../../test/primeVue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
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
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows one operational health panel while an active Run preserves the trusted result', async () => {
    useMswHandlers(
      http.get('/api/admin/credits/health', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
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
    expect(screen.getByRole('button', { name: '检查进行中' })).toBeDisabled()
    expect(screen.queryByText(/健康分数/)).not.toBeInTheDocument()
    expect(document.querySelector('canvas')).toBeNull()
  })

  it('recovers from an initial load failure', async () => {
    let shouldFail = true
    useMswHandlers(
      http.get('/api/admin/credits/health', () =>
        shouldFail
          ? HttpResponse.json({ code: 503, msg: 'unavailable' }, { status: 503 })
          : HttpResponse.json({ code: 0, msg: 'ok', data: makeHealth() }),
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
      http.get('/api/admin/credits/health', () =>
        shouldFail
          ? HttpResponse.json({ code: 503, msg: 'unavailable' }, { status: 503 })
          : HttpResponse.json({ code: 0, msg: 'ok', data: makeHealth() }),
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

  it('starts a manual check with an audited reason and refreshes health', async () => {
    const requests: unknown[] = []
    useMswHandlers(
      http.get('/api/admin/credits/health', () =>
        HttpResponse.json({ code: 0, msg: 'ok', data: makeHealth() }),
      ),
      http.post('/api/admin/credits/reconciliation/runs', async ({ request }) => {
        requests.push(await request.json())
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: makeRun({ id: 14, status: 'queued', completed_at: undefined }),
        })
      }),
    )
    const { user } = renderPage()

    await user.click(await screen.findByRole('button', { name: '立即检查' }))
    await user.type(screen.getByRole('textbox', { name: /操作原因/ }), '  运营复核  ')
    await user.click(screen.getByRole('button', { name: '启动检查' }))

    await waitFor(() => expect(requests).toEqual([{ reason: '运营复核' }]))
    expect(toastMocks.success).toHaveBeenCalledWith('账本检查已启动')
  })
})

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return {
    ...renderWithUser(AdminCreditHealthPage, {
    global: {
      plugins: [
        [PrimeVue, primeVueTestOptions],
        [VueQueryPlugin, { queryClient }],
      ],
      stubs: { CheckCircle: true, Clock: true, WarningTriangle: true, XmarkCircle: true },
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
    stale_hold_after_seconds: 86400,
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
