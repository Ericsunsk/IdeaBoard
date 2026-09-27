import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMswHandlers } from '../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../test/render'
import { mockAppToast } from '../../../test/toast'
import { createOutboundPolicyHandlers, createOutboundPolicyMswState } from './testing'
import AdminOutboundPolicyCard from './AdminOutboundPolicyCard.vue'

vi.mock('../../../shared/composables/useAppToast')

const confirmMock = vi.hoisted(() => vi.fn().mockResolvedValue(true))
const readReasonMock = vi.hoisted(() => vi.fn().mockReturnValue('policy review'))

vi.mock('../shared/composables/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock, readReason: readReasonMock }),
}))

describe('AdminOutboundPolicyCard', () => {
  let updateRequests: Array<{ body: unknown; operatorReason: string | null }>
  let allowHosts: string[]

  beforeEach(() => {
    mockAppToast()
    confirmMock.mockReset()
    confirmMock.mockResolvedValue(true)
    readReasonMock.mockReset()
    readReasonMock.mockReturnValue('policy review')
    allowHosts = ['models.internal']
    const policyApi = createOutboundPolicyMswState({
      getAllowHosts: () => allowHosts,
      onUpdate: (body) => {
        allowHosts = body.allow_hosts
      },
    })
    updateRequests = policyApi.updateRequests
    useMswHandlers(...createOutboundPolicyHandlers(policyApi))
  })

  it('lets an admin update private host access through control with an operator reason', async () => {
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
          body: {
            allow_hosts: ['models.internal', '10.0.0.8:8443'],
            reason: 'policy review',
          },
          operatorReason: 'policy review',
        },
      ])
    })
    expect(confirmMock).toHaveBeenCalledOnce()
    expect(screen.getByText('已额外放行 2 项')).toBeVisible()
  })

  it('aborts the save when the operator cancels the confirm dialog', async () => {
    confirmMock.mockResolvedValueOnce(false)
    const { user } = renderCard()

    await screen.findByText('已额外放行 1 项')
    const allowHostsInput = screen.getByRole('textbox', { name: /允许访问的私有主机/ })
    await user.clear(allowHostsInput)
    await user.type(allowHostsInput, 'draft.internal')

    await user.click(screen.getByRole('button', { name: '保存访问策略' }))

    await waitFor(() => {
      expect(confirmMock).toHaveBeenCalledOnce()
    })
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(updateRequests).toEqual([])
  })
})

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return renderWithUser(AdminOutboundPolicyCard, {
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}
