import { beforeEach, describe, expect, test, vi } from 'vitest'
import { computed, defineComponent, toValue, type MaybeRefOrGetter } from 'vue'
import {
  QueryClient,
  VueQueryPlugin,
  type QueryKey,
  type UseQueryOptions,
} from '@tanstack/vue-query'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import { makeSessionUser } from '../../../../test/factories'
import { useUserStore } from '../../../auth'
import WorkspaceSettingsDialog from './WorkspaceSettingsDialog.vue'
import type { WorkspaceMemberEntry } from '../../session/accountWorkspace'

vi.mock('../../../../core/composables/useAppToast')

// The workspace member transport is faked per workspace: the section
// controller under test keeps the real query wiring (shared roster key,
// invalidation on writes), so per-workspace isolation and refetch
// convergence are exercised, not stubbed out.
const fakeMembers = vi.hoisted(() => ({
  rosters: new Map<string, WorkspaceMemberEntry[]>(),
  lookup: new Map<string, string>(),
  calls: [] as Array<Record<string, unknown>>,
  failNext: null as { op: string; code: string } | null,
}))

function fakeKey(workspaceId: MaybeRefOrGetter<string>) {
  return ['workspaces', toValue(workspaceId), 'members'] as const
}

function maybeFail(op: string): void {
  if (fakeMembers.failNext?.op === op) {
    const failure = fakeMembers.failNext
    fakeMembers.failNext = null
    throw { code: failure.code }
  }
}

function cloneRoster(workspaceId: string): WorkspaceMemberEntry[] {
  return (fakeMembers.rosters.get(workspaceId) ?? []).map((entry) => ({ ...entry }))
}

function classifyReason(code: string): string {
  switch (code) {
    case 'workspace_member_not_found':
      return 'unknown-user'
    case 'workspace_not_found':
      return 'hidden'
    case 'workspace_forbidden':
      return 'forbidden'
    case 'membership_transition_forbidden':
      return 'last-owner'
    case 'workspace_member_limit_reached':
      return 'member-limit'
    case 'workspace_idempotency_conflict':
      return 'idempotency-conflict'
    case 'bad_request':
    case 'validation_failed':
    case 'idempotency_key_required':
      return 'invalid'
    default:
      return 'sync-failed'
  }
}

vi.mock('../../../workspace/selection/workspaceMembersResource', () => ({
  // The TanStack assertion is the naming contract: `retry`/`enabled` stay
  // verbatim as the query client's public API (mirrors the resource).
  workspaceMembersQueryOptions: (workspaceId: MaybeRefOrGetter<string>) =>
    ({
      queryKey: fakeKey(workspaceId),
      queryFn: () => {
        maybeFail('list')
        return Promise.resolve(cloneRoster(toValue(workspaceId).trim()))
      },
      staleTime: 15_000,
      retry: false,
      enabled: computed(() => toValue(workspaceId).trim().length > 0),
    }) as UseQueryOptions<WorkspaceMemberEntry[], Error, WorkspaceMemberEntry[], QueryKey>,
  fetchWorkspaceMembers: (workspaceId: string) => {
    maybeFail('list')
    return Promise.resolve(cloneRoster(workspaceId))
  },
  lookupWorkspaceMemberId: (workspaceId: string, email: string) => {
    maybeFail('lookup')
    const userId = fakeMembers.lookup.get(email.trim().toLowerCase())
    if (!userId) throw { code: 'workspace_member_not_found' }
    return Promise.resolve(userId)
  },
  inviteWorkspaceMember: (
    workspaceId: string,
    input: { userId: string; role: string },
    options: { idempotencyKey?: string } = {},
  ) => {
    maybeFail('invite')
    fakeMembers.calls.push({ op: 'invite', workspaceId, ...input, ...options })
    const roster = fakeMembers.rosters.get(workspaceId) ?? []
    const entry = { userId: input.userId, role: input.role as WorkspaceMemberEntry['role'] }
    fakeMembers.rosters.set(workspaceId, [...roster, entry])
    return Promise.resolve(entry)
  },
  changeWorkspaceMemberRole: (workspaceId: string, memberId: string, input: { role: string }) => {
    maybeFail('role')
    fakeMembers.calls.push({ op: 'role', workspaceId, memberId, ...input })
    const roster = (fakeMembers.rosters.get(workspaceId) ?? []).map((entry) =>
      entry.userId === memberId
        ? { ...entry, role: input.role as WorkspaceMemberEntry['role'] }
        : entry,
    )
    fakeMembers.rosters.set(workspaceId, roster)
    return Promise.resolve(roster.find((entry) => entry.userId === memberId))
  },
  removeWorkspaceMemberEntry: (workspaceId: string, memberId: string) => {
    maybeFail('remove')
    fakeMembers.calls.push({ op: 'remove', workspaceId, memberId })
    fakeMembers.rosters.set(
      workspaceId,
      (fakeMembers.rosters.get(workspaceId) ?? []).filter((entry) => entry.userId !== memberId),
    )
    return Promise.resolve()
  },
  classifyWorkspaceMemberError: (error: unknown) =>
    classifyReason((error as { code?: unknown } | null)?.code as string),
  invalidateWorkspaceMemberQueries: (
    client: Pick<QueryClient, 'invalidateQueries'>,
    workspaceId: string,
  ) => {
    client.invalidateQueries({ queryKey: fakeKey(workspaceId), exact: true })
  },
}))

