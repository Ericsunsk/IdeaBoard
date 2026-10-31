import { computed, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { workspaceBillingTransferIdempotencyKey } from '@/api/http/idempotencyKey'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { userFacingErrorCopy } from '../../../../core/errors/errorCatalog'
import { accountScoped, noRetry } from '../../../../core/query/queryDefaults'
import { invalidateWorkspaceMemberQueries, workspaceQueryKeys } from '../accountWorkspace'
import { billingAccountQueryKeys } from '../billing-account/resource'
import { billingRecordsQueryKeys, isBillingRecordsQueryForHandover } from '../billing/resource'
import { memberQuotaQueryKeys } from '../quota/resource'
import { isUsageQueryForHandover, usageQueryKeys } from '../usage/resource'
import {
  useWorkspaceSettingsBillingAccount,
  useWorkspaceSettingsRoster,
} from '../workspace-settings'
import {
  acceptTransferInvite,
  billingTransferQueryKeys,
  cancelTransferInvite,
  fetchBillingManagers,
  fetchTransferInvites,
  initiateTransferInvite,
  isTransferInviteLive,
  rejectTransferInvite,
  type BillingTransferInviteSummary,
} from './resource'

type Decision = 'accept' | 'reject' | 'cancel'

type Confirmation =
  | { kind: Decision; invite: BillingTransferInviteSummary }
  | { kind: 'invite'; targetUserId: string }

function useTransferQueries(
  workspaceId: () => string,
  identity: () => string,
  isEnabledGate: () => boolean,
) {
  const isEnabled = computed(() => isEnabledGate() && workspaceId() !== '' && identity() !== '')
  const accountContext = useWorkspaceSettingsBillingAccount({ workspaceId, identity, isEnabled })
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
    enabled: computed(
      () => isEnabled.value && accountContext.account.value?.consumptionScope === 'account',
    ),
    ...accountScoped(),
    ...noRetry(),
  })
  const rosterQuery = useWorkspaceSettingsRoster({ workspaceId, isEnabled })
  return { accountContext, invitesQuery, managersQuery, rosterQuery }
}

