import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import {
  fetchMemberQuotaList,
  fetchSelfMemberQuota,
  isQuotaExhausted as isExhausted,
  MAX_MEMBER_QUOTA_LIMIT,
  memberQuotaQueryKeys,
  parseQuotaPeriodValue,
  updateMemberQuota,
  validateQuotaLimit,
  type MemberQuotaPeriod,
  type MemberQuotaSummary,
  type UpdateMemberQuotaInput,
} from './resource'
import type { WorkspaceMemberEntry } from '../accountWorkspace'
import { useWorkspaceSettingsRoster } from '../workspace-settings'

// The account projection supplies the capability using the server quota policy.
// Consumption visibility and unrelated Organization roles never grant it.
type MemberQuotaSectionOptions = {
  workspaceId: MaybeRefOrGetter<string>
  canManage: MaybeRefOrGetter<boolean>
  isExpanded: MaybeRefOrGetter<boolean>
  // Management forms stay behind the local toggle: the list (and its roster
  // assistance) only load once a manager expands them.
  isListEnabled?: MaybeRefOrGetter<boolean> | undefined
}

export function useMemberQuotaSection(options: MemberQuotaSectionOptions) {
  const { t } = useI18n()
  const toast = useAppToast()
  const queryClient = useQueryClient()

  const workspaceIdOf = (): string => toValue(options.workspaceId).trim()
  const canManage = computed(() => toValue(options.canManage))

  const selfQuery = useQuery({
    queryKey: computed(() => memberQuotaQueryKeys.self(workspaceIdOf())),
    queryFn: ({ signal }) => fetchSelfMemberQuota(workspaceIdOf(), signal),
    enabled: computed(() => toValue(options.isExpanded) && workspaceIdOf() !== ''),
    ...accountScoped(),
    ...noRetry(),
  })

  const isListEnabled = computed(
    () =>
      toValue(options.isExpanded) &&
      canManage.value &&
      (options.isListEnabled == null || toValue(options.isListEnabled)) &&
      workspaceIdOf() !== '',
  )

  const listQuery = useQuery({
    queryKey: computed(() => memberQuotaQueryKeys.list(workspaceIdOf())),
    queryFn: ({ signal }) => fetchMemberQuotaList(workspaceIdOf(), signal),
    enabled: isListEnabled,
    ...accountScoped(),
    ...noRetry(),
  })

  const rosterQuery = useWorkspaceSettingsRoster({
    workspaceId: workspaceIdOf,
    isEnabled: isListEnabled,
  })

  const selfQuota = computed<MemberQuotaSummary | null>(() => selfQuery.data.value ?? null)
  const quotas = computed<MemberQuotaSummary[]>(() => listQuery.data.value ?? [])
  const roster = computed<WorkspaceMemberEntry[]>(() => rosterQuery.data.value ?? [])
  const isSelfExhausted = computed(() => (selfQuota.value ? isExhausted(selfQuota.value) : false))

  function retrySelf(): void {
    void selfQuery.refetch()
  }

  function retryList(): void {
    void listQuery.refetch()
  }

  function invalidateQuotaQueries(): Promise<void> {
    return Promise.all([
      queryClient.invalidateQueries({
        queryKey: memberQuotaQueryKeys.self(workspaceIdOf()),
        exact: true,
      }),
      queryClient.invalidateQueries({
        queryKey: memberQuotaQueryKeys.list(workspaceIdOf()),
        exact: true,
      }),
    ]).then(() => undefined)
  }

  const memberOptions = computed<string[]>(() => {
    const seen = new Set<string>()
    for (const entry of roster.value) seen.add(entry.userId)
    for (const row of quotas.value) seen.add(row.userId)
    return [...seen].sort()
  })

  function memberLabel(userId: string): string {
    return roster.value.find((entry) => entry.userId === userId)?.email || userId
  }

  const targetUserId = ref('')
  const targetPeriod = ref<MemberQuotaPeriod>('daily')
  const limitDraft = ref('')
  const isSaving = ref(false)
  const saveError = ref<string | null>(null)

  function pickTargetUser(value: unknown): void {
    saveError.value = null
    targetUserId.value = typeof value === 'string' ? value.trim() : ''
  }

  function pickTargetPeriod(value: unknown): void {
    saveError.value = null
    const period = parseQuotaPeriodValue(value)
    if (period != null) {
      targetPeriod.value = period
    }
  }

  function describeSaveError(error: unknown): string {
    const code = userErrorCode(error)
    return code != null
      ? `${t('workspace.quota.saveFailed')} (${code})`
      : t('workspace.quota.saveFailed')
  }

  async function saveQuota(): Promise<void> {
    if (isSaving.value) return
    const userId = targetUserId.value.trim()
    if (!userId) {
      saveError.value = t('workspace.quota.pickMember')
      return
    }
    let input: UpdateMemberQuotaInput
    const period = targetPeriod.value
    if (period === 'unlimited') {
      input = { period: 'unlimited' }
    } else {
      const limit = validateQuotaLimit(limitDraft.value)
      if (limit == null) {
        saveError.value = t('workspace.quota.invalidLimit', { max: MAX_MEMBER_QUOTA_LIMIT })
        return
      }
      input = { period, limitCredits: limit }
    }
    await persistQuota(userId, input)
  }

  async function persistQuota(userId: string, input: UpdateMemberQuotaInput): Promise<void> {
    if (isSaving.value || !canManage.value) return
    isSaving.value = true
    saveError.value = null
    try {
      await updateMemberQuota(workspaceIdOf(), userId, input)
      await invalidateQuotaQueries()
      toast.success(t('workspace.quota.saved'))
    } catch (error) {
      toast.error(describeSaveError(error))
    } finally {
      isSaving.value = false
    }
  }

  function clearQuota(userId: string): Promise<void> {
    return persistQuota(userId, { period: 'unlimited' })
  }

  return {
    selfQuota,
    isSelfLoading: computed(() => selfQuery.isLoading.value),
    hasSelfError: computed(() => selfQuery.isError.value),
    isSelfExhausted,
    retrySelf,
    quotas,
    isListLoading: computed(() => listQuery.isLoading.value),
    hasListError: computed(() => listQuery.isError.value),
    retryList,
    canManage,
    memberOptions,
    memberLabel,
    targetUserId,
    targetPeriod,
    limitDraft,
    isSaving,
    saveError,
    pickTargetUser,
    pickTargetPeriod,
    saveQuota,
    clearQuota,
  }
}