const DialogRootStub = defineComponent({
  props: { open: { type: Boolean, required: true } },
  emits: ['update:open'],
  template: '<div v-if="open"><slot /></div>',
})
const DialogPassthroughStub = defineComponent({ template: '<slot />' })
const DialogContentStub = defineComponent({
  inheritAttrs: false,
  template: '<section role="dialog" tabindex="-1" v-bind="$attrs"><slot /></section>',
})
const DialogTitleStub = defineComponent({ template: '<h2><slot /></h2>' })

const dialogStubs = {
  DialogRoot: DialogRootStub,
  DialogPortal: DialogPassthroughStub,
  DialogOverlay: DialogPassthroughStub,
  DialogContent: DialogContentStub,
  DialogDescription: DialogPassthroughStub,
  DialogTitle: DialogTitleStub,
}

const ME = 'usr_me'

function seedRosters() {
  fakeMembers.rosters.set('ws_personal1', [
    { userId: ME, role: 'owner', email: 'me@example.com' },
    { userId: 'usr_a', role: 'member' },
  ])
  fakeMembers.rosters.set('ws_team2', [
    { userId: 'usr_boss', role: 'owner', email: 'boss@example.com' },
    { userId: ME, role: 'admin' },
  ])
  fakeMembers.rosters.set('ws_team3', [
    { userId: 'usr_boss', role: 'owner', email: 'boss@example.com' },
    { userId: ME, role: 'member' },
  ])
  fakeMembers.lookup.set('new@example.com', 'usr_new')
  fakeMembers.lookup.set('a@example.com', 'usr_a')
}

function dialogItems() {
  return [
    {
      publicId: 'ws_personal1',
      name: 'Personal',
      organizationId: null,
      isDefault: true,
    },
    {
      publicId: 'ws_team2',
      name: 'Team 2',
      organizationId: 'org_c3',
      isDefault: false,
    },
    {
      publicId: 'ws_team3',
      name: 'Team 3',
      organizationId: 'org_c3',
      isDefault: false,
    },
  ]
}

