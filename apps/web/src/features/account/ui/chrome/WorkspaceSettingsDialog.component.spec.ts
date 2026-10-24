import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RenderOptions } from '@testing-library/vue'
import { renderWithUser as renderWithUserBase, screen, waitFor } from '../../../../test/render'
import WorkspaceSettingsDialog from './WorkspaceSettingsDialog.vue'
import type {
  UserAccountMenuOrganizationPanel,
  UserAccountMenuWorkspace,
  UserAccountMenuWorkspaceItem,
} from './userAccountMenu.types'

function withQuery<C>(options?: RenderOptions<C>): RenderOptions<C> {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return {
    ...options,
    global: {
      ...options?.global,
      plugins: [...(options?.global?.plugins ?? []), [VueQueryPlugin, { queryClient }]],
    },
  }
}
function renderWithUser<C>(view: C, options?: RenderOptions<C>) {
  return renderWithUserBase(view, withQuery(options))
}

// Mirror the UserAccountMenu harness: stub the Reka dialog shell and the
// workspace-row sections so these specs focus on organization panel wiring.
const DialogRootStub = defineComponent({
  props: {
    open: { type: Boolean, required: true },
  },
  emits: ['update:open'],
  template: `
    <div v-if="open" @keydown.escape="$emit('update:open', false)">
      <slot />
    </div>
  `,
})

const DialogPassthroughStub = defineComponent({
  template: '<slot />',
})

const DialogContentStub = defineComponent({
  inheritAttrs: false,
  template: `
    <section role="dialog" tabindex="-1" v-bind="$attrs">
      <slot />
    </section>
  `,
})

vi.mock('../../../../core/composables/useAppToast')
vi.mock('../../session/billing-account/resource', async (load) => ({
  ...(await load<typeof import('../../session/billing-account/resource')>()),
  fetchBillingAccount: vi.fn().mockRejectedValue(new Error('no billing account')),
}))
// Dialog-level single roster provider fires one workspace-members read per
// active workspace; these organization-panel specs stub the member sections,
// so resolve the roster to empty instead of hitting the network.
vi.mock('../../session/accountWorkspace', async (load) => ({
  ...(await load<typeof import('../../session/accountWorkspace')>()),
  workspaceMembersQueryOptions: (workspaceId: import('vue').MaybeRefOrGetter<string>) => ({
    queryKey: ['workspace-members', workspaceId],
    queryFn: vi.fn().mockResolvedValue([]),
  }),
}))
vi.mock('./BillingTransferSection.vue', () => ({ default: { template: '<div />' } }))
vi.mock('./WorkspaceMemberSection.vue', () => ({ default: { template: '<div />' } }))
vi.mock('./WorkspaceLimitsSection.vue', () => ({ default: { template: '<div />' } }))
vi.mock('./WorkspaceCreateDialog.vue', () => ({ default: { template: '<div />' } }))

const PanelStub = defineComponent({
  props: {
    organizationId: { type: String, required: true },
    organizationName: { type: String, required: true },
    actorRole: { type: String, default: null },
    canManage: { type: Boolean, default: false },
    canLeave: { type: Boolean, default: false },
  },
  template: `
    <section data-testid="org-panel">
      {{ organizationName }}|{{ actorRole }}|{{ canManage }}|{{ canLeave }}
    </section>
  `,
})

const WORKSPACE: UserAccountMenuWorkspaceItem = {
  publicId: 'ws_1',
  name: 'W',
  organizationId: null,
  isDefault: false,
}

