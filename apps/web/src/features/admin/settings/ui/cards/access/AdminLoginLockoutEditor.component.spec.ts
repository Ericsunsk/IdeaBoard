import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../../../test/render'
import { mockAppToast } from '../../../../../../test/toast'
import AdminLoginLockoutEditor from './AdminLoginLockoutEditor.vue'

vi.mock('../../../../../../core/composables/useAppToast')

describe('AdminLoginLockoutEditor', () => {
  const lockoutPuts: Array<{ body: unknown }> = []

  function useLockoutHandlers() {
    useMswHandlers(
      http.get('/api/control/v1/platform/settings/registration', () =>
        HttpResponse.json({ data: { enabled: false } }),
      ),
      http.get('/api/control/v1/platform/settings/rate-limit', () =>
        HttpResponse.json({ data: { window_seconds: 60, max_requests: 120 } }),
      ),
      http.get('/api/control/v1/platform/settings/login-lockout', () =>
        HttpResponse.json({ data: { threshold: 5, duration_seconds: 300 } }),
      ),
      http.put('/api/control/v1/platform/settings/login-lockout', async ({ request }) => {
        const body = await request.json()
        lockoutPuts.push({
          body,
        })
        return HttpResponse.json({ data: body })
      }),
    )
  }

  function renderEditor() {
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return renderWithUser(AdminLoginLockoutEditor, {
      global: {
        plugins: [[VueQueryPlugin, { queryClient }]],
      },
    })
  }

  beforeEach(() => {
    mockAppToast()
    vi.clearAllMocks()
    lockoutPuts.length = 0
    useLockoutHandlers()
  })

  it('hydrates threshold and duration fields from the control plane', async () => {
    renderEditor()

    expect(screen.getByText('登录锁定')).toBeVisible()
    expect(await screen.findByDisplayValue('5')).toBeVisible()
    expect(screen.getByDisplayValue('300')).toBeVisible()
  })

  it('renders a single reset/save pair in the card footer actions', async () => {
    renderEditor()
    await screen.findByDisplayValue('5')

    expect(screen.getAllByRole('button', { name: '重置' })).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: '保存' })).toHaveLength(1)
  })

  it('saves edited lockout values without a confirm dialog (saveLoginLockout)', async () => {
    const toast = mockAppToast()
    const { user } = renderEditor()
    await screen.findByDisplayValue('5')
    const threshold = await screen.findByDisplayValue('5')

    await user.clear(threshold)
    await user.type(threshold, '8')
    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(lockoutPuts).toHaveLength(1))
    expect(lockoutPuts[0]?.body).toEqual({ threshold: 8, duration_seconds: 300 })
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('登录锁定已保存'))
  })

  it('restores saved lockout values without network traffic (resetLoginLockout)', async () => {
    const { user } = renderEditor()
    await screen.findByDisplayValue('5')
    const duration = (await screen.findByDisplayValue('300')) as HTMLInputElement

    await user.clear(duration)
    await user.type(duration, '600')
    expect(duration.value).toBe('600')

    await user.click(screen.getByRole('button', { name: '重置' }))

    await waitFor(() => expect(duration.value).toBe('300'))
    expect(lockoutPuts).toHaveLength(0)
  })
})
