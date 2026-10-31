import { computed, ref, toValue, watch, type ComputedRef, type MaybeRefOrGetter } from 'vue'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import { useI18n } from 'vue-i18n'
import {
  organizationClosureIdempotencyKey,
  organizationCreationIdempotencyKey,
} from '@/api/http/idempotencyKey'
import { useAppToast } from '../../../../core/composables/useAppToast'
import { userErrorCode } from '../../../../core/errors/errorCatalog'
import {
  WORKSPACE_NAME_MAX_LENGTH,
  cancelOrganizationInvite,
  closeOrganization,
  fetchOrganizationInvites,
  fetchOrganizationMembers,
  fetchOrganizations,
  inviteOrganizationMember,
  provisionOrganization,
  removeOrganizationMember,
  renameOrganization,
  updateOrganizationMember,
  workspaceQueryKeys,
  type OrganizationMemberSummary,
} from '../../selection/workspaceResource'
import { getOrganizationScopeStore } from '../scope'

function useOrganizationFacts(identity: ComputedRef<string>, isEnabled: ComputedRef<boolean>) {
  const { t } = useI18n()
  const queryClient = useQueryClient()
  const organizationKey = computed(() => [
    ...workspaceQueryKeys.organizations,
    'viewer',
    identity.value,
  ])
  const organizationsQuery = useQuery({
    queryKey: organizationKey,
    queryFn: fetchOrganizations,
    enabled: isEnabled,
  })
  const organization = computed(() =>
    isEnabled.value ? (organizationsQuery.data.value?.[0] ?? null) : null,
  )
  const organizationId = computed(() => organization.value?.publicId ?? '')
  const membersKey = computed(() => [
    ...workspaceQueryKeys.organizationMembers(organizationId.value),
    identity.value,
  ])
  const membersQuery = useQuery({
    queryKey: membersKey,
    queryFn: ({ queryKey, signal }) => fetchOrganizationMembers(String(queryKey[1]), { signal }),
    enabled: () => isEnabled.value && organizationId.value !== '',
  })
  const members = computed(() =>
    isEnabled.value && organizationId.value ? (membersQuery.data.value ?? []) : [],
  )
  const actorRole = computed(
    () => members.value.find((member) => member.userId === identity.value)?.role ?? null,
  )
  const isClosed = computed(() => organization.value?.status === 'closed')
  const canManage = computed(
    () => !isClosed.value && (actorRole.value === 'owner' || actorRole.value === 'admin'),
  )
  const canLeave = computed(() => !isClosed.value && actorRole.value !== null)
  const canClose = computed(() => !isClosed.value && actorRole.value === 'owner')
  const invitesKey = computed(() => [
    ...workspaceQueryKeys.organizations,
    organizationId.value,
    'invites',
    identity.value,
  ])
  const invitesQuery = useQuery({
    queryKey: invitesKey,
    queryFn: ({ queryKey, signal }) => fetchOrganizationInvites(String(queryKey[1]), { signal }),
    enabled: () => isEnabled.value && organizationId.value !== '' && canManage.value,
  })
  const pendingInvites = computed(() => (canManage.value ? (invitesQuery.data.value ?? []) : []))
  const isLoading = computed(
    () =>
      isEnabled.value &&
      (organizationsQuery.isPending.value ||
        (organizationId.value !== '' && membersQuery.isPending.value) ||
        (canManage.value && invitesQuery.isPending.value)),
  )
  const loadError = computed(() =>
    isEnabled.value &&
    (organizationsQuery.isError.value ||
      (organizationId.value !== '' && membersQuery.isError.value) ||
      (canManage.value && invitesQuery.isError.value))
      ? t('workspace.switcher.members.loadFailed')
      : null,
  )

  return {
    queryClient,
    organizationKey,
    organization,
    members,
    actorRole,
    isClosed,
    canManage,
    canLeave,
    canClose,
    pendingInvites,
    isLoading,
    loadError,
  }
}

interface OrganizationManagementOptions {
  identity: MaybeRefOrGetter<string | null | undefined>
  isEnabled: MaybeRefOrGetter<boolean>
  onScopeExit: () => void
}

