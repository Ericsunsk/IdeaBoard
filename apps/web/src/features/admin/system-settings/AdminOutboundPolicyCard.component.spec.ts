import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { http, HttpResponse, useMswHandlers } from '../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import AdminOutboundPolicyCard from './AdminOutboundPolicyCard.vue'

vi.mock('../../../shared/composables/useAppToast', () => ({
  useAppToast: () => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }),
}))

describe('AdminOutboundPolicyCard', () => {
  let updateRequests: unknown[]
  let allowHosts: string[]

  beforeEach(() => {
    updateRequests = []
    allowHosts = ['models.internal']
    useMswHandlers(
      http.get('/api/admin/settings/outbound-policy', () =>
        HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { allow_hosts: allowHosts },
        }),
      ),
      http.put('/api/admin/settings/outbound-policy', async ({ request }) => {
        const body = (await request.json()) as { allow_hosts: string[] }
        updateRequests.push(body)
        allowHosts = body.allow_hosts
        return HttpResponse.json({
          code: 0,
          msg: 'ok',
          data: { allow_hosts: allowHosts },
        })
      }),
    )
  })

  it('lets an admin update private host access immediately', async () => {
    const { user } = renderCard()

    await screen.findByText('已额外放行 1 项')
    const allowHostsInput = screen.getByRole('textbox', { name: /允许访问的私有主机/ })
    expect(allowHostsInput).toHaveValue('models.internal')
    expect(screen.getByText('已额外放行 1 项')).toBeVisible()
    expect(screen.queryByText('Task Runtime')).not.toBeInTheDocument()
    expect(screen.queryByText('Event Retention')).not.toBeInTheDocument()

    await user.clear(allowHostsInput)
    await user.type(allowHostsInput, 'Models.Internal\n10.0.0.8:8443')

    await user.click(screen.getByRole('button', { name: '保存访问策略' }))

    await waitFor(() => {
      expect(updateRequests).toEqual([
        {
          allow_hosts: ['models.internal', '10.0.0.8:8443'],
        },
      ])
    })
    expect(screen.getByText('已额外放行 2 项')).toBeVisible()
  })
})

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return renderWithUser(AdminOutboundPolicyCard, {
    global: {
      plugins: [
        [PrimeVue, { unstyled: true }],
        [VueQueryPlugin, { queryClient }],
      ],
    },
  })
}
