import { describe, expect, it, vi } from 'vitest'
import { defineComponent, markRaw } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { renderWithUser, screen, waitFor } from '@/test/render'
import WorkspaceSettingsDialog from './WorkspaceSettingsDialog.vue'
import type {
  UserAccountMenuOrganizationPanel,
  UserAccountMenuWorkspaceItem,
} from '@/features/account/ui/chrome/userAccountMenu.types'

vi.mock('@/core/composables/useAppToast')
vi.mock('@/features/account/session/billing-account/resource', async (load) => ({
  ...(await load<typeof import('@/features/account/session/billing-account/resource')>()),
  fetchBillingAccount: vi.fn().mockRejectedValue(new Error('no billing account')),
}))
vi.mock('@/features/account/session/accountWorkspace', async (load) => ({
  ...(await load<typeof import('@/features/account/session/accountWorkspace')>()),
  workspaceMembersQueryOptions: () => ({
    queryKey: ['workspace-members'],
    queryFn: async () => [],
  }),
}))
vi.mock('../members/WorkspaceMemberSection.vue', () => ({ default: { template: '<div />' } }))
vi.mock('./WorkspaceLimitsSection.vue', () => ({ default: { template: '<div />' } }))
vi.mock('../dialogs/WorkspaceCreateDialog.vue', () => ({ default: { template: '<div />' } }))

const DialogRootStub = defineComponent({
  props: { open: Boolean },
  template: '<div v-if="open"><slot /></div>',
})
const Passthrough = defineComponent({ template: '<slot />' })
const Content = defineComponent({ template: '<section role="dialog"><slot /></section>' })
const Panel = defineComponent({
  props: { label: { type: String, required: true } },
  emits: ['dismiss'],
  template:
    '<section data-testid="org-panel">{{ label }}<button @click="$emit(\'dismiss\')">Done</button></section>',
})
const WORKSPACE: UserAccountMenuWorkspaceItem = {
  publicId: 'ws_1',
  name: 'W',
  organizationId: null,
  isDefault: false,
}
function renderDialog(items = [WORKSPACE], name: string | null = 'Acme') {
  const panel: UserAccountMenuOrganizationPanel = {
    component: markRaw(Panel),
    bindings: { label: 'Opaque management' },
    name,
    setEnabled: vi.fn(),
    refresh: vi.fn(async () => {}),
  }
  const view = renderWithUser(WorkspaceSettingsDialog, {
    props: {
      isOpen: true,
      items,
      currentUserId: 'usr_1',
      organizationPanel: panel,
      onRetry: vi.fn(),
      onCreate: vi.fn(),
      onRename: vi.fn(),
      onDelete: vi.fn(),
    },
    global: {
      plugins: [
        [
          VueQueryPlugin,
          { queryClient: new QueryClient({ defaultOptions: { queries: { retry: false } } }) },
        ],
      ],
      stubs: {
        DialogRoot: DialogRootStub,
        DialogPortal: Passthrough,
        DialogOverlay: Passthrough,
        DialogContent: Content,
        DialogTitle: Passthrough,
        DialogDescription: Passthrough,
      },
    },
  })
  return { view, panel }
}

describe('WorkspaceSettingsDialog opaque organization host', () => {
  it('groups workspaces and uses the module title without reading its members or roles', () => {
    renderDialog([
      WORKSPACE,
      { ...WORKSPACE, publicId: 'ws_org', name: 'Team', organizationId: 'org_1' },
    ])
    expect(screen.getByText('Personal')).toBeVisible()
    expect(screen.getByText('Acme').closest('section')?.textContent).toMatch(/Overview.*Team/)
    expect(screen.queryByTestId('org-panel')).toBeNull()
  })
  it('renders the opaque module independently of Workspace selection, including an empty organization', async () => {
    const { view, panel } = renderDialog([], null)
    expect(panel.setEnabled).toHaveBeenCalledWith(true)
    await view.user.click(screen.getByRole('button', { name: 'Overview' }))
    expect(screen.getByTestId('org-panel')).toHaveTextContent('Opaque management')
    expect(screen.getByRole('button', { name: 'Overview' })).toHaveAttribute('aria-current', 'page')
  })
  it('keeps Settings selection local and closes when the opaque module dismisses', async () => {
    const { view } = renderDialog()
    await view.user.click(screen.getByRole('button', { name: 'Overview' }))
    await view.user.click(screen.getByRole('button', { name: 'W' }))
    expect(screen.queryByTestId('org-panel')).toBeNull()
    expect(screen.getByRole('button', { name: 'W' })).toHaveAttribute('aria-current', 'page')
    await view.user.click(screen.getByRole('button', { name: 'Overview' }))
    await view.user.click(screen.getByRole('button', { name: 'Done' }))
    expect(view.emitted('update:isOpen')).toContainEqual([false])
  })
  it('disables the module when the dialog closes', async () => {
    const { view, panel } = renderDialog()
    await view.rerender({ isOpen: false })
    await waitFor(() => expect(panel.setEnabled).toHaveBeenLastCalledWith(false))
  })
})