export function useBillingTransferSection(options: {
  workspaceId: MaybeRefOrGetter<string>
  currentUserId: MaybeRefOrGetter<string | null | undefined>
  isEnabled?: MaybeRefOrGetter<boolean>
}) {
  const { t } = useI18n()
  const toast = useAppToast()
  const client = useQueryClient()
  const workspaceId = () => toValue(options.workspaceId)
  const identity = () => toValue(options.currentUserId) ?? ''
  const isEnabledGate = () => (options.isEnabled !== undefined ? toValue(options.isEnabled) : true)
  const { accountContext, invitesQuery, managersQuery, rosterQuery } = useTransferQueries(
    workspaceId,
    identity,
    isEnabledGate,
  )
  const account = accountContext.account
  const invites = computed(() => invitesQuery.data.value ?? [])
  const managers = computed(() =>
    account.value?.consumptionScope === 'account' ? (managersQuery.data.value ?? []) : [],
  )
  const myRole = computed(() => managers.value.find((member) => member.userId === identity())?.role)
  // Only live (unexpired pending) invites gate decisions and re-issue: a
  // lapsed invite moves nothing and the owner may send again.
  const liveInvites = computed(() => invites.value.filter((invite) => isTransferInviteLive(invite)))
  const hasPendingInvite = computed(() => liveInvites.value.length > 0)
  const addressedInvite = computed(
    () =>
      liveInvites.value.find(
        (invite) =>
          invite.toUserId === identity() &&
          invite.billingAccountId === account.value?.billingAccountId,
      ) ?? null,
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
  const confirmation = ref<Confirmation | null>(null)
  const canInitiate = computed(
    () => myRole.value === 'owner' && !hasPendingInvite.value && !hasLoadError.value,
  )
  // Handover starts from a member row (owner view, never the own row).
  function canHandOverTo(targetUserId: string): boolean {
    return canInitiate.value && targetUserId !== '' && targetUserId !== identity()
  }
  let intent: { signature: string; key: string } | null = null
  let scopeVersion = 0

  watch([workspaceId, identity], () => {
    ++scopeVersion
    confirmation.value = null
    actionError.value = null
    intent = null
  })

  function memberName(userId: string): string {
    return rosterQuery.data.value?.find((member) => member.userId === userId)?.email || userId
  }
  function canDecide(invite: BillingTransferInviteSummary, decision: Decision): boolean {
    if (!isTransferInviteLive(invite)) return false
    if (invite.billingAccountId !== account.value?.billingAccountId) return false
    return decision === 'cancel' ? invite.fromUserId === identity() : invite.toUserId === identity()
  }
  // Every handover confirmation mints a fresh intent: retries of one open
  // dialog reuse its key, while a later re-issue (after lapse or a terminal
  // decision) starts a new key instead of replaying the old invite.
  function requestHandOver(targetUserId: string): void {
    if (isSaving.value || !canHandOverTo(targetUserId)) return
    intent = null
    confirmation.value = { kind: 'invite', targetUserId }
  }
  function requestDecision(invite: BillingTransferInviteSummary, kind: Decision): void {
    if (!isSaving.value && canDecide(invite, kind)) confirmation.value = { kind, invite }
  }
  function dismissConfirmation(): void {
    if (!isSaving.value) confirmation.value = null
  }
  function retry(): void {
    // Directed retry: only the failed instance refetches so one section
    // error never refetches the whole dialog (no double retry with the
    // dialog-level billing account).
    if (accountContext.error.value != null) {
      void accountContext.retry()
      return
    }
    if (invitesQuery.isError.value) {
      void invitesQuery.refetch()
      return
    }
    if (rosterQuery.isError.value) {
      void rosterQuery.retry()
      return
    }
    if (account.value?.consumptionScope === 'account' && managersQuery.isError.value) {
      void managersQuery.refetch()
    }
  }
  async function invalidate(
    scopeId: string,
    viewer: string,
    operation: 'invite' | Decision,
    changed?: BillingTransferInviteSummary | undefined,
  ): Promise<void> {
    // Every decision changes invitation/manager/roster facts. Acceptance
    // additionally changes ownership and consumption visibility, even though
    // the account identity and its wallet balance stay the same.
    const affected = new Set([scopeId, ...(changed?.scopeWorkspaceIds ?? [])])
    const refreshes: Array<Promise<void>> = []
    for (const id of affected) {
      invalidateWorkspaceMemberQueries(client, id)
      refreshes.push(
        client.invalidateQueries({
          queryKey: [...billingTransferQueryKeys.list(id), viewer],
          exact: true,
        }),
      )
      refreshes.push(
        client.invalidateQueries({
          queryKey: billingTransferQueryKeys.members(id, viewer),
          exact: true,
        }),
      )
      if (operation === 'accept') {
        refreshes.push(
          client.invalidateQueries({
            queryKey: billingAccountQueryKeys.detail(id, viewer),
            exact: true,
          }),
          client.invalidateQueries({ queryKey: memberQuotaQueryKeys.self(id), exact: true }),
          client.invalidateQueries({ queryKey: memberQuotaQueryKeys.list(id), exact: true }),
        )
      }
    }
    if (operation === 'accept') {
      // Handover invalidation rides the named key factories: exact keys
      // where the full key is known, narrow predicates through the
      // resource-owned handover matchers where scope/range vary. No raw
      // queryKey[3..5] subscripts at this call site (request-count gate:
      // scalar-only keys, single factory owner per family).
      refreshes.push(
        client.invalidateQueries({ queryKey: workspaceQueryKeys.list, exact: true }),
        client.invalidateQueries({
          queryKey: billingRecordsQueryKeys.all,
          predicate: (query) =>
            isBillingRecordsQueryForHandover(
              query.queryKey,
              affected,
              changed?.billingAccountId,
              viewer,
            ),
        }),
        client.invalidateQueries({
          queryKey: usageQueryKeys.all,
          predicate: (query) =>
            isUsageQueryForHandover(query.queryKey, affected, changed?.billingAccountId, viewer),
        }),
      )
    }
    await Promise.all(refreshes)
  }
  async function runMutation(
    operation: 'invite' | Decision,
    target: string,
    run: (scopeId: string, key: string | undefined) => Promise<BillingTransferInviteSummary | void>,
  ): Promise<void> {
    if (isSaving.value) return
    const scopeId = workspaceId()
    const viewer = identity()
    const version = scopeVersion
    const signature = `${scopeId}:${viewer}:${operation}:${target}`
    if (intent?.signature !== signature) {
      intent = { signature, key: workspaceBillingTransferIdempotencyKey(operation) }
    }
    isSaving.value = true
    actionError.value = null
    try {
      const changed = await run(scopeId, intent?.key)
      await invalidate(scopeId, viewer, operation, changed ?? undefined)
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
  async function confirm(): Promise<void> {
    const pending = confirmation.value
    if (!pending) return
    if (pending.kind === 'invite') {
      if (!canHandOverTo(pending.targetUserId)) return
      const target = pending.targetUserId
      await runMutation('invite', target, (id, key) =>
        initiateTransferInvite(id, target, { idempotencyKey: key }),
      )
      return
    }
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
  return {
    account,
    invites,
    liveInvites,
    managers,
    isLoading,
    hasLoadError,
    retry,
    isSaving,
    actionError,
    canHandOverTo,
    addressedInvite,
    memberName,
    canDecide,
    requestHandOver,
    requestDecision,
    confirmation,
    dismissConfirmation,
    confirm,
  }
}