/** Organization facts and actions belong to Workspace; hosts only control visibility. */
export function useManagement(options: OrganizationManagementOptions) {
  const { t } = useI18n()
  const toast = useAppToast()
  const identity = computed(() => toValue(options.identity)?.trim() ?? '')
  const isEnabled = computed(() => toValue(options.isEnabled) && identity.value !== '')
  const { queryClient, organizationKey, actorRole, ...facts } = useOrganizationFacts(
    identity,
    isEnabled,
  )
  const { organization, members, canManage, canLeave, canClose, pendingInvites } = facts

  const inviteEmail = ref('')
  const inviteError = ref<string | null>(null)
  const isSaving = ref(false)
  const isRevokingInvite = ref(false)
  const isLeaving = ref(false)
  const isClosing = ref(false)
  const organizationName = ref('')
  const organizationError = ref<string | null>(null)
  const isCreatingOrganization = ref(false)
  const draftName = ref('')
  const isRenaming = ref(false)
  const isRenamingBusy = ref(false)
  const renameError = ref<string | null>(null)
  const exitVersion = ref(0)
  let creationKey = organizationCreationIdempotencyKey()
  let identityGeneration = 0
  let viewGeneration = 0

  watch(
    [identity, isEnabled],
    ([viewer], [previousViewer]) => {
      if (viewer !== previousViewer) identityGeneration++
      viewGeneration++
      isSaving.value = false
      isRevokingInvite.value = false
      isLeaving.value = false
      isClosing.value = false
      isCreatingOrganization.value = false
      isRenamingBusy.value = false
      inviteEmail.value = ''
      inviteError.value = null
      organizationName.value = ''
      organizationError.value = null
      isRenaming.value = false
      renameError.value = null
      creationKey = organizationCreationIdempotencyKey()
    },
    { flush: 'sync' },
  )

  function viewerContext() {
    const viewer = identity.value
    const currentIdentityGeneration = identityGeneration
    const currentViewGeneration = viewGeneration
    const isCurrent = () =>
      identity.value === viewer && identityGeneration === currentIdentityGeneration
    return {
      viewer,
      // Successful writes still reconcile facts after the host closes; local feedback does not.
      isCurrent,
      isVisible: () => isCurrent() && isEnabled.value && viewGeneration === currentViewGeneration,
    }
  }

  function context() {
    const org = organization.value
    if (!org) return null
    return {
      org,
      ...viewerContext(),
    }
  }

  function actionError(error: unknown): string {
    const code = userErrorCode(error)?.toLowerCase() ?? ''
    const rules: Array<[string, string]> = [
      ['organization_limit_reached', 'workspace.management.limitReached'],
      ['organization_closed', 'workspace.management.closeBlocked'],
      ['organization_closure_blocked', 'workspace.management.closeBlocked'],
      ['membership_transition_forbidden', 'workspace.switcher.members.noPermission'],
      ['organization_member_not_found', 'workspace.switcher.members.memberUnknown'],
      ['not_found', 'workspace.switcher.members.memberUnknown'],
    ]
    return t(
      rules.find(([part]) => code.includes(part))?.[1] ?? 'workspace.switcher.members.actionFailed',
    )
  }

  async function refresh(): Promise<void> {
    await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.organizations })
  }

  async function refreshWorkspaces(): Promise<void> {
    await queryClient.invalidateQueries({ queryKey: workspaceQueryKeys.list, exact: true })
  }

  async function createOrganization(): Promise<void> {
    const name = organizationName.value.trim()
    if (!isEnabled.value || organization.value || isCreatingOrganization.value) return
    if (!name || name.length > WORKSPACE_NAME_MAX_LENGTH) {
      organizationError.value = t('workspace.switcher.organization.invalidName', {
        max: WORKSPACE_NAME_MAX_LENGTH,
      })
      return
    }
    const scope = viewerContext()
    isCreatingOrganization.value = true
    organizationError.value = null
    try {
      await provisionOrganization(name, creationKey)
      if (!scope.isCurrent()) return
      if (scope.isVisible()) {
        organizationName.value = ''
        creationKey = organizationCreationIdempotencyKey()
      }
      await Promise.all([refresh(), refreshWorkspaces()])
    } catch {
      if (scope.isVisible()) toast.error(t('workspace.switcher.organization.createFailed'))
    } finally {
      if (scope.isVisible()) isCreatingOrganization.value = false
    }
  }

  function startRename(): void {
    if (!organization.value || !canManage.value) return
    draftName.value = organization.value.name
    renameError.value = null
    isRenaming.value = true
  }
  function cancelRename(): void {
    isRenaming.value = false
    renameError.value = null
  }
  async function commitRename(): Promise<void> {
    const scope = context()
    if (!scope || !canManage.value || !isRenaming.value || isRenamingBusy.value) return
    const name = draftName.value.trim()
    if (name === scope.org.name) return cancelRename()
    if (!name || name.length > WORKSPACE_NAME_MAX_LENGTH) {
      renameError.value = t('workspace.switcher.organization.invalidName', {
        max: WORKSPACE_NAME_MAX_LENGTH,
      })
      return
    }
    isRenamingBusy.value = true
    renameError.value = null
    try {
      const updated = await renameOrganization(scope.org.publicId, { name })
      if (!scope.isCurrent()) return
      queryClient.setQueryData(organizationKey.value, [updated])
      if (scope.isVisible()) isRenaming.value = false
      await refresh()
    } catch {
      if (scope.isVisible()) toast.error(t('workspace.switcher.organization.renameFailed'))
    } finally {
      if (scope.isVisible()) isRenamingBusy.value = false
    }
  }

  function canSelfLeave(member: OrganizationMemberSummary): boolean {
    return canLeave.value && member.userId === identity.value
  }
  function canPromoteToAdmin(member: OrganizationMemberSummary): boolean {
    return (
      canManage.value &&
      actorRole.value === 'owner' &&
      member.userId !== identity.value &&
      member.role === 'member'
    )
  }
  function canDemoteToMember(member: OrganizationMemberSummary): boolean {
    return (
      canManage.value &&
      actorRole.value === 'owner' &&
      member.userId !== identity.value &&
      member.role === 'admin'
    )
  }
  function canRemoveMember(member: OrganizationMemberSummary): boolean {
    return (
      canManage.value &&
      member.role !== 'owner' &&
      member.userId !== identity.value &&
      (actorRole.value === 'owner' || member.role !== 'admin')
    )
  }
  function hasActions(member: OrganizationMemberSummary): boolean {
    return (
      canSelfLeave(member) ||
      canPromoteToAdmin(member) ||
      canDemoteToMember(member) ||
      canRemoveMember(member)
    )
  }

  async function saveMemberChange(
    scope: NonNullable<ReturnType<typeof context>>,
    request: () => Promise<unknown>,
    successKey: string,
  ): Promise<void> {
    isSaving.value = true
    try {
      await request()
      if (!scope.isCurrent()) return
      await Promise.all([refresh(), refreshWorkspaces()])
      if (scope.isVisible()) toast.success(t(successKey))
    } catch (error) {
      if (scope.isVisible()) toast.error(actionError(error))
    } finally {
      if (scope.isVisible()) isSaving.value = false
    }
  }
  async function changeRole(
    member: OrganizationMemberSummary,
    role: 'member' | 'admin',
  ): Promise<void> {
    const scope = context()
    if (
      !scope ||
      isSaving.value ||
      !(role === 'admin' ? canPromoteToAdmin(member) : canDemoteToMember(member))
    )
      return
    await saveMemberChange(
      scope,
      () => updateOrganizationMember(scope.org.publicId, member.userId, { role }),
      'workspace.switcher.members.roleUpdated',
    )
  }
  async function removeMember(member: OrganizationMemberSummary): Promise<void> {
    const scope = context()
    if (!scope || isSaving.value || !canRemoveMember(member)) return
    await saveMemberChange(
      scope,
      () => removeOrganizationMember(scope.org.publicId, member.userId),
      'workspace.switcher.members.removed',
    )
  }
  async function invite(): Promise<void> {
    const scope = context()
    const email = inviteEmail.value.trim()
    if (!scope || !canManage.value || !email || isSaving.value) return
    const normalized = email.toLowerCase()
    if (
      members.value.some(
        (member) => member.email?.toLowerCase() === normalized || member.userId === email,
      )
    ) {
      inviteError.value = t('workspace.switcher.members.alreadyMember')
      return
    }
    if (pendingInvites.value.some((pending) => pending.inviteeEmail.toLowerCase() === normalized)) {
      inviteError.value = t('workspace.switcher.members.alreadyInvited')
      return
    }
    isSaving.value = true
    inviteError.value = null
    try {
      await inviteOrganizationMember(scope.org.publicId, { email, role: 'member' })
      if (!scope.isCurrent()) return
      if (scope.isVisible()) inviteEmail.value = ''
      await refresh()
      if (scope.isVisible()) toast.success(t('workspace.switcher.members.invited'))
    } catch (error) {
      if (scope.isVisible()) toast.error(actionError(error))
    } finally {
      if (scope.isVisible()) isSaving.value = false
    }
  }
  async function revokeInvite(inviteId: string): Promise<void> {
    const scope = context()
    if (!scope || !canManage.value || isRevokingInvite.value) return
    isRevokingInvite.value = true
    try {
      await cancelOrganizationInvite(scope.org.publicId, inviteId)
      if (!scope.isCurrent()) return
      await refresh()
      if (scope.isVisible()) toast.success(t('workspace.switcher.members.revoked'))
    } catch (error) {
      if (scope.isVisible()) toast.error(actionError(error))
    } finally {
      if (scope.isVisible()) isRevokingInvite.value = false
    }
  }

  async function exitOrganization(scope: NonNullable<ReturnType<typeof context>>): Promise<void> {
    if (!scope.isCurrent()) return
    getOrganizationScopeStore().selectPersonal()
    queryClient.setQueryData(organizationKey.value, [])
    await Promise.all([refresh(), refreshWorkspaces()])
    if (!scope.isCurrent()) return
    options.onScopeExit()
    exitVersion.value++
  }
  async function leave(): Promise<boolean> {
    const scope = context()
    if (!scope || !canLeave.value || isLeaving.value) return false
    isLeaving.value = true
    try {
      await removeOrganizationMember(scope.org.publicId, scope.viewer)
      await exitOrganization(scope)
      if (scope.isVisible()) toast.success(t('workspace.management.leaveSuccess'))
      return true
    } catch (error) {
      if (scope.isVisible()) toast.error(actionError(error))
      return false
    } finally {
      if (scope.isVisible()) isLeaving.value = false
    }
  }
  async function close(): Promise<boolean> {
    const scope = context()
    if (!scope || !canClose.value || isClosing.value) return false
    isClosing.value = true
    try {
      await closeOrganization(
        scope.org.publicId,
        {},
        { idempotencyKey: organizationClosureIdempotencyKey() },
      )
      await exitOrganization(scope)
      if (scope.isVisible()) toast.success(t('workspace.management.closeSuccess'))
      return true
    } catch (error) {
      if (scope.isVisible())
        toast.error(
          userErrorCode(error)?.includes('organization_closure_blocked')
            ? t('workspace.management.closeBlocked')
            : t('workspace.management.closeFailed'),
        )
      return false
    } finally {
      if (scope.isVisible()) isClosing.value = false
    }
  }

  return {
    ...facts,
    inviteEmail,
    inviteError,
    isSaving,
    isRevokingInvite,
    isLeaving,
    isClosing,
    organizationName,
    organizationError,
    isCreatingOrganization,
    draftName,
    isRenaming,
    isRenamingBusy,
    renameError,
    exitVersion,
    refresh,
    createOrganization,
    startRename,
    cancelRename,
    commitRename,
    canSelfLeave,
    canPromoteToAdmin,
    canDemoteToMember,
    canRemoveMember,
    hasActions,
    changeRole,
    removeMember,
    invite,
    revokeInvite,
    leave,
    close,
  }
}

export type OrganizationManagement = ReturnType<typeof useManagement>
