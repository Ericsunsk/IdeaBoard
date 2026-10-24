import { computed, inject, provide, toValue, type InjectionKey, type MaybeRefOrGetter } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { workspaceMembersQueryOptions } from '../accountWorkspace'
import { useBillingAccountContext } from '../billing-account/useBillingAccountContext'

type RosterOptions = {
  workspaceId: MaybeRefOrGetter<string>
  isEnabled: MaybeRefOrGetter<boolean>
}
type AccountOptions = RosterOptions & {
  identity: MaybeRefOrGetter<string | null | undefined>
}

function useRosterRead(options: RosterOptions) {
  const query = useQuery({
    ...workspaceMembersQueryOptions(options.workspaceId),
    enabled: computed(() => toValue(options.isEnabled) && toValue(options.workspaceId) !== ''),
  })
  return {
    data: computed(() => (toValue(options.isEnabled) ? query.data.value : undefined)),
    isLoading: computed(() => toValue(options.isEnabled) && query.isLoading.value),
    isError: computed(() => toValue(options.isEnabled) && query.isError.value),
    retry: () => {
      if (toValue(options.isEnabled) && query.isError.value) void query.refetch()
    },
  }
}

type SettingsReads = {
  workspaceId: MaybeRefOrGetter<string>
  identity: MaybeRefOrGetter<string | null | undefined>
  isEnabled: MaybeRefOrGetter<boolean>
  roster: ReturnType<typeof useRosterRead>
  billingAccount: ReturnType<typeof useBillingAccountContext>
}
const settingsReadsKey: InjectionKey<SettingsReads> = Symbol('workspaceSettingsReads')

// The dialog owns eager shared reads. Sections use the same module when mounted alone.
export function provideWorkspaceSettingsReads(options: AccountOptions) {
  const roster = useRosterRead(options)
  const billingAccount = useBillingAccountContext(options)
  provide(settingsReadsKey, { ...options, roster, billingAccount })
  return billingAccount
}

function useSettingsScope(options: RosterOptions) {
  const shared = inject(settingsReadsKey, null)
  const isMatched = computed(
    () =>
      shared != null &&
      toValue(options.workspaceId) !== '' &&
      toValue(shared.workspaceId) === toValue(options.workspaceId),
  )
  const isEnabled = computed(
    () =>
      toValue(options.isEnabled) &&
      toValue(options.workspaceId) !== '' &&
      (!shared || toValue(shared.isEnabled)),
  )
  return { shared, isMatched, isEnabled }
}

export function useWorkspaceSettingsRoster(options: RosterOptions) {
  const { shared, isMatched, isEnabled } = useSettingsScope(options)
  const own = useRosterRead({
    workspaceId: options.workspaceId,
    isEnabled: computed(() => isEnabled.value && !isMatched.value),
  })
  const active = computed(() => (isMatched.value && shared ? shared.roster : own))
  return {
    data: computed(() => (isEnabled.value ? active.value.data.value : undefined)),
    isLoading: computed(() => isEnabled.value && active.value.isLoading.value),
    isError: computed(() => isEnabled.value && active.value.isError.value),
    retry: () => {
      if (isEnabled.value) active.value.retry()
    },
  }
}

export function useWorkspaceSettingsBillingAccount(options: AccountOptions) {
  const { shared, isMatched: isWorkspaceMatched, isEnabled } = useSettingsScope(options)
  const isMatched = computed(
    () =>
      isWorkspaceMatched.value &&
      shared != null &&
      toValue(shared.identity) === toValue(options.identity),
  )
  const own = useBillingAccountContext({
    ...options,
    isEnabled: computed(() => isEnabled.value && !isMatched.value),
  })
  const active = computed(() => (isMatched.value && shared ? shared.billingAccount : own))
  return {
    account: computed(() => (isEnabled.value ? active.value.account.value : null)),
    error: computed(() => (isEnabled.value ? active.value.error.value : null)),
    isLoading: computed(() => isEnabled.value && active.value.isLoading.value),
    retry: () => {
      if (isEnabled.value && active.value.error.value != null) void active.value.retry()
    },
  }
}