function renderDialog(overrides?: {
  orgs?: { publicId: string; name: string }[]
  members?: { userId: string; role: 'owner' | 'admin' | 'member' }[]
  currentUserId?: string | null
  items?: (typeof WORKSPACE)[]
  onRenameOrganization?: UserAccountMenuWorkspace['onRenameOrganization']
  onCreateOrganization?: UserAccountMenuWorkspace['onCreateOrganization']
  onLoadOrganizations?: UserAccountMenuWorkspace['onLoadOrganizations']
}) {
  const panel: UserAccountMenuOrganizationPanel = {
    component: PanelStub,
    onLoadMembers: vi.fn(async () => overrides?.members ?? []),
    onInviteMember: vi.fn(async (_org: string, input: { userId: string }) => ({
      userId: input.userId,
      role: 'member' as const,
    })),
    onUpdateMember: vi.fn(async (_org: string, memberId: string) => ({
      userId: memberId,
      role: 'member' as const,
    })),
    onRemoveMember: vi.fn(async () => {}),
    onLeaveMember: vi.fn(async () => {}),
  }
  return {
    view: renderWithUser(WorkspaceSettingsDialog, {
      props: {
        isOpen: true,
        items: overrides?.items ?? [WORKSPACE],
        selectedId: overrides?.items?.[0]?.publicId ?? 'ws_1',
        currentUserId: overrides?.currentUserId ?? 'usr_1',
        onRetry: vi.fn(),
        onCreate: vi.fn(),
        onRename: vi.fn(),
        onDelete: vi.fn(),
        onLoadOrganizations:
          overrides?.onLoadOrganizations ?? vi.fn(async () => overrides?.orgs ?? []),
        ...(overrides?.onCreateOrganization === undefined
          ? {}
          : { onCreateOrganization: overrides.onCreateOrganization }),
        ...(overrides?.onRenameOrganization === undefined
          ? {}
          : { onRenameOrganization: overrides.onRenameOrganization }),
        organizationPanel: panel,
      },
      global: {
        stubs: {
          DialogRoot: DialogRootStub,
          DialogPortal: DialogPassthroughStub,
          DialogOverlay: DialogPassthroughStub,
          DialogContent: DialogContentStub,
          DialogTitle: DialogPassthroughStub,
          DialogDescription: DialogPassthroughStub,
        },
      },
    }),
    panel,
  }
}

async function openOrganizationSettings(user: {
  click: (el: Element) => Promise<void>
}): Promise<void> {
  await user.click(screen.getByRole('button', { name: 'Settings' }))
  await waitFor(() => expect(screen.getByTestId('org-panel')).toBeTruthy())
}

