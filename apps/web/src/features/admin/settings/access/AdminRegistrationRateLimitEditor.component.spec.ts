import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import AdminRegistrationRateLimitEditor from './AdminRegistrationRateLimitEditor.vue'

vi.mock('../../../../shared/composables/useAppToast')

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('access review'))

vi.mock('../../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock, readReason: readReasonMock }),
}))

describe('AdminRegistrationRateLimitEditor', () => {
  const registrationPuts: Array<{ body: unknown; operatorReason: string | null }> = []
  const rateLimitPuts: Array<{ body: unknown; operatorReason: string | null }> = []
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
          operatorReason: request.headers.get('X-Operator-Reason'),
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
          operatorReason: request.headers.get('X-Operator-Reason'),
        })
        return HttpResponse.json({ data: body })
      }),
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
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReturnValue('access review')
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

  it('toggles registration with an operator reason (handleToggleRegistration)', async () => {
    const { user } = renderEditor()
    const toggle = await screen.findByRole('switch', { name: '开放注册' })

    await user.click(toggle)

    await waitFor(() => expect(registrationPuts).toHaveLength(1))
    expect(registrationPuts[0]?.body).toEqual({ enabled: true })
    expect(registrationPuts[0]?.operatorReason).toBe('access review')
    expect(confirmMock).toHaveBeenCalledOnce()
    await waitFor(() => expect(toggle).toBeChecked())
  })

  it('reverts the registration switch when the confirm dialog is cancelled', async () => {
    confirmMock.mockResolvedValueOnce(false)
    const { user } = renderEditor()
    const toggle = await screen.findByRole('switch', { name: '开放注册' })

    await user.click(toggle)

    await waitFor(() => expect(confirmMock).toHaveBeenCalledOnce())
    expect(registrationPuts).toHaveLength(0)
    expect(toggle).not.toBeChecked()
  })

  it('saves edited rate-limit values with an operator reason (saveRateLimit)', async () => {
    const { user } = renderEditor()
    await screen.findByDisplayValue('60')
    const maxRequests = await screen.findByDisplayValue('120')

    await user.clear(maxRequests)
    await user.type(maxRequests, '240')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(rateLimitPuts).toHaveLength(1))
    expect(rateLimitPuts[0]?.body).toEqual({ window_seconds: 60, max_requests: 240 })
    expect(rateLimitPuts[0]?.operatorReason).toBe('access review')
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
