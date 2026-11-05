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
  // Actor identity for the Org role matrix (Owner sets Admin, Owner/Admin set
  // Member, Admin never touches Owner/Admin, Owner rows stay unlimited).
  // Organization facts are supplied by the Workspace owner of this surface.
  currentUserId?: MaybeRefOrGetter<string | null | undefined> | undefined
  organizationMembers: MaybeRefOrGetter<
    readonly { userId: string; role: 'owner' | 'admin' | 'member' }[]
  >
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
    // Managers render the list, never the self block. Keep the settings read
    // budget at four GETs including Organization membership and shared reads.
    enabled: computed(
      () => toValue(options.isExpanded) && !canManage.value && workspaceIdOf() !== '',
    ),
    ...accountScoped(),
    ...noRetry(),
  })

  const isListEnabled = computed(
    () => toValue(options.isExpanded) && canManage.value && workspaceIdOf() !== '',
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

  function memberRole(userId: string) {
    return (
      toValue(options.organizationMembers).find((entry) => entry.userId === userId)?.role ?? null
    )
  }

  const actorRole = computed(() => {
    const current = toValue(options.currentUserId)?.trim()
    if (!current) return null
    return memberRole(current)
  })

  // Org adjust matrix (mirrors the backend CanSet policy, fail-closed):
  // Owner rows stay unlimited (never adjustable); an Admin actor adjusts
  // Member rows only (Owner sets Admin, Owner/Admin set Member); unknown or
  // non-manager actors adjust nothing. canAdjustTarget is the single adjust
  // truth: visibility gates the manager surface, target roles gate the row.
  function canAdjustTarget(targetUserId: string): boolean {
    if (!canManage.value) return false
    const actor = actorRole.value
    const targetRole = memberRole(targetUserId)
    if (actor !== 'owner' && actor !== 'admin') return false
    if (targetRole == null) return false
    if (targetRole === 'owner') return false
    if (actor === 'admin' && targetRole === 'admin') return false
    return true
  }

  // Single fail-closed guard for picking and saving a target:
  // forbidden actor/target pairs toast the same way and never send;
  // backend still enforces 403.
  function guardAdjustTarget(targetUserId: string): boolean {
    if (!canAdjustTarget(targetUserId)) {
      toast.error(t('workspace.quota.saveFailed'))
      return false
    }
    return true
  }

  function pickTargetUser(value: unknown): void {
    saveError.value = null
    const next = typeof value === 'string' ? value.trim() : ''
    if (!next) {
      targetUserId.value = ''
      return
    }
    // Whitelist to the dialog options (roster + listed quotas): arbitrary
    // strings never become a target, even when called programmatically.
    if (!memberOptions.value.includes(next)) {
      toast.error(t('workspace.quota.saveFailed'))
      return
    }
    // Disabled options (Owner rows, Admin-actor Owner/Admin rows) reject the
    // programmatic pick the same way instead of arming a forbidden save.
    if (!guardAdjustTarget(next)) {
      return
    }
    targetUserId.value = next
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

  async function saveQuota(): Promise<boolean> {
    if (isSaving.value) return false
    const userId = targetUserId.value.trim()
    if (!userId) {
      saveError.value = t('workspace.quota.pickMember')
      return false
    }
    // Front-stop the Org role matrix before any transport: forbidden
    // actor/target pairs never send (backend still enforces 403).
    if (!guardAdjustTarget(userId)) {
      return false
    }
    let input: UpdateMemberQuotaInput
    const period = targetPeriod.value
    if (period === 'unlimited') {
      input = { period: 'unlimited' }
    } else {
      const limit = validateQuotaLimit(limitDraft.value)
      if (limit == null) {
        saveError.value = t('workspace.quota.invalidLimit', { max: MAX_MEMBER_QUOTA_LIMIT })
        return false
      }
      input = { period, limitCredits: limit }
    }
    isSaving.value = true
    saveError.value = null
    try {
      await updateMemberQuota(workspaceIdOf(), userId, input)
      await invalidateQuotaQueries()
      toast.success(t('workspace.quota.saved'))
      return true
    } catch (error) {
      toast.error(describeSaveError(error))
      return false
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
    targetUserId,
    targetPeriod,
    limitDraft,
    isSaving,
    saveError,
    pickTargetUser,
    pickTargetPeriod,
    saveQuota,
    memberRole,
    canAdjustTarget,
  }
}
