import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import {
  CAPPED_QUOTA_PERIODS,
  DEFAULT_QUOTA_PRESETS,
  defaultMemberQuotaQueryKeys,
  fetchDefaultMemberQuota,
  MAX_MEMBER_QUOTA_LIMIT,
  parseQuotaPeriodValue,
  updateDefaultMemberQuota,
  validateQuotaLimit,
  type DefaultMemberQuotaSummary,
  type MemberQuotaPeriod,
  type UpdateDefaultMemberQuotaInput,
} from './resource'

// Account default new-member quota section controller (#947). The default
// rides TanStack Query on the workspace default key; writes go through the
// default adapter and converge by invalidation — never optimistic. Only
// billing managers see management controls (`canManage` derives from the
// billing-account consumption scope the dialog resolves); the backend
// still enforces 403 for every write. Absent defaults surface the
// fail-closed guidance (409 default_member_quota_not_configured) instead
// of an implicit unlimited.
type DefaultMemberQuotaSectionOptions = {
  workspaceId: MaybeRefOrGetter<string>
  /** Billing-account consumption scope: 'account' means the caller is a billing manager. */
  consumptionScope: MaybeRefOrGetter<string | null | undefined>
  isExpanded: MaybeRefOrGetter<boolean>
}

export function useDefaultMemberQuotaSection(options: DefaultMemberQuotaSectionOptions) {
  const { t } = useI18n()
  const toast = useAppToast()
  const queryClient = useQueryClient()

  const workspaceIdOf = (): string => toValue(options.workspaceId).trim()
  const canManage = computed(() => toValue(options.consumptionScope) === 'account')

  const detailQuery = useQuery({
    queryKey: computed(() => defaultMemberQuotaQueryKeys.detail(workspaceIdOf())),
    queryFn: ({ signal }) => fetchDefaultMemberQuota(workspaceIdOf(), signal),
    enabled: computed(() => toValue(options.isExpanded) && workspaceIdOf() !== ''),
    ...accountScoped(),
    ...noRetry(),
  })

  const policy = computed<DefaultMemberQuotaSummary | null>(() => detailQuery.data.value ?? null)
  const isLoading = computed(() => detailQuery.isLoading.value)
  const hasLoadError = computed(() => detailQuery.isError.value)
  const isNotConfigured = computed(
    () => userErrorCode(detailQuery.error.value) === 'default_member_quota_not_configured',
  )

  function retry(): void {
    void detailQuery.refetch()
  }

  function invalidate(): Promise<void> {
    return queryClient
      .invalidateQueries({ queryKey: defaultMemberQuotaQueryKeys.detail(workspaceIdOf()) })
      .then(() => undefined)
  }

  const targetPeriod = ref<MemberQuotaPeriod>('monthly')
  const limitDraft = ref('')
  const isSaving = ref(false)
  const saveError = ref<string | null>(null)

  function pickTargetPeriod(value: unknown): void {
    saveError.value = null
    const period = parseQuotaPeriodValue(value)
    if (period != null) {
      targetPeriod.value = period
    }
  }

  function pickPreset(limit: number): void {
    saveError.value = null
    targetPeriod.value = 'monthly'
    limitDraft.value = String(limit)
  }

  function describeSaveError(error: unknown): string {
    const code = userErrorCode(error)
    return code != null
      ? `${t('workspace.quota.defaultSaveFailed')} (${code})`
      : t('workspace.quota.defaultSaveFailed')
  }

  async function saveDefault(): Promise<void> {
    if (isSaving.value) return
    const period = targetPeriod.value
    let input: UpdateDefaultMemberQuotaInput
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
      await updateDefaultMemberQuota(workspaceIdOf(), input)
      await invalidate()
      toast.success(t('workspace.quota.defaultSaved'))
    } catch (error) {
      saveError.value = describeSaveError(error)
      toast.error(saveError.value)
    } finally {
      isSaving.value = false
    }
  }

  return {
    policy,
    isLoading,
    hasLoadError,
    isNotConfigured,
    retry,
    canManage,
    cappedPeriods: CAPPED_QUOTA_PERIODS,
    presets: DEFAULT_QUOTA_PRESETS,
    targetPeriod,
    limitDraft,
    isSaving,
    saveError,
    pickTargetPeriod,
    pickPreset,
    saveDefault,
  }
}
