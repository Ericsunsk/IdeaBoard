import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import {
  CAPPED_QUOTA_PERIODS,
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
import { workspaceMembersQueryOptions, type WorkspaceMemberEntry } from '../accountWorkspace'

// Member quota section controller for the workspace settings dialog. The
// self read rides TanStack Query for every member; the manager list and
// the roster picker converge by invalidation — never optimistic. Only
// billing managers see management controls (`canManage` derives from the
// billing-account consumption scope the dialog resolves); the backend
// still enforces 403 for every write.
type MemberQuotaSectionOptions = {
  workspaceId: MaybeRefOrGetter<string>
  /** Billing-account consumption scope: 'account' means the caller is a billing manager. */
  consumptionScope: MaybeRefOrGetter<string | null | undefined>
  isExpanded: MaybeRefOrGetter<boolean>
}

export function useMemberQuotaSection(options: MemberQuotaSectionOptions) {
  const { t } = useI18n()
  const toast = useAppToast()
  const queryClient = useQueryClient()

  const workspaceIdOf = (): string => toValue(options.workspaceId).trim()
  const canManage = computed(() => toValue(options.consumptionScope) === 'account')

  const selfQuery = useQuery({
    queryKey: computed(() => memberQuotaQueryKeys.self(workspaceIdOf())),
    queryFn: ({ signal }) => fetchSelfMemberQuota(workspaceIdOf(), signal),
    enabled: computed(() => toValue(options.isExpanded) && workspaceIdOf() !== ''),
    ...accountScoped(),
    ...noRetry(),
  })

  const listQuery = useQuery({
    queryKey: computed(() => memberQuotaQueryKeys.list(workspaceIdOf())),
    queryFn: ({ signal }) => fetchMemberQuotaList(workspaceIdOf(), signal),
    enabled: computed(
      () => toValue(options.isExpanded) && canManage.value && workspaceIdOf() !== '',
    ),
    ...accountScoped(),
    ...noRetry(),
  })

  const rosterBaseOptions = workspaceMembersQueryOptions(options.workspaceId)
  const rosterQuery = useQuery({
    ...rosterBaseOptions,
    enabled: computed(
      () =>
        toValue(options.isExpanded) &&
        canManage.value &&
        toValue(rosterBaseOptions.enabled ?? true),
    ),
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
      queryClient.invalidateQueries({ queryKey: memberQuotaQueryKeys.self(workspaceIdOf()) }),
      queryClient.invalidateQueries({ queryKey: memberQuotaQueryKeys.list(workspaceIdOf()) }),
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
    isSaving.value = true
    saveError.value = null
    try {
      await updateMemberQuota(workspaceIdOf(), userId, input)
      await invalidateQuotaQueries()
      toast.success(t('workspace.quota.saved'))
    } catch (error) {
      saveError.value = describeSaveError(error)
      toast.error(saveError.value)
    } finally {
      isSaving.value = false
    }
  }

  async function clearQuota(userId: string): Promise<void> {
    if (isSaving.value) return
    isSaving.value = true
    saveError.value = null
    try {
      await updateMemberQuota(workspaceIdOf(), userId, { period: 'unlimited' })
      await invalidateQuotaQueries()
      toast.success(t('workspace.quota.saved'))
    } catch (error) {
      saveError.value = describeSaveError(error)
      toast.error(saveError.value)
    } finally {
      isSaving.value = false
    }
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
    cappedPeriods: CAPPED_QUOTA_PERIODS,
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
