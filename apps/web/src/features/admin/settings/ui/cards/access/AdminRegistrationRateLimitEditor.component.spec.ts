import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../../../test/render'
import { mockAppToast } from '../../../../../../test/toast'
import AdminRegistrationRateLimitEditor from './AdminRegistrationRateLimitEditor.vue'

vi.mock('../../../../../../core/composables/useAppToast')

describe('AdminRegistrationRateLimitEditor', () => {
  const registrationPuts: Array<{ body: unknown }> = []
  const rateLimitPuts: Array<{ body: unknown }> = []
  let registrationEnabled = false

  function useAccessHandlers() {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/registration', () =>
        HttpResponse.json({ data: { enabled: registrationEnabled } }),
      ),
      http.put('/api/control/v1/platform/settings/registration', async ({ request }) => {
        const body = await request.json()
        registrationPuts.push({
          body,
        })
        registrationEnabled = (body as { enabled: boolean }).enabled
        return HttpResponse.json({ data: body })
      }),
      http.get('/api/control/v1/platform/settings/rate-limit', () =>
        HttpResponse.json({ data: { window_seconds: 60, max_requests: 120 } }),
      ),
      http.put('/api/control/v1/platform/settings/rate-limit', async ({ request }) => {
        const body = await request.json()
        rateLimitPuts.push({
          body,
        })
        return HttpResponse.json({ data: body })
      }),
      http.get('/api/control/v1/platform/settings/login-lockout', () =>
        HttpResponse.json({ data: { threshold: 5, duration_seconds: 300 } }),
      ),
    )
  }

  function renderEditor() {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return renderWithUser(AdminRegistrationRateLimitEditor, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })
  }

  beforeEach(() => {
    mockAppToast()
    vi.clearAllMocks()
    registrationPuts.length = 0
    rateLimitPuts.length = 0
    registrationEnabled = false
    useAccessHandlers()
  })

  it('hydrates registration switch and rate-limit fields from the control plane', async () => {
    renderEditor()

    expect(screen.getByText('注册限流')).toBeVisible()
    const toggle = await screen.findByRole('switch', { name: '开放注册' })
    expect(toggle).not.toBeChecked()
    expect(await screen.findByDisplayValue('60')).toBeVisible()
    expect(screen.getByDisplayValue('120')).toBeVisible()
  })

  it('renders a single reset/save pair in the card footer actions', async () => {
    renderEditor()
    await screen.findByDisplayValue('60')

    // 去重回归：模板内只保留 #actions 单组 AdminEditorActions，登录锁定内联栏已移除。
    expect(screen.getAllByRole('button', { name: '重置' })).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: '保存' })).toHaveLength(1)
    expect(screen.queryByRole('region', { name: '登录锁定' })).not.toBeInTheDocument()
  })

  it('toggles registration without a confirm dialog (handleToggleRegistration)', async () => {
    const { user } = renderEditor()
    const toggle = await screen.findByRole('switch', { name: '开放注册' })

    await user.click(toggle)

    await waitFor(() => expect(registrationPuts).toHaveLength(1))
    expect(registrationPuts[0]?.body).toEqual({ enabled: true })
    await waitFor(() => expect(toggle).toBeChecked())
  })

  it('saves edited rate-limit values without a confirm dialog (saveRateLimit)', async () => {
    const { user } = renderEditor()
    await screen.findByDisplayValue('60')
    const maxRequests = await screen.findByDisplayValue('120')

    await user.clear(maxRequests)
    await user.type(maxRequests, '240')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(rateLimitPuts).toHaveLength(1))
    expect(rateLimitPuts[0]?.body).toEqual({ window_seconds: 60, max_requests: 240 })
  })

  it('restores saved rate-limit values without network traffic (resetRateLimit)', async () => {
    const { user } = renderEditor()
    await screen.findByDisplayValue('60')
    const maxRequests = (await screen.findByDisplayValue('120')) as HTMLInputElement

    await user.clear(maxRequests)
    await user.type(maxRequests, '999')
    expect(maxRequests.value).toBe('999')

    await user.click(screen.getByRole('button', { name: '重置' }))

    await waitFor(() => expect(maxRequests.value).toBe('120'))
    expect(rateLimitPuts).toHaveLength(0)
  })
})