describe('WorkspaceSettingsDialog organization panel', () => {
  it('groups rows into Personal and Organization with a leading Settings row', async () => {
    renderDialog({
      items: [
        { publicId: 'ws_personal', name: 'Personal space', organizationId: null, isDefault: false },
        { publicId: 'ws_org', name: 'Team', organizationId: 'org_c3', isDefault: false },
      ],
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
    })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())

    expect(screen.getByText('Personal')).toBeTruthy()
    expect(screen.getByText('Acme')).toBeTruthy()
    // Organization group leads with Settings, then its workspaces.
    const orgGroup = screen.getByText('Acme').closest('section') as HTMLElement | null
    expect(orgGroup?.textContent).toMatch(/Settings.*Team/)
    expect(screen.getByRole('button', { name: 'Settings' })).toBeVisible()
    // Settings is selected independently: workspace rows stay unselected.
    expect(screen.getByRole('button', { name: 'Team' })).not.toHaveAttribute('aria-current', 'page')
  })

  it('titles the Organization group generically without an organization', async () => {
    renderDialog({ orgs: [] })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    expect(screen.getByText('Organization')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Settings' })).toBeVisible()
  })

  it('exposes the management surface with the owner role once the organization exists', async () => {
    const { view } = renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
    })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    expect(screen.queryByTestId('org-panel')).toBeNull()
    await openOrganizationSettings(view.user)
    expect(screen.getByTestId('org-panel').textContent).toContain('Acme')
    expect(screen.getByTestId('org-panel').textContent).toContain('owner')
    expect(screen.getByTestId('org-panel').textContent).toContain('true|true')
  })

  it('gates management for members while still allowing leave', async () => {
    const { view } = renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'member' }],
    })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    await openOrganizationSettings(view.user)
    expect(screen.getByTestId('org-panel').textContent).toContain('member')
    expect(screen.getByTestId('org-panel').textContent).toContain('false|true')
  })

  it('keeps workspace and Settings selection mutually exclusive', async () => {
    const { view } = renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
    })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())

    await openOrganizationSettings(view.user)
    expect(screen.getByRole('button', { name: 'Settings' })).toHaveAttribute('aria-current', 'page')

    await view.user.click(screen.getByRole('button', { name: 'W' }))
    await waitFor(() => expect(screen.queryByTestId('org-panel')).toBeNull())
    expect(screen.getByRole('button', { name: 'W' })).toHaveAttribute('aria-current', 'page')
    expect(screen.getByRole('button', { name: 'Settings' })).not.toHaveAttribute(
      'aria-current',
      'page',
    )
  })

  it('renders no management surface without an organization', async () => {
    renderDialog({ orgs: [] })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    expect(screen.queryByTestId('org-panel')).toBeNull()
  })

  it('keeps organization management available when the active scope has no workspace', async () => {
    const { view } = renderDialog({
      items: [],
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
    })

    await openOrganizationSettings(view.user)
    expect(screen.getByTestId('org-panel').textContent).toContain('Acme')
    expect(screen.getByText('No workspaces yet.')).toBeTruthy()
  })

  it('creates the organization from the Settings pane when none exists', async () => {
    const onLoadOrganizations = vi.fn(
      async () => [{ publicId: 'org_new', name: 'Acme' }] as { publicId: string; name: string }[],
    )
    // First load (dialog open) sees no organization, so Settings shows the
    // create form; the reload after creation resolves the new organization.
    onLoadOrganizations.mockResolvedValueOnce([])
    const onCreateOrganization = vi.fn(async () => ({ publicId: 'org_new', name: 'Acme' }))
    const { view } = renderDialog({ items: [], onLoadOrganizations, onCreateOrganization })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())

    await view.user.click(screen.getByRole('button', { name: 'Settings' }))
    await waitFor(() => expect(screen.getByLabelText('Organization name')).toBeTruthy())

    await view.user.type(screen.getByLabelText('Organization name'), 'Acme')
    await view.user.click(screen.getByRole('button', { name: 'Create organization account' }))

    await waitFor(() =>
      expect(onCreateOrganization).toHaveBeenCalledWith(
        'Acme',
        expect.stringContaining('organization-create:'),
      ),
    )
    await waitFor(() => expect(screen.getByTestId('org-panel')).toBeTruthy())
    // Group title and Settings header converge on the new organization name.
    expect(screen.getAllByText('Acme').length).toBeGreaterThanOrEqual(2)
  })

  it('renames the organization through PATCH and converges the group title', async () => {
    const onRenameOrganization = vi.fn(async () => ({ publicId: 'org_c3', name: 'Renamed' }))
    const { view } = renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
      onRenameOrganization,
    })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    await openOrganizationSettings(view.user)

    await view.user.click(screen.getByRole('button', { name: 'Rename Acme' }))
    const input = screen.getByLabelText('Rename Acme')
    await view.user.clear(input)
    await view.user.type(input, 'Renamed')
    await view.user.keyboard('{Enter}')

    await waitFor(() => expect(onRenameOrganization).toHaveBeenCalledWith('org_c3', 'Renamed'))
    await waitFor(() => expect(screen.getAllByText('Renamed').length).toBeGreaterThanOrEqual(2))
    expect(screen.queryByLabelText('Rename Acme')).toBeNull()
  })

  it('surfaces organization rename failures inline without renaming', async () => {
    const onRenameOrganization = vi.fn(async () => {
      throw new Error('boom')
    })
    const { view } = renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
      onRenameOrganization,
    })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    await openOrganizationSettings(view.user)

    await view.user.click(screen.getByRole('button', { name: 'Rename Acme' }))
    const input = screen.getByLabelText('Rename Acme')
    await view.user.clear(input)
    await view.user.type(input, 'Renamed')
    await view.user.keyboard('{Enter}')

    await waitFor(() =>
      expect(screen.getByText('Could not rename the organization. Please try again.')).toBeTruthy(),
    )
    // Rename stays open on failure: the group title keeps the old name while
    // the header shows the editing input.
    expect(screen.getAllByText('Acme')).toHaveLength(1)
  })

  it('hides organization rename for members without manage permission', async () => {
    const { view } = renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'member' }],
      onRenameOrganization: vi.fn(),
    })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    await openOrganizationSettings(view.user)

    expect(screen.queryByRole('button', { name: 'Rename Acme' })).toBeNull()
  })
})