function renderMembersDialog() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = renderWithUser(WorkspaceSettingsDialog, {
    props: {
      isOpen: true,
      items: dialogItems(),
      selectedId: 'ws_personal1',
      currentUserId: ME,
      onRetry: vi.fn(),
      onCreate: vi.fn(),
      onRename: vi.fn(),
      onDelete: vi.fn(),
    },
    global: {
      stubs: dialogStubs,
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
  return { ...view, queryClient }
}

// Sections render through v-show: collapsed rows stay in the DOM but hidden,
// so visibility (not presence) separates expanded from collapsed state. The
// per-workspace aria-label sits on the section (role `region`); text queries
// must scope through here because rosters overlap across workspaces.
function sectionByWorkspace(name: string): HTMLElement {
  return screen.getByRole('region', { name: `Members of ${name}` })
}

describe('WorkspaceSettingsDialog members', () => {
  let toast: AppToastMocks

  beforeEach(async () => {
    await setUserLocale('en')
    fakeMembers.calls.length = 0
    fakeMembers.failNext = null
    fakeMembers.rosters.clear()
    fakeMembers.lookup.clear()
    seedRosters()
    toast = mockAppToast()
  })

  test('renders the selected workspace members and lists remaining workspaces', async () => {
    renderMembersDialog()

    // Selected workspace roster loads with management controls (owner view).
    expect(await screen.findByText('me@example.com')).toBeVisible()
    expect(screen.getByText('usr_a')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Invite' })).toBeVisible()

    // Unselected workspaces are listed in navigation but not shown in right pane.
    expect(screen.getByRole('button', { name: 'Team 2' })).toBeVisible()
    expect(screen.queryByRole('region', { name: 'Members of Team 2' })).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Invite' })).toHaveLength(1)
  })

  test('switches active workspace when clicking a workspace in the sidebar', async () => {
    const { user } = renderMembersDialog()

    expect(screen.queryByRole('region', { name: 'Members of Team 2' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Team 2' }))

    const team = within(await screen.findByRole('region', { name: 'Members of Team 2' }))
    expect(await team.findByText('boss@example.com')).toBeVisible()

    await user.click(screen.getByRole('button', { name: 'Default' }))
    expect(await screen.findByRole('region', { name: 'Members of Default' })).toBeVisible()
    expect(screen.queryByRole('region', { name: 'Members of Team 2' })).not.toBeInTheDocument()
  })

  test('changes a role through PATCH and converges the roster', async () => {
    const { user } = renderMembersDialog()

    // Compact dropdown: the closed trigger shows the confirmed value…
    const trigger = await screen.findByRole('combobox', { name: 'Role for usr_a' })
    expect(trigger).toHaveTextContent('Member')

    await user.click(trigger)
    await user.click(await screen.findByRole('option', { name: 'Admin' }))

    await waitFor(() =>
      expect(fakeMembers.calls).toContainEqual(
        expect.objectContaining({ op: 'role', workspaceId: 'ws_personal1', memberId: 'usr_a' }),
      ),
    )
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Role updated.'))
    // …and converges to the new value once the roster refetch lands.
    await waitFor(() =>
      expect(screen.getByRole('combobox', { name: 'Role for usr_a' })).toHaveTextContent('Admin'),
    )
  })

  test('keeps the confirmed role with a toast when the role change fails', async () => {
    const { user } = renderMembersDialog()
    fakeMembers.failNext = { op: 'role', code: 'membership_transition_forbidden' }

    const trigger = await screen.findByRole('combobox', { name: 'Role for usr_a' })
    await user.click(trigger)
    await user.click(await screen.findByRole('option', { name: 'Owner' }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('The last owner cannot be removed or demoted.'),
    )
    // Controlled dropdown: the failed write never moves the confirmed value.
    expect(screen.getByRole('combobox', { name: 'Role for usr_a' })).toHaveTextContent('Member')
  })

  test('requires confirmation before removing, naming the board-grant sweep', async () => {
    const { user } = renderMembersDialog()
    await screen.findByText('usr_a')

    // Leave/Remove is an independent text button at the row end, outside the
    // role dropdown: no segmented pill, no radiogroup.
    expect(screen.queryByRole('radiogroup')).not.toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: 'Role for usr_a' })).toBeVisible()
    const removeButton = screen.getByRole('button', { name: 'Remove' })

    await user.click(removeButton)

    const note = await screen.findByText(/also removes their board permissions/)
    expect(note).toBeVisible()
    expect(fakeMembers.calls.filter((call) => call.op === 'remove')).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByText(/also removes their board permissions/)).not.toBeInTheDocument()
    expect(fakeMembers.calls.filter((call) => call.op === 'remove')).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Remove' }))
    await user.click(await screen.findByRole('button', { name: 'Confirm remove' }))

    await waitFor(() =>
      expect(fakeMembers.calls).toContainEqual(
        expect.objectContaining({ op: 'remove', workspaceId: 'ws_personal1', memberId: 'usr_a' }),
      ),
    )
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Member removed.'))
    await waitFor(() => expect(screen.queryByText('usr_a')).not.toBeInTheDocument())
  })

  test('invites by email through lookup plus POST with a fresh idempotency key', async () => {
    const { user } = renderMembersDialog()
    await screen.findByText('usr_a')

    await user.type(screen.getByLabelText('Invite by email'), 'new@example.com')
    await user.click(screen.getByRole('button', { name: 'Invite' }))

    await waitFor(() =>
      expect(fakeMembers.calls).toContainEqual(
        expect.objectContaining({
          op: 'invite',
          workspaceId: 'ws_personal1',
          userId: 'usr_new',
          role: 'member',
        }),
      ),
    )
    const invite = fakeMembers.calls.find((call) => call.op === 'invite') as {
      idempotencyKey?: string
    }
    expect(invite.idempotencyKey).toMatch(/^workspace:members\.invite:/)
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('Member invited.'))
    expect(await screen.findByText('usr_new')).toBeVisible()
  })

  test('empty email toasts instead of rendering inline', async () => {
    const { user } = renderMembersDialog()
    await screen.findByText('usr_a')

    await user.click(screen.getByRole('button', { name: 'Invite' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Email cannot be empty.'))
    expect(fakeMembers.calls.filter((call) => call.op === 'lookup')).toHaveLength(0)
    expect(fakeMembers.calls.filter((call) => call.op === 'invite')).toHaveLength(0)
  })

  test('reports the five-person limit and refreshes the roster after a concurrent invite', async () => {
    const { user } = renderMembersDialog()
    await screen.findByText('usr_a')
    fakeMembers.rosters
      .get('ws_personal1')
      ?.push(
        { userId: 'usr_b', role: 'member' },
        { userId: 'usr_c', role: 'member' },
        { userId: 'usr_d', role: 'member' },
      )
    fakeMembers.failNext = { op: 'invite', code: 'workspace_member_limit_reached' }

    await user.type(screen.getByLabelText('Invite by email'), 'new@example.com')
    await user.click(screen.getByRole('button', { name: 'Invite' }))

    await waitFor(() =>
      expect(toast.warning).toHaveBeenCalledWith(
        'This workspace has reached its limit of 5 people, including owners.',
      ),
    )
    expect(await screen.findByText('usr_d')).toBeVisible()
    expect(screen.getByLabelText('Invite by email')).toHaveValue('new@example.com')
    expect(toast.success).not.toHaveBeenCalled()
  })

  test('unknown emails toast user-not-found without a POST', async () => {
    const { user } = renderMembersDialog()
    await screen.findByText('usr_a')

    await user.type(screen.getByLabelText('Invite by email'), 'ghost@example.com')
    await user.click(screen.getByRole('button', { name: 'Invite' }))

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('No registered user found for this email.'),
    )
    expect(fakeMembers.calls.filter((call) => call.op === 'invite')).toHaveLength(0)
  })

  test('already-member emails warn without a POST', async () => {
    const { user } = renderMembersDialog()
    await screen.findByText('usr_a')

    await user.type(screen.getByLabelText('Invite by email'), 'a@example.com')
    await user.click(screen.getByRole('button', { name: 'Invite' }))

    await waitFor(() =>
      expect(toast.warning).toHaveBeenCalledWith('This user is already a member.'),
    )
    expect(fakeMembers.calls.filter((call) => call.op === 'invite')).toHaveLength(0)
  })

  test('plain members see a read-only roster without management controls', async () => {
    const { user } = renderMembersDialog()

    await user.click(screen.getByRole('button', { name: 'Team 3' }))

    const team = within(sectionByWorkspace('Team 3'))
    expect(team.getByText('Only owners and admins can manage members.')).toBeVisible()
    expect(team.queryByRole('button', { name: 'Invite' })).not.toBeInTheDocument()
    expect(team.queryByRole('button', { name: 'Remove' })).not.toBeInTheDocument()
    expect(team.queryByRole('button', { name: 'Leave' })).not.toBeInTheDocument()
    expect(team.queryByRole('combobox')).not.toBeInTheDocument()
    // Roles still read as text: owner row plus own member row.
    expect(team.getByText('boss@example.com')).toBeVisible()
    expect(team.getByText('Owner')).toBeVisible()
  })

  test('admins manage but cannot offer the owner role', async () => {
    const { user } = renderMembersDialog()

    await user.click(screen.getByRole('button', { name: 'Team 2' }))
    const team = within(sectionByWorkspace('Team 2'))

    // Admin view keeps the invite row and role dropdowns…
    expect(await team.findByRole('button', { name: 'Invite' })).toBeVisible()
    // …but the owner option is not rendered at all for non-owners (no
    // disabled placeholder; the server still guards 403).
    const trigger = await team.findByRole('combobox', { name: 'Role for boss@example.com' })
    await user.click(trigger)
    expect(await screen.findByRole('option', { name: 'Member' })).toBeVisible()
    expect(screen.getByRole('option', { name: 'Admin' })).toBeVisible()
    expect(screen.queryByRole('option', { name: 'Owner' })).not.toBeInTheDocument()
  })

  test('member writes stay isolated per workspace', async () => {
    const { user } = renderMembersDialog()

    await user.click(screen.getByRole('button', { name: 'Team 2' }))
    const team = within(sectionByWorkspace('Team 2'))
    await team.findByText('boss@example.com')

    await user.type(team.getByLabelText('Invite by email'), 'new@example.com')
    await user.click(team.getByRole('button', { name: 'Invite' }))

    await waitFor(() =>
      expect(fakeMembers.calls).toContainEqual(
        expect.objectContaining({ op: 'invite', workspaceId: 'ws_team2', userId: 'usr_new' }),
      ),
    )
    // The Team 2 section converges; the Personal roster is untouched.
    await waitFor(() => expect(team.getByText('usr_new')).toBeVisible())
    expect(
      fakeMembers.rosters.get('ws_personal1')?.some((entry) => entry.userId === 'usr_new'),
    ).toBe(false)
    expect(screen.queryByRole('region', { name: 'Members of Default' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Default' }))
    expect(within(sectionByWorkspace('Default')).queryByText('usr_new')).toBeNull()
  })

  test('renders roster avatars from entry avatar_url and no image without one', async () => {
    fakeMembers.rosters.set('ws_personal1', [
      { userId: ME, role: 'owner', email: 'me@example.com' },
      {
        userId: 'usr_a',
        role: 'member',
        email: 'a@example.com',
        avatar_url: '/api/v1/public/users/usr_a/avatar?v=3',
      },
      { userId: 'usr_b', role: 'member', email: 'b@example.com' },
    ])
    renderMembersDialog()

    const section = within(await screen.findByRole('region', { name: 'Members of Default' }))
    // Wait for the roster payload: the region shell renders before the query
    // resolves, and an early query would see zero images.
    await section.findByText('a@example.com')
    // Images stay `display:none` until loaded in happy-dom (excluded from
    // accessible-name computation), so collect the rendered imgs and assert
    // their alt/src pairs: exactly the avatar-carrying member renders one.
    const imgs = section.getAllByRole('img', { hidden: true })
    expect(imgs.map((img) => [img.getAttribute('alt'), img.getAttribute('src')])).toEqual([
      ['a@example.com', '/api/v1/public/users/usr_a/avatar?v=3&size=64'],
    ])
  })

  test('falls back to the viewer session avatar for the viewer entry', async () => {
    const store = useUserStore()
    store.setAuth(
      makeSessionUser({ public_id: ME, avatar: '/api/v1/public/users/usr_me/avatar?v=7' }),
    )
    try {
      fakeMembers.rosters.set('ws_personal1', [
        { userId: ME, role: 'owner', email: 'me@example.com' },
        { userId: 'usr_a', role: 'member', email: 'a@example.com' },
      ])
      renderMembersDialog()

      const section = within(await screen.findByRole('region', { name: 'Members of Default' }))
      await section.findByText('me@example.com')
      const imgs = section.getAllByRole('img', { hidden: true })
      expect(imgs.map((img) => [img.getAttribute('alt'), img.getAttribute('src')])).toEqual([
        ['me@example.com', '/api/v1/public/users/usr_me/avatar?v=7&size=64'],
      ])
    } finally {
      store.clearAuth()
    }
  })

  test('roster failures render retry and converge on retry', async () => {
    const { user } = renderMembersDialog()
    fakeMembers.failNext = { op: 'list', code: 'workspace_forbidden' }

    await user.click(screen.getByRole('button', { name: 'Team 2' }))

    const team = within(sectionByWorkspace('Team 2'))
    expect(await team.findByText('Members could not be loaded.')).toBeVisible()
    await user.click(team.getByRole('button', { name: 'Retry' }))

    await waitFor(() => expect(team.getByText('boss@example.com')).toBeVisible())
  })
})
