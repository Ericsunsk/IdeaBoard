import { QueryClient, VueQueryPlugin, type UseQueryOptions } from '@tanstack/vue-query'
import { mount, flushPromises } from '@vue/test-utils'
import { computed, defineComponent, h, nextTick, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { waitFor } from '@testing-library/vue'
import { fetchBillingAccount, type BillingAccountScope } from '../billing-account/resource'
import type { WorkspaceMemberEntry } from '../accountWorkspace'
import {
  provideWorkspaceSettingsReads,
  useWorkspaceSettingsBillingAccount,
  useWorkspaceSettingsRoster,
} from './index'

const fetchRoster = vi.hoisted(() => vi.fn())
vi.mock('../accountWorkspace', async (load) => ({
  ...(await load<typeof import('../accountWorkspace')>()),
  workspaceMembersQueryOptions: (workspaceId: MaybeRefOrGetter<string>) =>
    ({
      queryKey: computed(() => ['members', toValue(workspaceId)]),
      queryFn: () => fetchRoster(toValue(workspaceId)),
      retry: false,
      staleTime: Infinity,
    }) satisfies UseQueryOptions<WorkspaceMemberEntry[], Error>,
}))
vi.mock('../billing-account/resource', async (load) => ({
  ...(await load<typeof import('../billing-account/resource')>()),
  fetchBillingAccount: vi.fn(),
}))

function account(workspaceId: string): BillingAccountScope {
  return {
    workspaceId,
    billingAccountId: `ba_${workspaceId}`,
    consumptionScope: 'account',
    isShared: true,
    canManageMemberQuotas: true,
  }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((complete) => {
    resolve = complete
  })
  return { promise, resolve }
}
function setupReads(isShared = true, isRosterOnly = false) {
  const selected = ref('ws_1')
  const selectedIdentity = ref('user_1')
  const target = ref('ws_1')
  const identity = ref('user_1')
  const isOpen = ref(true)
  const isExpanded = ref(true)
  let roster!: ReturnType<typeof useWorkspaceSettingsRoster>
  let billing!: ReturnType<typeof useWorkspaceSettingsBillingAccount>
  const Child = defineComponent({
    setup() {
      roster = useWorkspaceSettingsRoster({ workspaceId: target, isEnabled: isExpanded })
      if (!isRosterOnly)
        billing = useWorkspaceSettingsBillingAccount({
          workspaceId: target,
          identity,
          isEnabled: isExpanded,
        })
      return () => h('div')
    },
  })
  const Host = defineComponent({
    setup() {
      if (isShared)
        provideWorkspaceSettingsReads({
          workspaceId: selected,
          identity: selectedIdentity,
          isEnabled: isOpen,
        })
      return () => h(Child)
    },
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrapper = mount(Host, { global: { plugins: [[VueQueryPlugin, { queryClient: client }]] } })
  return {
    selected,
    selectedIdentity,
    target,
    identity,
    isOpen,
    isExpanded,
    roster,
    billing,
    client,
    stop: () => {
      wrapper.unmount()
      client.clear()
    },
  }
}

describe('Workspace Settings reads', () => {
  beforeEach(() => {
    fetchRoster
      .mockReset()
      .mockImplementation(async (id: string) => [{ userId: id, role: 'owner' }])
    vi.mocked(fetchBillingAccount)
      .mockReset()
      .mockImplementation(async (id) => account(id))
  })

  it('shares a single roster/account request across the dialog and sections', async () => {
    const host = setupReads()
    await waitFor(() => expect(host.billing.account.value?.workspaceId).toBe('ws_1'))
    expect(host.roster.data.value?.[0]?.userId).toBe('ws_1')
    expect(fetchRoster).toHaveBeenCalledTimes(1)
    expect(fetchBillingAccount).toHaveBeenCalledTimes(1)
    host.stop()
  })

  it('supports independent roster mounting without resolving a Billing Account', async () => {
    const host = setupReads(false, true)
    await waitFor(() => expect(host.roster.data.value?.[0]?.userId).toBe('ws_1'))
    expect(fetchBillingAccount).not.toHaveBeenCalled()
    host.isExpanded.value = false
    expect(host.roster.data.value).toBeUndefined()
    host.stop()
  })

  it('falls back for a different workspace and reuses the shared read after restoring it', async () => {
    const host = setupReads()
    await waitFor(() => expect(host.billing.account.value?.workspaceId).toBe('ws_1'))
    host.target.value = 'ws_2'
    await waitFor(() => expect(host.billing.account.value?.workspaceId).toBe('ws_2'))
    expect(host.roster.data.value?.[0]?.userId).toBe('ws_2')
    host.target.value = 'ws_1'
    await waitFor(() => expect(host.billing.account.value?.workspaceId).toBe('ws_1'))
    expect(host.roster.data.value?.[0]?.userId).toBe('ws_1')
    expect(fetchRoster).toHaveBeenCalledTimes(2)
    expect(fetchBillingAccount).toHaveBeenCalledTimes(2)
    host.stop()
  })

  it('requires matching identity before reusing the shared account', async () => {
    const host = setupReads()
    await waitFor(() => expect(host.billing.account.value?.identity).toBe('user_1'))
    const next = deferred<BillingAccountScope>()
    vi.mocked(fetchBillingAccount).mockImplementationOnce(() => next.promise)
    host.identity.value = 'user_2'
    await nextTick()
    expect(host.billing.account.value).toBeNull()
    next.resolve(account('ws_1'))
    await waitFor(() => expect(host.billing.account.value?.identity).toBe('user_2'))
    expect(fetchRoster).toHaveBeenCalledTimes(1)
    expect(fetchBillingAccount).toHaveBeenCalledTimes(2)
    host.stop()
  })

  it('hides retained data and blocks retries while the parent is disabled', async () => {
    const host = setupReads()
    await waitFor(() => expect(host.billing.account.value).not.toBeNull())
    host.isOpen.value = false
    expect(host.roster.data.value).toBeUndefined()
    expect(host.billing.account.value).toBeNull()
    host.roster.retry()
    host.billing.retry()
    await flushPromises()
    expect(fetchRoster).toHaveBeenCalledTimes(1)
    expect(fetchBillingAccount).toHaveBeenCalledTimes(1)
    host.isOpen.value = true
    await waitFor(() => expect(host.billing.account.value?.workspaceId).toBe('ws_1'))
    host.stop()
  })

  it('ignores late responses from a workspace that is no longer selected', async () => {
    const late = deferred<BillingAccountScope>()
    const lateRoster = deferred<WorkspaceMemberEntry[]>()
    fetchRoster.mockImplementationOnce(() => lateRoster.promise)
    vi.mocked(fetchBillingAccount).mockImplementationOnce(() => late.promise)
    const host = setupReads()
    host.selected.value = 'ws_2'
    host.target.value = 'ws_2'
    await waitFor(() => expect(host.billing.account.value?.workspaceId).toBe('ws_2'))
    late.resolve(account('ws_1'))
    lateRoster.resolve([{ userId: 'old_member', role: 'owner' }])
    await flushPromises()
    expect(host.billing.account.value?.workspaceId).toBe('ws_2')
    expect(host.roster.data.value?.[0]?.userId).toBe('ws_2')
    host.stop()
  })

  it('retries failed account resolution without refetching the successful roster', async () => {
    vi.mocked(fetchBillingAccount).mockRejectedValueOnce(new Error('account unavailable'))
    const host = setupReads()
    await waitFor(() => expect(host.billing.error.value).not.toBeNull())
    host.roster.retry()
    host.billing.retry()
    await waitFor(() => expect(host.billing.account.value?.workspaceId).toBe('ws_1'))
    expect(fetchRoster).toHaveBeenCalledTimes(1)
    expect(fetchBillingAccount).toHaveBeenCalledTimes(2)
    host.stop()
  })

  it('retries only the failed read and leaves successful siblings untouched', async () => {
    fetchRoster.mockRejectedValueOnce(new Error('roster unavailable'))
    const host = setupReads()
    await waitFor(() => expect(host.roster.isError.value).toBe(true))
    await waitFor(() => expect(host.billing.account.value).not.toBeNull())
    host.billing.retry()
    host.roster.retry()
    await waitFor(() => expect(host.roster.data.value?.[0]?.userId).toBe('ws_1'))
    expect(fetchRoster).toHaveBeenCalledTimes(2)
    expect(fetchBillingAccount).toHaveBeenCalledTimes(1)
    host.stop()
  })
})
