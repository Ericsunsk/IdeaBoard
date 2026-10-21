import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent, reactive, ref } from 'vue'
import { beforeEach, describe, expect, it, test, vi } from 'vitest'
import { renderWithUser } from '../../../../test/render'
import { billingAccountQueryKeys } from './resource'
import { useBillingAccount } from './useBillingAccount'

const fetchAccount = vi.hoisted(() => vi.fn())
const identity = reactive({ user: { public_id: 'user-1' } })
vi.mock('../../../auth', () => ({ useUserStore: () => identity }))
vi.mock('../accountWorkspace', () => ({
  useAccountWorkspace: () => ({ workspaceId: ref('ws_1'), isSelectionSettled: ref(true) }),
}))
vi.mock('./resource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./resource')>()),
  fetchBillingAccount: fetchAccount,
}))
const Host = defineComponent({
  setup: () => useBillingAccount(),
  template:
    '<output data-testid="scope">{{ JSON.stringify([readScope.identity, readScope.consumptionScope]) }}</output><output data-testid="ready">{{ isReady }}</output>',
})
const account = (consumptionScope: 'account' | 'member') => ({
  billingAccountId: 'ba_1',
  workspaceId: 'ws_1',
  isShared: true,
  consumptionScope,
})

describe('billing authorization scope', () => {
  beforeEach(() => {
    identity.user.public_id = 'user-1'
    fetchAccount.mockReset()
  })

  it('re-resolves the same workspace for another identity without reusing a manager scope', async () => {
    fetchAccount.mockResolvedValueOnce(account('account')).mockResolvedValueOnce(account('member'))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(Host, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
    await waitFor(() =>
      expect(screen.getByTestId('scope')).toHaveTextContent('["user-1","account"]'),
    )
    identity.user.public_id = 'user-2'
    await waitFor(() =>
      expect(screen.getByTestId('scope')).toHaveTextContent('["user-2","member"]'),
    )
    expect(fetchAccount).toHaveBeenCalledTimes(2)
  })

  test('waits for the new identity and ignores the previous identity late resolution', async () => {
    let resolvePrevious!: (scope: ReturnType<typeof account>) => void
    fetchAccount
      .mockImplementationOnce(
        () =>
          new Promise((resolve) => {
            resolvePrevious = resolve
          }),
      )
      .mockResolvedValueOnce(account('member'))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(Host, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
    await waitFor(() => expect(fetchAccount).toHaveBeenCalledOnce())
    expect(screen.getByTestId('ready')).toHaveTextContent('false')
    identity.user.public_id = 'user-2'
    await waitFor(() =>
      expect(screen.getByTestId('scope')).toHaveTextContent('["user-2","member"]'),
    )
    resolvePrevious(account('account'))
    await Promise.resolve()
    expect(screen.getByTestId('scope')).toHaveTextContent('["user-2","member"]')
    expect(screen.getByTestId('ready')).toHaveTextContent('true')
  })

  it('disables dependent reads when revalidating a cached authorization fails', async () => {
    fetchAccount
      .mockResolvedValueOnce(account('account'))
      .mockRejectedValueOnce(new Error('permission lookup failed'))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(Host, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
    await waitFor(() => expect(screen.getByTestId('ready')).toHaveTextContent('true'))
    await queryClient.invalidateQueries({ queryKey: billingAccountQueryKeys.all })
    await waitFor(() => expect(screen.getByTestId('ready')).toHaveTextContent('false'))
  })

  it('changes the downstream scope key when the server refreshes financial standing', async () => {
    fetchAccount.mockResolvedValueOnce(account('account')).mockResolvedValueOnce(account('member'))
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(Host, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
    await waitFor(() =>
      expect(screen.getByTestId('scope')).toHaveTextContent('["user-1","account"]'),
    )
    await queryClient.invalidateQueries({ queryKey: billingAccountQueryKeys.all })
    await waitFor(() =>
      expect(screen.getByTestId('scope')).toHaveTextContent('["user-1","member"]'),
    )
  })
})
