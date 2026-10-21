import { computed, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { workspaceBillingTransferIdempotencyKey } from '../../../../api/idempotencyKey'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'
import {
  invalidateWorkspaceMemberQueries,
  workspaceMembersQueryOptions,
  workspaceQueryKeys,
} from '../accountWorkspace'
import { billingAccountQueryKeys } from '../billing-account/resource'
import { useBillingAccountContext } from '../billing-account/useBillingAccountContext'
import { creditsQueryKeys } from '../credits/resource'
import { memberQuotaQueryKeys } from '../quota/resource'
import { workspaceBudgetQueryKeys } from '../budget/resource'
import {
  acceptTransferInvite,
  billingTransferQueryKeys,
  cancelTransferInvite,
  detachBillingMember,
  fetchBillingManagers,
  fetchTransferInvites,
  initiateTransferInvite,
  rejectTransferInvite,
  type BillingManagerSummary,
  type BillingTransferInviteSummary,
} from './resource'

type Decision = 'accept' | 'reject' | 'cancel'

function useTransferQueries(workspaceId: () => string, identity: () => string) {
  const isEnabled = computed(() => workspaceId() !== '' && identity() !== '')
  const accountContext = useBillingAccountContext({ workspaceId, identity, isEnabled })
  const invitesQuery = useQuery({
    queryKey: computed(() => [...billingTransferQueryKeys.list(workspaceId()), identity()]),
    queryFn: ({ signal }) => fetchTransferInvites(workspaceId(), { signal }),
    enabled: isEnabled,
    ...accountScoped(),
    ...noRetry(),
  })
  const managersQuery = useQuery({
    queryKey: computed(() => billingTransferQueryKeys.members(workspaceId(), identity())),
    queryFn: ({ signal }) => fetchBillingManagers(workspaceId(), signal),
    enabled: computed(() => isEnabled.value && accountContext.consumptionScope.value === 'account'),
    ...accountScoped(),
    ...noRetry(),
  })
  const rosterQuery = useQuery({
    ...workspaceMembersQueryOptions(workspaceId),
    enabled: isEnabled,
  })
  return { accountContext, invitesQuery, managersQuery, rosterQuery }
}

export function useBillingTransferSection(options: {
  workspaceId: MaybeRefOrGetter<string>
  currentUserId: MaybeRefOrGetter<string | null | undefined>
}) {
  const { t } = useI18n()
  const toast = useAppToast()
  const client = useQueryClient()
  const workspaceId = () => toValue(options.workspaceId)
  const identity = () => toValue(options.currentUserId) ?? ''
  const { accountContext, invitesQuery, managersQuery, rosterQuery } = useTransferQueries(
    workspaceId,
    identity,
  )
  const account = accountContext.account
  const invites = computed(() => invitesQuery.data.value ?? [])
  const managers = computed(() =>
    account.value?.consumptionScope === 'account' ? (managersQuery.data.value ?? []) : [],
  )
  const myRole = computed(() => managers.value.find((member) => member.userId === identity())?.role)
  const hasPendingInvite = computed(() =>
    invites.value.some((invite) => invite.status === 'pending'),
  )
  const hasLoadError = computed(
    () =>
      accountContext.error.value != null ||
      invitesQuery.isError.value ||
      rosterQuery.isError.value ||
      (account.value?.consumptionScope === 'account' && managersQuery.isError.value),
  )
  const isLoading = computed(
    () =>
      accountContext.isLoading.value ||
      invitesQuery.isLoading.value ||
      rosterQuery.isLoading.value ||
      (account.value?.consumptionScope === 'account' && managersQuery.isLoading.value),
  )
  const isSaving = ref(false)
  const actionError = ref<string | null>(null)
  const recipientId = ref('')
  const confirmation = ref<
    | { kind: Decision; invite: BillingTransferInviteSummary }
    | { kind: 'detach'; member: BillingManagerSummary }
    | null
  >(null)
  const canInitiate = computed(
    () => myRole.value === 'owner' && !hasPendingInvite.value && !hasLoadError.value,
  )
  const recipients = computed(() =>
    (rosterQuery.data.value ?? []).filter((member) => member.userId !== identity()),
  )
  const canInvite = computed(
    () =>
      canInitiate.value && recipients.value.some((member) => member.userId === recipientId.value),
  )
  let intent: { signature: string; key: string } | null = null
  let scopeVersion = 0

  watch([workspaceId, identity], () => {
    ++scopeVersion
    recipientId.value = ''
    confirmation.value = null
    actionError.value = null
    intent = null
  })
  watch(recipientId, () => {
    intent = null
    actionError.value = null
  })

  function memberName(userId: string): string {
    return rosterQuery.data.value?.find((member) => member.userId === userId)?.email || userId
  }
  function canDecide(invite: BillingTransferInviteSummary, decision: Decision): boolean {
    if (invite.status !== 'pending' || invite.billingAccountId !== account.value?.billingAccountId)
      return false
    return decision === 'cancel' ? invite.fromUserId === identity() : invite.toUserId === identity()
  }
  function canDetach(member: BillingManagerSummary): boolean {
    if (!myRole.value || hasLoadError.value) return false
    if (
      member.role === 'owner' &&
      managers.value.filter((manager) => manager.role === 'owner').length <= 1
    )
      return false
    return (
      myRole.value === 'owner' ||
      member.userId === identity() ||
      (myRole.value === 'admin' && member.role === 'admin')
    )
  }
  function requestDecision(invite: BillingTransferInviteSummary, kind: Decision): void {
    if (!isSaving.value && canDecide(invite, kind)) confirmation.value = { kind, invite }
  }
  function requestDetach(member: BillingManagerSummary): void {
    if (!isSaving.value && canDetach(member)) confirmation.value = { kind: 'detach', member }
  }
  function dismissConfirmation(): void {
    if (!isSaving.value) confirmation.value = null
  }
  function retry(): void {
    void accountContext.retry()
    void invitesQuery.refetch()
    void rosterQuery.refetch()
    if (account.value?.consumptionScope === 'account') void managersQuery.refetch()
  }
  async function invalidate(
    scopeId: string,
    changed?: BillingTransferInviteSummary,
  ): Promise<void> {
    for (const id of new Set([scopeId, ...(changed?.scopeWorkspaceIds ?? [])])) {
      invalidateWorkspaceMemberQueries(client, id)
    }
    await Promise.all(
      [
        billingTransferQueryKeys.all,
        billingAccountQueryKeys.all,
        workspaceQueryKeys.list,
        creditsQueryKeys.all,
        memberQuotaQueryKeys.all,
        workspaceBudgetQueryKeys.all,
      ].map((queryKey) => client.invalidateQueries({ queryKey })),
    )
  }
  async function runMutation(
    operation: 'invite' | Decision | 'detach',
    target: string,
    run: (scopeId: string, key: string | undefined) => Promise<BillingTransferInviteSummary | void>,
  ): Promise<void> {
    if (isSaving.value) return
    const scopeId = workspaceId()
    const version = scopeVersion
    const signature = `${scopeId}:${identity()}:${operation}:${target}`
    if (operation !== 'detach' && intent?.signature !== signature) {
      intent = { signature, key: workspaceBillingTransferIdempotencyKey(operation) }
    }
    isSaving.value = true
    actionError.value = null
    try {
      const changed = await run(scopeId, operation === 'detach' ? undefined : intent?.key)
      await invalidate(scopeId, changed ?? undefined)
      if (version !== scopeVersion) return
      confirmation.value = null
      intent = null
      toast.success(t('workspace.switcher.handover.saved'))
    } catch (error) {
      if (version !== scopeVersion) return
      actionError.value = userFacingErrorCopy(
        error,
        t(`workspace.switcher.handover.${operation === 'invite' ? 'initiate' : operation}Failed`),
      )
      toast.error(actionError.value)
    } finally {
      isSaving.value = false
    }
  }
  async function initiate(): Promise<void> {
    if (!canInvite.value) return
    const target = recipientId.value
    await runMutation('invite', target, (id, key) =>
      initiateTransferInvite(id, target, { idempotencyKey: key }),
    )
  }
  async function confirm(): Promise<void> {
    const pending = confirmation.value
    if (!pending) return
    if (pending.kind === 'detach') {
      if (!canDetach(pending.member)) return
      const userId = pending.member.userId
      await runMutation('detach', userId, (id) => detachBillingMember(id, userId))
    } else {
      if (!canDecide(pending.invite, pending.kind)) return
      const { kind, invite } = pending
      const decide = {
        accept: acceptTransferInvite,
        reject: rejectTransferInvite,
        cancel: cancelTransferInvite,
      }[kind]
      await runMutation(kind, invite.publicId, (id, key) =>
        decide(id, invite.publicId, { idempotencyKey: key }),
      )
    }
  }
  return {
    account,
    invites,
    managers,
    isLoading,
    hasLoadError,
    retry,
    isSaving,
    actionError,
    canInitiate,
    canInvite,
    recipientId,
    recipients,
    memberName,
    canDecide,
    canDetach,
    requestDecision,
    requestDetach,
    confirmation,
    dismissConfirmation,
    initiate,
    confirm,
  }
}
