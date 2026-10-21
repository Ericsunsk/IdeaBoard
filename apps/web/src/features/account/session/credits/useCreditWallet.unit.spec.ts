import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent, reactive, ref } from 'vue'
import { beforeEach, describe, expect, test, vi } from 'vitest'
import { renderWithUser } from '../../../../test/render'
import { billingAccountQueryKeys } from '../billing-account/resource'
import { useCreditWallet } from './useCreditWallet'

const fetchWallet = vi.hoisted(() => vi.fn())
const fetchAccount = vi.hoisted(() => vi.fn())
const identity = reactive({ user: { public_id: 'user-1' } })
const isEnabled = ref(true)
vi.mock('../../../auth', () => ({ useUserStore: () => identity }))
vi.mock('./useCreditsSync', () => ({ useCreditsSync: vi.fn() }))
vi.mock('./resource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./resource')>()),
  fetchCreditWallet: fetchWallet,
}))
vi.mock('../accountWorkspace', () => ({
  useAccountWorkspace: () => ({
    workspaceId: ref('ws_1'),
    isSelectionSettled: ref(true),
  }),
}))
vi.mock('../billing-account/resource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../billing-account/resource')>()),
  fetchBillingAccount: fetchAccount,
}))
const Host = defineComponent({
  setup: () => useCreditWallet(isEnabled),
  template: '<output data-testid="wallet">{{ state }}</output>',
})

function buildAccount(consumptionScope: 'account' | 'member' = 'account') {
  return {
    billingAccountId: 'ba_1',
    workspaceId: 'ws_1',
    isShared: false,
    consumptionScope,
  }
}

function renderWallet() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  renderWithUser(Host, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
  return queryClient
}

describe('useCreditWallet', () => {
  beforeEach(() => {
    identity.user.public_id = 'user-1'
    isEnabled.value = true
    fetchAccount.mockReset().mockResolvedValue(buildAccount())
    fetchWallet.mockReset().mockResolvedValue({
      billing_account_id: 'ba_1',
      available_credits: 1280,
      reserved_credits: 0,
    })
  })

  test('surfaces account resolution failure while the dependent wallet query is disabled', async () => {
    fetchAccount.mockRejectedValueOnce(new Error('account unavailable'))
    renderWallet()
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('error'))
    expect(fetchWallet).not.toHaveBeenCalled()
  })

  test('shows loading until the account has resolved', () => {
    fetchAccount.mockReturnValueOnce(new Promise(() => {}))
    renderWallet()
    expect(screen.getByTestId('wallet')).toHaveTextContent('loading')
    expect(fetchWallet).not.toHaveBeenCalled()
  })

  test('projects the balance and pins the request to the resolved account', async () => {
    renderWallet()
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('1280'))
    expect(screen.getByTestId('wallet')).toHaveTextContent('ba_1')
    expect(fetchWallet).toHaveBeenCalledWith('ws_1', expect.any(AbortSignal), 'ba_1')
  })

  test('maps a failed wallet read to error', async () => {
    fetchWallet.mockRejectedValueOnce(new Error('wallet unavailable'))
    renderWallet()
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('error'))
    expect(screen.getByTestId('wallet')).not.toHaveTextContent('loading')
  })

  test('does not reuse the previous identity balance on the same account', async () => {
    renderWallet()
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('1280'))
    fetchWallet.mockRejectedValueOnce(new Error('second identity denied'))
    identity.user.public_id = 'user-2'
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('error'))
    expect(screen.getByTestId('wallet')).not.toHaveTextContent('1280')
    expect(fetchWallet).toHaveBeenCalledTimes(2)
  })

  test('keeps wallet visibility independent from consumption visibility', async () => {
    const queryClient = renderWallet()
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('1280'))
    fetchAccount.mockResolvedValueOnce(buildAccount('member'))
    await queryClient.invalidateQueries({ queryKey: billingAccountQueryKeys.all })
    expect(screen.getByTestId('wallet')).toHaveTextContent('1280')
    expect(fetchWallet).toHaveBeenCalledOnce()
  })

  test('hides a cached balance when the account read is disabled', async () => {
    renderWallet()
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('1280'))
    isEnabled.value = false
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('loading'))
    expect(screen.getByTestId('wallet')).not.toHaveTextContent('1280')
  })

  test('hides a cached balance when account revalidation fails', async () => {
    const queryClient = renderWallet()
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('1280'))
    fetchAccount.mockRejectedValueOnce(new Error('authorization lookup failed'))
    await queryClient.invalidateQueries({ queryKey: billingAccountQueryKeys.all })
    await waitFor(() => expect(screen.getByTestId('wallet')).toHaveTextContent('error'))
    expect(screen.getByTestId('wallet')).not.toHaveTextContent('1280')
  })
})
