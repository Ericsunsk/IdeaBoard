import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { screen, waitFor } from '@testing-library/vue'
import { defineComponent, reactive, ref } from 'vue'
import { beforeEach, describe, expect, it, test, vi } from 'vitest'
import { renderWithUser } from '../../../../test/render'
import { billingAccountQueryKeys } from './resource'
import { useBillingAccount } from './useBillingAccount'
import { useBillingAccountContext } from './useBillingAccountContext'

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

describe('caller-selected Billing Account context', () => {
  beforeEach(() => {
    identity.user.public_id = 'user-1'
    fetchAccount.mockReset()
  })

  test('keeps a local target independent from global selection and cancels an obsolete same-target generation', async () => {
    const target = ref('ws_a')
    let finishOld!: (value: ReturnType<typeof account>) => void
    fetchAccount.mockImplementation((workspaceId: string) => {
      if (workspaceId === 'ws_a' && !finishOld) {
        return new Promise((resolve) => {
          finishOld = resolve
        })
      }
      return Promise.resolve({
        ...account(workspaceId === 'ws_a' ? 'member' : 'account'),
        workspaceId,
        billingAccountId: `ba_${workspaceId}`,
      })
    })
    const LocalAndGlobal = defineComponent({
      setup() {
        return {
          local: useBillingAccountContext({
            workspaceId: target,
            identity: () => identity.user.public_id,
          }),
          global: useBillingAccount(),
        }
      },
      template:
        '<output data-testid="local">{{ local.isReady.value }}:{{ local.readScope.value.workspaceId }}:{{ local.consumptionScope.value }}</output><output data-testid="global">{{ global.readScope.value.workspaceId }}:{{ global.consumptionScope.value }}</output>',
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    renderWithUser(LocalAndGlobal, { global: { plugins: [[VueQueryPlugin, { queryClient }]] } })
    await waitFor(() => expect(screen.getByTestId('global')).toHaveTextContent('ws_1:account'))
    expect(screen.getByTestId('local')).toHaveTextContent('false:ws_a:')
    target.value = 'ws_b'
    await waitFor(() => expect(screen.getByTestId('local')).toHaveTextContent('true:ws_b:account'))
    target.value = 'ws_a'
    await waitFor(() => expect(screen.getByTestId('local')).toHaveTextContent('true:ws_a:member'))
    finishOld({ ...account('account'), workspaceId: 'ws_a' })
    await Promise.resolve()
    await Promise.resolve()
    expect(screen.getByTestId('local')).toHaveTextContent('true:ws_a:member')
    expect(screen.getByTestId('global')).toHaveTextContent('ws_1:account')
  })

  test('fails closed on prerequisite error and restores management through the context retry', async () => {
    fetchAccount
      .mockRejectedValueOnce(new Error('account unavailable'))
      .mockResolvedValueOnce(account('account'))
    const Local = defineComponent({
      setup: () => useBillingAccountContext({ workspaceId: 'ws_1', identity: 'user-1' }),
      template:
        '<output data-testid="permission">{{ isReady }}:{{ consumptionScope }}</output><output role="alert">{{ error?.message }}</output><button @click="retry()">Retry</button>',
    })
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { user } = renderWithUser(Local, {
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    })
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('account unavailable'))
    expect(screen.getByTestId('permission')).toHaveTextContent('false:')
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(screen.getByTestId('permission')).toHaveTextContent('true:account'))
    expect(screen.getByRole('alert')).toBeEmptyDOMElement()
  })
})
