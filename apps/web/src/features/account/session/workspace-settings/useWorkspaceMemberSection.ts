import { computed, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import { workspaceMemberIdempotencyKey } from '@/api/http/idempotencyKey'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { useAppClipboard } from '../../../../core/composables/useAppClipboard'
import { useWorkspaceSettingsRoster } from './reads'
import {
  cancelWorkspaceMemberInvite,
  changeWorkspaceMemberRole,
  classifyWorkspaceMemberError,
  createWorkspaceMemberInvite,
  invalidateWorkspaceMemberQueries,
  removeWorkspaceMemberEntry,
  workspaceMemberInvitesQueryOptions,
  type WorkspaceMemberEntry,
  type WorkspaceMemberFailureReason,
  type WorkspaceMemberInvite,
  type WorkspaceMemberRole,
} from '../accountWorkspace'

// Member section controller behind one expanded workspace row in the
// workspace settings dialog. The roster rides TanStack Query on the shared
// generated workspace-members key (same entry the Share panel reads).
// Standalone reads are gated by expansion; the dialog's shared session scope
// controls shared reads. Writes go through the
// workspace member adapter with per-attempt idempotency keys and converge
// by invalidation — never optimistic, so a failed role change rolls back
// by construction (the controlled select keeps the confirmed value).
//
// Only owners/admins see management controls: `canManage` derives from the
// operator's own roster row (matched by `currentUserId`), and the backend
// still enforces 403 for every write.

export type WorkspaceMemberSectionOptions = {
  workspaceId: MaybeRefOrGetter<string>
  currentUserId?: MaybeRefOrGetter<string | null | undefined> | undefined
  isExpanded: MaybeRefOrGetter<boolean>
  // Personal Default is private (#952): the section hides the invite bar
  // and blocks invite attempts with the private-workspace copy. Callers
  // pass the workspace list's isDefault marker; the backend still enforces
  // 403 workspace_default_protected.
  isDefaultWorkspace?: MaybeRefOrGetter<boolean | null | undefined> | undefined
}

export function memberDisplayName(entry: WorkspaceMemberEntry): string {
  return entry.email || entry.userId
}

/** Failure reason to `workspace.switcher.members.*` copy key (single mapping). */
function memberFailureCopyKey(
  reason: WorkspaceMemberFailureReason,
  unknownUserKey: string,
): string {
  switch (reason) {
    case 'unknown-user':
      return unknownUserKey
    case 'forbidden':
      return 'workspace.switcher.members.noPermission'
    case 'default-protected':
      return 'workspace.switcher.members.noPermission'
    case 'last-owner':
      return 'workspace.switcher.members.lastOwner'
    case 'member-limit':
      return 'workspace.switcher.members.memberLimitReached'
    case 'idempotency-conflict':
      return 'workspace.switcher.members.conflict'
    case 'invalid':
      return 'workspace.switcher.members.invalidRequest'
    default:
      return 'workspace.switcher.members.actionFailed'
  }
}

export function useWorkspaceMemberSection(options: WorkspaceMemberSectionOptions) {
  const { t } = useI18n()
  const toast = useAppToast()
  const { copyText } = useAppClipboard()
  const queryClient = useQueryClient()

  const workspaceIdOf = (): string => toValue(options.workspaceId).trim()
  const rosterQuery = useWorkspaceSettingsRoster({
    workspaceId: workspaceIdOf,
    isEnabled: options.isExpanded,
  })
  const roster = computed<WorkspaceMemberEntry[]>(() => rosterQuery.data.value ?? [])
  const myRole = computed<WorkspaceMemberRole | null>(() => {
    const current = toValue(options.currentUserId)?.trim()
    if (!current) return null
    return roster.value.find((entry) => entry.userId === current)?.role ?? null
  })
  const canManage = computed(() => myRole.value === 'owner' || myRole.value === 'admin')
  const isOwner = computed(() => myRole.value === 'owner')
  const isDefaultWorkspace = computed(() => toValue(options.isDefaultWorkspace) === true)

  const invitesQuery = useQuery({
    ...workspaceMemberInvitesQueryOptions(workspaceIdOf),
    enabled: computed(
      () =>
        toValue(options.isExpanded) &&
        canManage.value &&
        !isDefaultWorkspace.value &&
        workspaceIdOf() !== '',
    ),
  })
  const pendingInvites = computed<WorkspaceMemberInvite[]>(() =>
    (invitesQuery.data.value ?? []).filter((inv) => inv.status === 'pending'),
  )

  const collaboratorCount = computed(() => roster.value.length + pendingInvites.value.length)
  const isLimitReached = computed(() => collaboratorCount.value >= 5)

  const isLoading = computed(() => rosterQuery.isLoading.value)
  const hasLoadError = computed(() => rosterQuery.isError.value)

  function retryRoster(): void {
    rosterQuery.retry()
  }

  function invalidateRoster(): void {
    invalidateWorkspaceMemberQueries(queryClient, workspaceIdOf())
  }

  const pendingKeys = ref(new Set<string>())

  function isPending(key: string): boolean {
    return pendingKeys.value.has(key)
  }

  function trackPending<T>(key: string, run: () => Promise<T>): Promise<T> {
    pendingKeys.value = new Set(pendingKeys.value).add(key)
    return run().finally(() => {
      const next = new Set(pendingKeys.value)
      next.delete(key)
      pendingKeys.value = next
    })
  }

  function reportFailure(reason: WorkspaceMemberFailureReason, unknownUserKey: string): void {
    const copy = t(memberFailureCopyKey(reason, unknownUserKey))
    if (reason === 'idempotency-conflict' || reason === 'member-limit') toast.warning(copy)
    else toast.error(copy)
  }

  const inviteEmail = ref('')
  const inviteRole = ref<WorkspaceMemberRole>('member')
  const recentInviteTokens = ref<Record<string, string>>({})

  function getInviteClaimUrl(token: string): string {
    const origin = typeof window !== 'undefined' ? window.location.origin : ''
    return `${origin}/invite/claim?token=${encodeURIComponent(token)}`
  }

  function getInviteLink(invite: WorkspaceMemberInvite): string | null {
    const token = invite.token || recentInviteTokens.value[invite.id]
    return token ? getInviteClaimUrl(token) : null
  }

  async function copyInviteLink(invite: WorkspaceMemberInvite): Promise<void> {
    const link = getInviteLink(invite)
    if (!link) return
    if (await copyText(link)) {
      toast.info(t('workspace.switcher.members.inviteLinkCopied'))
    }
  }

  function pickInviteRole(value: unknown): void {
    if (value === 'owner' || value === 'admin' || value === 'member') {
      inviteRole.value = value
    }
  }

  // Invite is backend-authoritative: only the empty-email guard stays
  // client-side (no request worth sending). Duplicate/limit/pending races
  // and Default protection converge through classifyWorkspaceMemberError
  // (backend 403/409/422) with the shared roster invalidation below; the
  // limit hint itself stays visible in the template (counter + disabled
  // input), not as a pre-flight toast.
  async function inviteMember(): Promise<void> {
    const email = inviteEmail.value.trim()
    if (!email) {
      toast.error(t('workspace.switcher.members.emailEmpty'))
      return
    }
    await trackPending('invite', async () => {
      let invite: WorkspaceMemberInvite
      try {
        invite = await createWorkspaceMemberInvite(
          workspaceIdOf(),
          { email, role: inviteRole.value },
          { idempotencyKey: workspaceMemberIdempotencyKey('invite') },
        )
      } catch (error) {
        const reason = classifyWorkspaceMemberError(error)
        if (reason === 'idempotency-conflict' || reason === 'member-limit') invalidateRoster()
        reportFailure(reason, 'workspace.switcher.members.actionFailed')
        return
      }
      inviteEmail.value = ''
      inviteRole.value = 'member'
      if (invite.token) {
        recentInviteTokens.value = {
          ...recentInviteTokens.value,
          [invite.id]: invite.token,
        }
      }
      // Single toast only: the invite link stays reachable from the pending
      // invite row (per-row copy button via getInviteLink/copyInviteLink).
      toast.success(t('workspace.switcher.members.invited'))
      // Roster invalidation covers this write: the inviter's view is the
      // workspace-scoped roster + pending invites. No syncInvite* call is
      // needed here — those helpers serve the invitee-side accept/decline
      // paths (session flows + ClaimPage), not the inviter-side create.
      invalidateRoster()
    })
  }

  async function revokeInvite(inviteId: string): Promise<void> {
    if (isPending(`revoke:${inviteId}`)) return
    await trackPending(`revoke:${inviteId}`, async () => {
      try {
        await cancelWorkspaceMemberInvite(workspaceIdOf(), inviteId)
        toast.success(t('workspace.switcher.members.revoked'))
      } catch (error) {
        const reason = classifyWorkspaceMemberError(error)
        reportFailure(reason, 'workspace.switcher.members.revokeFailed')
        return
      }
      invalidateRoster()
    })
  }

  // Controlled selects keep the confirmed value: the caller resets the
  // DOM control when this resolves false, so a failed write visibly rolls
  // back instead of stranding the picked value.
  async function changeMemberRole(userId: string, role: WorkspaceMemberRole): Promise<boolean> {
    const current = roster.value.find((entry) => entry.userId === userId)?.role
    if (!current || current === role || isPending(`role:${userId}`)) return false
    return trackPending(`role:${userId}`, async () => {
      try {
        await changeWorkspaceMemberRole(workspaceIdOf(), userId, { role })
      } catch (error) {
        const reason = classifyWorkspaceMemberError(error)
        if (reason === 'idempotency-conflict') invalidateRoster()
        reportFailure(reason, 'workspace.switcher.members.memberUnknown')
        return false
      }
      toast.success(t('workspace.switcher.members.roleUpdated'))
      invalidateRoster()
      return true
    })
  }

  // Two-step removal: DELETE also sweeps board grants server-side with no
  // Undo, so the confirm step names that consequence before any write.
  const confirmingRemoveId = ref<string | null>(null)

  function requestRemove(userId: string): void {
    if (isPending(`remove:${userId}`)) return
    if (isDefaultWorkspace.value && toValue(options.currentUserId)?.trim() === userId) return
    confirmingRemoveId.value = userId
  }

  function cancelRemove(): void {
    confirmingRemoveId.value = null
  }

  async function confirmRemove(userId: string): Promise<void> {
    if (confirmingRemoveId.value !== userId || isPending(`remove:${userId}`)) return
    if (isDefaultWorkspace.value && toValue(options.currentUserId)?.trim() === userId) return
    const isSelf = toValue(options.currentUserId)?.trim() === userId
    await trackPending(`remove:${userId}`, async () => {
      try {
        await removeWorkspaceMemberEntry(workspaceIdOf(), userId, {
          idempotencyKey: workspaceMemberIdempotencyKey('remove'),
        })
      } catch (error) {
        const reason = classifyWorkspaceMemberError(error)
        if (reason === 'idempotency-conflict') invalidateRoster()
        reportFailure(reason, 'workspace.switcher.members.memberUnknown')
        return
      }
      confirmingRemoveId.value = null
      toast.success(
        t(
          isSelf
            ? 'workspace.switcher.members.leftWorkspace'
            : 'workspace.switcher.members.removed',
        ),
      )
      invalidateRoster()
    })
  }

  return {
    roster,
    pendingInvites,
    collaboratorCount,
    isLimitReached,
    myRole,
    canManage,
    isOwner,
    isDefaultWorkspace,
    isLoading,
    hasLoadError,
    retryRoster,
    isPending,
    inviteEmail,
    inviteRole,
    pickInviteRole,
    inviteMember,
    revokeInvite,
    copyInviteLink,
    getInviteLink,
    changeMemberRole,
    confirmingRemoveId,
    requestRemove,
    cancelRemove,
    confirmRemove,
  }
}
