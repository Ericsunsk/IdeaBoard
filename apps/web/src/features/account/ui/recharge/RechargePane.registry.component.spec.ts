import { cleanup } from '@testing-library/vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { renderWithUser, screen } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import { accountTabRegistry } from '../shared/tabRegistry'
import RechargePane from './RechargePane.vue'
import {
  paymentsContextKey,
  type PaymentsContext,
  type PaymentsResource,
} from '../../session/payments/resource'

vi.mock('../../session/accountWorkspace', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../session/accountWorkspace')>()),
  useAccountWorkspace: () => ({
    workspaceId: ref('ws_1'),
    isSelectionSettled: ref(true),
  }),
}))

vi.mock('../../../../core/composables/useAppToast')

const purchaseOptions = {
  // eslint-disable-next-line governance/boolean-prefix -- Wire contract field (PaymentPurchaseOptions), same shape as RechargePane.component.spec.
  purchases_available: true,
  quick_idea_quantities: [1000, 2000, 3000, 5000, 10_000],
  min_idea_quantity: 500,
  max_idea_quantity: 500_000,
  ideas_per_usd: 100,
  terms: { version: '2026-08-01', url: '/terms' },
  refund_policy: { version: '2026-08-01', url: '/refund-policy' },
} as const

function makeResource(overrides: Partial<PaymentsResource> = {}): PaymentsResource {
  return {
    getPurchaseOptions: vi.fn().mockResolvedValue(purchaseOptions),
    createCheckout: vi.fn(),
    getReturnStatus: vi.fn().mockResolvedValue({
      idea_quantity: 1000,
      status: 'processing',
      updated_at: '2026-08-01T00:30:00Z',
    }),
    listPendingOrders: vi.fn().mockResolvedValue({ items: [] }),
    continuePendingOrder: vi.fn(),
    abandonPendingOrder: vi.fn(),
    listRecharges: vi.fn().mockResolvedValue({ items: [] }),
    subscribe: vi.fn(() => () => undefined),
    ...overrides,
  }
}

function resolveRechargePane() {
  const entry = accountTabRegistry
    .flatMap((group) => group.tabs.map((tab) => ({ group, tab })))
    .find(({ tab }) => tab.key === 'recharge')
  if (!entry) throw new Error('recharge tab missing from accountTabRegistry')
  return entry
}

function mountViaRegistry(resource: PaymentsResource) {
  const { group, tab } = resolveRechargePane()
  expect(tab.pane.component).toBe(RechargePane)
  const paneProps = tab.pane.props({
    // eslint-disable-next-line governance/boolean-prefix -- Registry context field (AccountTabPaneContext.active), same call shape as AccountDialogContent.
    active: true,
    group,
    tab,
    paymentReturn: null,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
  } as any) as { isActive: boolean; paymentReturn: null }
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const context: PaymentsContext = {
    resource,
    navigateToCheckout: vi.fn(),
  }
  const rendered = renderWithUser(tab.pane.component, {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    props: paneProps as any,
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
      provide: { [paymentsContextKey as symbol]: context },
    },
  })
  return { rendered, resource, paneProps }
}

describe('RechargePane via accountTabRegistry', () => {
  it('loads purchase options through the real registry wiring', async () => {
    mockAppToast()
    const resource = makeResource()
    const { paneProps } = mountViaRegistry(resource)

    // 用户症状：对话框弹出（标题可见）但数量包空白。
    expect(await screen.findByRole('heading', { name: 'Top up' })).toBeVisible()
    const packages = await screen.findAllByRole('button', { name: /Ideas.*USD/ })
    expect(packages).toHaveLength(5)
    expect(resource.getPurchaseOptions).toHaveBeenCalledWith('ws_1', expect.any(AbortSignal))
    // H1: registry 必须把 active 转写为 pane 的 isActive，否则查询永不 enabled。
    expect(paneProps).toEqual({ isActive: true, paymentReturn: null })
  })

  it('reloads purchase options after leaving and re-entering the recharge tab', async () => {
    mockAppToast()
    const first = makeResource()
    mountViaRegistry(first)

    expect(await screen.findByRole('heading', { name: 'Top up' })).toBeVisible()
    expect(await screen.findAllByRole('button', { name: /Ideas.*USD/ })).toHaveLength(5)
    cleanup()

    const second = makeResource()
    mountViaRegistry(second)

    expect(await screen.findByRole('heading', { name: 'Top up' })).toBeVisible()
    expect(await screen.findAllByRole('button', { name: /Ideas.*USD/ })).toHaveLength(5)
    expect(second.getPurchaseOptions).toHaveBeenCalledWith('ws_1', expect.any(AbortSignal))
  })
})
