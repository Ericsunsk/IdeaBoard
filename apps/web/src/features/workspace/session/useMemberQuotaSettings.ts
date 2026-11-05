import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery, type QueryFunctionContext } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { useConfirmDialog, type ConfirmPort } from '@/core/composables/useConfirmDialog'
import { useFailureToast } from '@/core/composables/useFailureToast'
import { accountScoped, noRetry } from '@/core/query/queryDefaults'
import { useMemberQuotaSection } from '@/features/account'
import type { MemberQuotaSummary } from '@/features/account/session/quota/resource'
import { confirmSettingsSave } from '@/features/admin/shared'
import {
  fetchOrganizationMembers,
  workspaceQueryKeys,
  type OrganizationMemberSummary,
} from '../selection/workspaceResource'

type MemberQuotaSettingsOptions = {
  workspaceId: MaybeRefOrGetter<string>
  organizationId: MaybeRefOrGetter<string>
  currentUserId: MaybeRefOrGetter<string | null | undefined>
  canManage: MaybeRefOrGetter<boolean>
  isExpanded: MaybeRefOrGetter<boolean>
}

// Workspace owns Organization facts and their viewer provenance. Account owns
// quota policy; neither a Workspace role nor retained failed role data grants it.
function useQuotaOrganizationMembers(options: MemberQuotaSettingsOptions) {
  const isEnabled = computed(
    () =>
      toValue(options.canManage) &&
      toValue(options.isExpanded) &&
      toValue(options.organizationId) !== '' &&
      !!toValue(options.currentUserId),
  )
  const query = useQuery(
    computed(() => {
      const organizationId = toValue(options.organizationId)
      const identity = toValue(options.currentUserId) ?? ''
      return {
        queryKey: [...workspaceQueryKeys.organizationMembers(organizationId), identity],
        queryFn: ({ queryKey, signal }: QueryFunctionContext) =>
          fetchOrganizationMembers(String(queryKey[1]), { signal }),
        select: (members: OrganizationMemberSummary[]) => ({ organizationId, identity, members }),
        // eslint-disable-next-line governance/boolean-prefix -- TanStack Query's enabled option.
        enabled: isEnabled.value,
        ...accountScoped(),
        ...noRetry(),
      }
    }),
  )
  const members = computed(() => {
    if (!isEnabled.value || query.isError.value) return []
    const facts = query.data.value
    return facts?.organizationId === toValue(options.organizationId) &&
      facts.identity === toValue(options.currentUserId)
      ? facts.members
      : []
  })
  return { query, members }
}

// The UI chooses a scope and raises editor intents. This module owns role
// readiness, draft setup, confirmation, and successful dismissal as one flow.
export function useMemberQuotaSettings(options: MemberQuotaSettingsOptions) {
  const { t } = useI18n()
  const { query: membersQuery, members } = useQuotaOrganizationMembers(options)
  const quota = useMemberQuotaSection({ ...options, organizationMembers: members })
  const isListLoading = computed(() => quota.isListLoading.value || membersQuery.isLoading.value)
  const hasListError = computed(() => quota.hasListError.value || membersQuery.isError.value)
  const isEditDialogVisible = ref(false)
  const editMode = ref<'create' | 'edit'>('create')
  const clearConfirm = useConfirmDialog()
  const clearConfirmPort: ConfirmPort = {
    confirm: (request) => clearConfirm.confirmAsync(request),
  }

  useFailureToast(
    () => quota.hasSelfError.value,
    () => t('workspace.quota.loadFailed'),
  )

  const editor = computed(() => ({
    isVisible: isEditDialogVisible.value,
    mode: editMode.value,
    member: quota.targetUserId.value,
    period: quota.targetPeriod.value,
    limit: quota.limitDraft.value,
    error: quota.saveError.value,
    members: quota.memberOptions.value.map((userId) => ({
      label: quota.memberLabel(userId),
      value: userId,
      // eslint-disable-next-line governance/boolean-prefix -- SelectOptionItem contract.
      disabled: !quota.canAdjustTarget(userId),
    })),
  }))
  const confirmation = computed(() => ({
    isVisible: clearConfirm.visible.value,
    title: clearConfirm.title.value,
    message: clearConfirm.message.value,
    confirmLabel: clearConfirm.confirmLabel.value,
    intent: clearConfirm.intent.value,
    isLoading: clearConfirm.loading.value,
  }))

  function retryList(): void {
    if (quota.hasListError.value) quota.retryList()
    if (membersQuery.isError.value) void membersQuery.refetch()
  }

  function openCreateDialog(): void {
    quota.pickTargetUser('')
    quota.pickTargetPeriod('daily')
    quota.limitDraft.value = ''
    editMode.value = 'create'
    isEditDialogVisible.value = true
  }

  function openEditDialog(
    row: Pick<MemberQuotaSummary, 'userId' | 'period' | 'limitCredits'>,
  ): void {
    if (!quota.canAdjustTarget(row.userId)) {
      quota.pickTargetUser(row.userId)
      return
    }
    quota.pickTargetUser(row.userId)
    quota.pickTargetPeriod(row.period)
    quota.limitDraft.value = row.limitCredits != null ? String(row.limitCredits) : ''
    editMode.value = 'edit'
    isEditDialogVisible.value = true
  }

  function closeEditDialog(): void {
    isEditDialogVisible.value = false
  }

  function setLimitDraft(value: string): void {
    quota.limitDraft.value = value
  }

  function dismissConfirmation(isVisible: boolean): void {
    if (!isVisible) clearConfirm.close()
  }

  async function submitEditDialog(): Promise<void> {
    const userId = quota.targetUserId.value.trim()
    if (!userId || !quota.canAdjustTarget(userId)) {
      await quota.saveQuota()
      return
    }
    if (quota.targetPeriod.value === 'unlimited') {
      const isConfirmed = await confirmSettingsSave(clearConfirmPort, {
        title: t('workspace.quota.clearConfirmTitle'),
        message: t('workspace.quota.clearConfirmMessage', { name: quota.memberLabel(userId) }),
        confirmLabel: t('workspace.quota.clearConfirmAction'),
      })
      if (!isConfirmed) return
    }
    // Account rechecks the live capability and Org roles after the await.
    if (await quota.saveQuota()) closeEditDialog()
  }

  return {
    canManage: quota.canManage,
    selfQuota: quota.selfQuota,
    isSelfLoading: quota.isSelfLoading,
    hasSelfError: quota.hasSelfError,
    isSelfExhausted: quota.isSelfExhausted,
    retrySelf: quota.retrySelf,
    quotas: quota.quotas,
    memberLabel: quota.memberLabel,
    memberRole: quota.memberRole,
    canAdjustTarget: quota.canAdjustTarget,
    isSaving: computed(() => quota.isSaving.value),
    isListLoading,
    hasListError,
    retryList,
    editor,
    confirmation,
    openCreateDialog,
    openEditDialog,
    closeEditDialog,
    pickTargetUser: quota.pickTargetUser,
    pickTargetPeriod: quota.pickTargetPeriod,
    setLimitDraft,
    submitEditDialog,
    confirmUnlimited: clearConfirm.confirm,
    dismissConfirmation,
  }
}
