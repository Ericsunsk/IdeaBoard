import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import type { RenderOptions } from '@testing-library/vue'
import { renderWithUser as renderWithUserBase, screen, waitFor } from '../../../../test/render'
import WorkspaceSettingsDialog from './WorkspaceSettingsDialog.vue'
import type { UserAccountMenuOrganizationPanel } from './userAccountMenu.types'

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

const WORKSPACE = { publicId: 'ws_1', name: 'W', organizationId: null, isDefault: false }

function renderDialog(overrides?: {
  orgs?: { publicId: string; name: string }[]
  members?: { userId: string; role: 'owner' | 'admin' | 'member' }[]
  currentUserId?: string | null
  items?: (typeof WORKSPACE)[]
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
  return renderWithUser(WorkspaceSettingsDialog, {
    props: {
      isOpen: true,
      items: overrides?.items ?? [WORKSPACE],
      selectedId: overrides?.items?.[0]?.publicId ?? 'ws_1',
      currentUserId: overrides?.currentUserId ?? 'usr_1',
      onRetry: vi.fn(),
      onCreate: vi.fn(),
      onRename: vi.fn(),
      onDelete: vi.fn(),
      onLoadOrganizations: vi.fn(async () => overrides?.orgs ?? []),
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
  })
}

describe('WorkspaceSettingsDialog organization panel', () => {
  it('exposes the management surface with the owner role once the organization exists', async () => {
    renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
    })
    await waitFor(() => expect(screen.getByTestId('org-panel')).toBeTruthy())
    expect(screen.getByTestId('org-panel').textContent).toContain('Acme')
    expect(screen.getByTestId('org-panel').textContent).toContain('owner')
    expect(screen.getByTestId('org-panel').textContent).toContain('true|true')
  })

  it('gates management for members while still allowing leave', async () => {
    renderDialog({
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'member' }],
    })
    await waitFor(() => expect(screen.getByTestId('org-panel')).toBeTruthy())
    expect(screen.getByTestId('org-panel').textContent).toContain('member')
    expect(screen.getByTestId('org-panel').textContent).toContain('false|true')
  })

  it('renders no management surface without an organization', async () => {
    renderDialog({ orgs: [] })
    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy())
    expect(screen.queryByTestId('org-panel')).toBeNull()
  })

  it('keeps organization management available when the active scope has no workspace', async () => {
    renderDialog({
      items: [],
      orgs: [{ publicId: 'org_c3', name: 'Acme' }],
      members: [{ userId: 'usr_1', role: 'owner' }],
    })

    await waitFor(() => expect(screen.getByTestId('org-panel')).toBeTruthy())
    expect(screen.getByTestId('org-panel').textContent).toContain('Acme')
    expect(screen.getByText('No workspaces yet.')).toBeTruthy()
  })
})
