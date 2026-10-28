import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, ref } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { render, waitFor } from '../../../../test/render'
import { useManagement } from './useManagement'
import { getOrganizationScopeStore, installOrganizationScopeStore } from '../scope'
import {
  workspaceQueryKeys,
  type OrganizationInviteSummary,
  type OrganizationMemberSummary,
  type OrganizationSummary,
} from '../../selection/workspaceResource'

const resource = vi.hoisted(() => ({
  fetchOrganizations: vi.fn(),
  fetchOrganizationMembers: vi.fn(),
  fetchOrganizationInvites: vi.fn(),
  inviteOrganizationMember: vi.fn(),
  cancelOrganizationInvite: vi.fn(),
  updateOrganizationMember: vi.fn(),
  removeOrganizationMember: vi.fn(),
  closeOrganization: vi.fn(),
  renameOrganization: vi.fn(),
  provisionOrganization: vi.fn(),
}))
vi.mock('../../selection/workspaceResource', async (load) => ({
  ...(await load<typeof import('../../selection/workspaceResource')>()),
  ...resource,
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock('../../../../core/composables/useAppToast', () => ({ useAppToast: () => toast }))
let organizations: OrganizationSummary[]
let members: OrganizationMemberSummary[]
let invites: OrganizationInviteSummary[]
const OWNER: OrganizationMemberSummary = {
  userId: 'usr_owner',
  role: 'owner',
  email: 'owner@example.com',
}
const MEMBER: OrganizationMemberSummary = {
  userId: 'usr_member',
  role: 'member',
  email: 'member@example.com',
}
function harness() {
  const identity = ref('usr_owner')
  const isEnabled = ref(true)
  const onScopeExit = vi.fn()
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  let management: ReturnType<typeof useManagement> | undefined
  render(
    defineComponent({
      setup() {
        management = useManagement({ identity, isEnabled, onScopeExit })
        return () => h('div')
      },
    }),
    {
      global: { plugins: [[VueQueryPlugin, { queryClient }]] },
    },
  )
  if (!management) throw new Error('Management harness did not mount')
  return { management, identity, isEnabled, onScopeExit, queryClient }
}
async function ready(management: ReturnType<typeof useManagement>) {
  await waitFor(() => expect(management.members.value).toHaveLength(2))
}

beforeEach(() => {
  vi.resetAllMocks()
  organizations = [{ publicId: 'org_team', name: 'Acme', status: 'active' }]
  members = [OWNER, MEMBER]
  invites = []
  resource.fetchOrganizations.mockImplementation(async () => [...organizations])
  resource.fetchOrganizationMembers.mockImplementation(async () => [...members])
  resource.fetchOrganizationInvites.mockImplementation(async () => [...invites])
  resource.renameOrganization.mockImplementation(async (_id, input) => {
    organizations = [{ ...organizations[0]!, name: input.name }]
    return organizations[0]
  })
  resource.updateOrganizationMember.mockImplementation(async (_id, id, input) => {
    members = members.map((member) =>
      member.userId === id ? { ...member, role: input.role } : member,
    )
    return members.find((member) => member.userId === id)
  })
  resource.removeOrganizationMember.mockImplementation(async (_org, id) => {
    members = members.filter((member) => member.userId !== id)
    if (id === 'usr_owner') organizations = []
  })
  resource.closeOrganization.mockImplementation(async () => {
    const closed = { ...organizations[0]!, status: 'closed' as const }
    organizations = []
    return closed
  })
  installOrganizationScopeStore({
    persist: { readScope: () => ({ kind: 'personal' }), writeScope: vi.fn() },
  }).select({ kind: 'organization', organizationId: 'org_team' })
})

describe('Organization management interface', () => {
  it('loads the roster once and owns actor capabilities instead of asking its host', async () => {
    const { management } = harness()
    await ready(management)
    expect(resource.fetchOrganizationMembers).toHaveBeenCalledTimes(1)
    expect(management.canManage.value).toBe(true)
    expect(management.canClose.value).toBe(true)
    await management.changeRole(MEMBER, 'admin')
    await waitFor(() =>
      expect(management.members.value.find((member) => member.userId === MEMBER.userId)?.role).toBe(
        'admin',
      ),
    )
    expect(management.canRemoveMember(OWNER)).toBe(false)
  })
  it.each([
    { role: 'owner', canManage: true, canClose: true },
    { role: 'admin', canManage: true, canClose: false },
    { role: 'member', canManage: false, canClose: false },
  ] as const)(
    'enforces the $role management policy from the roster',
    async ({ role, canManage, canClose }) => {
      members = [{ ...OWNER, role }, MEMBER]
      const { management } = harness()
      await ready(management)
      expect(management.canManage.value).toBe(canManage)
      expect(management.canClose.value).toBe(canClose)
      expect(management.canPromoteToAdmin(MEMBER)).toBe(role === 'owner')
      expect(management.canRemoveMember({ userId: 'usr_admin', role: 'admin' })).toBe(
        role === 'owner',
      )
      expect(management.canRemoveMember(OWNER)).toBe(false)
    },
  )
  it('hides retained facts and rejects writes when its host disables the module', async () => {
    const { management, isEnabled } = harness()
    await ready(management)
    isEnabled.value = false
    await waitFor(() => expect(management.organization.value).toBeNull())
    expect(management.members.value).toEqual([])
    expect(management.canManage.value).toBe(false)
    management.inviteEmail.value = 'new@example.com'
    await management.invite()
    expect(resource.inviteOrganizationMember).not.toHaveBeenCalled()
  })
  it('prevents duplicate invitations across members and pending invitations', async () => {
    invites = [
      {
        id: 'osi_pending',
        organizationId: 'org_team',
        inviteeEmail: 'pending@example.com',
        role: 'member',
        status: 'pending',
        createdAt: '',
        expiresAt: '',
      },
    ]
    const { management } = harness()
    await ready(management)
    await waitFor(() => expect(management.pendingInvites.value).toHaveLength(1))
    management.inviteEmail.value = 'OWNER@example.com'
    await management.invite()
    expect(management.inviteError.value).toBe('This user is already a member.')
    management.inviteEmail.value = 'PENDING@example.com'
    await management.invite()
    expect(management.inviteError.value).toBe('An invitation has already been sent to this email.')
    expect(resource.inviteOrganizationMember).not.toHaveBeenCalled()
  })
  it('converges member invitation and cancellation through the same read module', async () => {
    const pending: OrganizationInviteSummary = {
      id: 'osi_pending',
      organizationId: 'org_team',
      inviteeEmail: 'new@example.com',
      role: 'member',
      status: 'pending',
      createdAt: '',
      expiresAt: '',
    }
    resource.inviteOrganizationMember.mockImplementation(async () => {
      invites = [pending]
      return pending
    })
    resource.cancelOrganizationInvite.mockImplementation(async () => {
      invites = []
    })
    const { management } = harness()
    await ready(management)
    management.inviteEmail.value = 'new@example.com'
    await management.invite()
    expect(management.pendingInvites.value).toEqual([pending])
    await management.revokeInvite(pending.id)
    expect(management.pendingInvites.value).toEqual([])
  })
  it('routes rename server failure to toast and converges a successful rename', async () => {
    const { management } = harness()
    await ready(management)
    management.startRename()
    management.draftName.value = 'Renamed'
    resource.renameOrganization.mockRejectedValueOnce(new Error('network'))
    await management.commitRename()
    expect(management.organization.value?.name).toBe('Acme')
    expect(management.isRenaming.value).toBe(true)
    expect(management.renameError.value).toBeNull()
    expect(toast.error).toHaveBeenCalledWith(
      'Could not rename the organization. Please try again.',
    )
    await management.commitRename()
    expect(management.organization.value?.name).toBe('Renamed')
    expect(management.isRenaming.value).toBe(false)
  })
  it.each(['close', 'leave'] as const)(
    'owns %s scope reset and refresh before requesting shell navigation',
    async (action) => {
      const { management, onScopeExit, queryClient } = harness()
      await ready(management)
      const invalidation = vi.spyOn(queryClient, 'invalidateQueries')
      expect(await management[action]()).toBe(true)
      expect(getOrganizationScopeStore().scope.value).toEqual({ kind: 'personal' })
      expect(invalidation).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: workspaceQueryKeys.list }),
      )
      expect(onScopeExit).toHaveBeenCalledTimes(1)
      expect(management.exitVersion.value).toBe(1)
    },
  )
  it.each(['close', 'leave'] as const)(
    'completes %s scope cleanup when Settings closes during the request',
    async (action) => {
      let finish!: () => void
      resource[
        action === 'close' ? 'closeOrganization' : 'removeOrganizationMember'
      ].mockImplementationOnce(
        () =>
          new Promise<void>((resolve) => {
            finish = resolve
          }),
      )
      const { management, isEnabled, onScopeExit, queryClient } = harness()
      await ready(management)
      const invalidation = vi.spyOn(queryClient, 'invalidateQueries')
      const pending = management[action]()
      isEnabled.value = false
      await waitFor(() => expect(management.organization.value).toBeNull())
      finish()
      expect(await pending).toBe(true)
      expect(getOrganizationScopeStore().scope.value).toEqual({ kind: 'personal' })
      expect(
        queryClient.getQueryData([...workspaceQueryKeys.organizations, 'viewer', 'usr_owner']),
      ).toEqual([])
      expect(invalidation).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: workspaceQueryKeys.list }),
      )
      expect(onScopeExit).toHaveBeenCalledTimes(1)
      expect(toast.success).not.toHaveBeenCalled()
    },
  )
  it('refreshes a successful member change after Settings closes', async () => {
    let finish!: () => void
    resource.updateOrganizationMember.mockImplementationOnce(async () => {
      await new Promise<void>((resolve) => {
        finish = resolve
      })
      members = [OWNER, { ...MEMBER, role: 'admin' }]
    })
    const { management, isEnabled, queryClient } = harness()
    await ready(management)
    const invalidation = vi.spyOn(queryClient, 'invalidateQueries')
    const pending = management.changeRole(MEMBER, 'admin')
    isEnabled.value = false
    await waitFor(() => expect(management.members.value).toEqual([]))
    finish()
    await pending
    expect(invalidation).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: workspaceQueryKeys.organizations }),
    )
    expect(invalidation).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: workspaceQueryKeys.list }),
    )
    expect(toast.success).not.toHaveBeenCalled()
    isEnabled.value = true
    await waitFor(() =>
      expect(management.members.value.find((member) => member.userId === MEMBER.userId)?.role).toBe(
        'admin',
      ),
    )
  })
  it('converges a late rename without overwriting a reopened rename form', async () => {
    let finish!: (value: OrganizationSummary) => void
    resource.renameOrganization.mockImplementationOnce(
      () =>
        new Promise<OrganizationSummary>((resolve) => {
          finish = resolve
        }),
    )
    const { management, isEnabled } = harness()
    await ready(management)
    management.startRename()
    management.draftName.value = 'Renamed'
    const pending = management.commitRename()
    isEnabled.value = false
    await waitFor(() => expect(management.organization.value).toBeNull())
    isEnabled.value = true
    await ready(management)
    management.startRename()
    management.draftName.value = 'New draft'
    organizations = [{ ...organizations[0]!, name: 'Renamed' }]
    finish(organizations[0]!)
    await pending
    expect(management.organization.value?.name).toBe('Renamed')
    expect(management.isRenaming.value).toBe(true)
    expect(management.draftName.value).toBe('New draft')
  })
  it('preserves scope and data on refused closure', async () => {
    resource.closeOrganization.mockRejectedValue({ code: 'organization_closure_blocked' })
    const { management, onScopeExit } = harness()
    await ready(management)
    expect(await management.close()).toBe(false)
    expect(management.organization.value?.name).toBe('Acme')
    expect(getOrganizationScopeStore().scope.value.kind).toBe('organization')
    expect(onScopeExit).not.toHaveBeenCalled()
    expect(toast.error).toHaveBeenCalledWith(
      expect.stringContaining('unsettled tasks or funds'),
    )
  })
  it('hides old identity facts and does not apply an old identity mutation to the new viewer', async () => {
    let finish!: () => void
    resource.closeOrganization.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve
        }),
    )
    const { management, identity, onScopeExit } = harness()
    await ready(management)
    const closing = management.close()
    identity.value = 'usr_other'
    organizations = []
    await waitFor(() => expect(management.members.value).toEqual([]))
    finish()
    await closing
    expect(onScopeExit).not.toHaveBeenCalled()
    expect(getOrganizationScopeStore().scope.value.kind).toBe('organization')
  })
  it('does not apply an old identity mutation after switching away and back', async () => {
    let finish!: () => void
    resource.closeOrganization.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve
        }),
    )
    const { management, identity, onScopeExit } = harness()
    await ready(management)
    const pending = management.close()
    identity.value = 'usr_other'
    identity.value = 'usr_owner'
    finish()
    await pending
    expect(onScopeExit).not.toHaveBeenCalled()
    expect(getOrganizationScopeStore().scope.value.kind).toBe('organization')
    expect(toast.success).not.toHaveBeenCalled()
  })
  it('supports Organization setup without a Workspace and keeps the retry intent stable', async () => {
    organizations = []
    resource.provisionOrganization
      .mockRejectedValueOnce(new Error('network'))
      .mockImplementationOnce(async () => {
        organizations = [{ publicId: 'org_team', name: 'Acme', status: 'active' }]
        return organizations[0]
      })
    const { management } = harness()
    await waitFor(() => expect(management.isLoading.value).toBe(false))
    management.organizationName.value = 'Acme'
    await management.createOrganization()
    await management.createOrganization()
    expect(resource.provisionOrganization.mock.calls[0]?.[1]).toBe(
      resource.provisionOrganization.mock.calls[1]?.[1],
    )
    expect(management.organization.value?.name).toBe('Acme')
  })
})
