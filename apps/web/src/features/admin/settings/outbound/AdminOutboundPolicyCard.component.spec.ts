import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMswHandlers } from '../../../../test/msw'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import { createOutboundPolicyHandlers, createOutboundPolicyMswState } from '../shared/testing'
import AdminOutboundPolicyCard from './AdminOutboundPolicyCard.vue'

vi.mock('../../../../core/composables/useAppToast')

describe('AdminOutboundPolicyCard', () => {
  let updateRequests: Array<{ body: unknown; operatorReason: string | null }>
  let allowHosts: string[]

  beforeEach(() => {
    mockAppToast()
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

  it('lets an admin update private host access through control without a confirm dialog', async () => {
    const { user } = renderCard()

    await screen.findByText('已额外放行 1 项')
    const allowHostsInput = screen.getByRole('textbox', { name: /允许访问的私有主机/ })
    expect(allowHostsInput).toHaveValue('models.internal')
    expect(screen.getByText('已额外放行 1 项')).toBeVisible()
    // 方案 A：已额外放行是正常态，走中性（secondary），不用 warn 黄。
    expect(screen.getByText('已额外放行 1 项').closest('[class*="bg-"]')).not.toHaveClass(
      'bg-warning-soft',
    )
    expect(screen.queryByText('Task Runtime')).not.toBeInTheDocument()
    expect(screen.queryByText('Event Retention')).not.toBeInTheDocument()

    await user.clear(allowHostsInput)
    await user.type(allowHostsInput, 'Models.Internal\n10.0.0.8:8443')

    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(updateRequests).toEqual([
        {
          body: {
            allow_hosts: ['models.internal', '10.0.0.8:8443'],
            reason: '',
          },
          operatorReason: null,
        },
      ])
    })
    expect(screen.getByText('已额外放行 2 项')).toBeVisible()
  })

  it('routes validation failure to toast only without an inline alert', async () => {
    const mocks = mockAppToast()
    const { user } = renderCard()

    await screen.findByText('已额外放行 1 项')
    const allowHostsInput = screen.getByRole('textbox', { name: /允许访问的私有主机/ })

    await user.clear(allowHostsInput)
    await user.type(allowHostsInput, 'not a host')

    await user.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => expect(mocks.error).toHaveBeenCalledOnce())
    expect(mocks.error).toHaveBeenCalledWith('私有主机仅支持 hostname、IP 或 host:port，每行一个')
    expect(updateRequests).toEqual([])
    // 校验失败仅 toast，不设内嵌 alert。
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows a loading spinner without an inline alert while loading', async () => {
    let resolveLoad!: (hosts: string[]) => void
    const pendingLoad = new Promise<string[]>((resolve) => {
      resolveLoad = resolve
    })
    useMswHandlers(
      ...createOutboundPolicyHandlers(
        createOutboundPolicyMswState({ getAllowHosts: () => pendingLoad }),
      ),
    )
    renderCard()

    // 加载态走 role=status + spinner（对齐 mail/payment），不设内嵌 alert。
    const status = await screen.findByRole('status')
    expect(status).toBeVisible()
    // AdminSpinner 内含 sr-only 同名文本，故用文本内容断言而非 getByText。
    expect(status).toHaveTextContent('正在加载私有地址访问策略…')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()

    resolveLoad(['models.internal'])
    await screen.findByText('已额外放行 1 项')
  })

  it('routes load failure to toast error without an inline alert and keeps Status as truth', async () => {
    const mocks = mockAppToast()
    useMswHandlers(
      ...createOutboundPolicyHandlers(
        createOutboundPolicyMswState({
          loadError: { status: 500, body: { error: { code: '500', message: 'down' } } },
        }),
      ),
    )
    renderCard()

    await waitFor(() => expect(mocks.error).toHaveBeenCalledOnce())
    expect(mocks.error).toHaveBeenCalledWith(expect.any(String))
    expect(mocks.success).not.toHaveBeenCalled()
    // 方案 A：加载失败只 toast，不进 Status；失败一律无内嵌 alert。
    expect(screen.queryByText('加载失败')).not.toBeInTheDocument()
    expect(screen.queryByText('加载中')).not.toBeInTheDocument()
    expect(screen.queryByText(/已额外放行/)).not.toBeInTheDocument()
    expect(screen.queryByText('未额外放行')).not.toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
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
