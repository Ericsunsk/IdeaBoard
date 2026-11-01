import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import OverviewPanel from './OverviewPanel.vue'
import { useManagement } from '../session/useManagement'

const resource = vi.hoisted(() => ({
  fetchOrganizations: vi.fn(),
  fetchOrganizationMembers: vi.fn(),
  fetchOrganizationInvites: vi.fn(),
  closeOrganization: vi.fn(),
  removeOrganizationMember: vi.fn(),
  renameOrganization: vi.fn(),
}))
vi.mock('../../selection/workspaceResource', async (load) => ({
  ...(await load<typeof import('../../selection/workspaceResource')>()),
  ...resource,
}))
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock('../../../../core/composables/useAppToast', () => ({ useAppToast: () => toast }))
const DialogRootStub = defineComponent({
  props: { open: Boolean },
  template: '<div v-if="open"><slot /></div>',
})
const Passthrough = defineComponent({ template: '<slot />' })
const DialogContent = defineComponent({ template: '<section role="dialog"><slot /></section>' })
function renderManagement(identity = 'usr_owner') {
  const onDismiss = vi.fn()
  const view = renderWithUser(
    defineComponent({
      setup() {
        const management = useManagement({
          identity,
          isEnabled: true,
          onScopeExit: vi.fn(),
        })
        return () => h(OverviewPanel, { management, onDismiss })
      },
    }),
    {
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
          DialogContent,
          DialogTitle: Passthrough,
          DialogDescription: Passthrough,
        },
      },
    },
  )
  return { ...view, onDismiss }
}
async function ready() {
  await screen.findByRole('button', { name: 'admin@example.com' })
}
beforeEach(() => {
  vi.resetAllMocks()
  resource.fetchOrganizations.mockResolvedValue([
    { publicId: 'org_team', name: 'Acme', status: 'active' },
  ])
  resource.fetchOrganizationMembers.mockResolvedValue([
    { userId: 'usr_owner', email: 'owner@example.com', role: 'owner' },
    {
      userId: 'usr_admin',
      email: 'admin@example.com',
      role: 'admin',
      avatar_url: 'https://example.com/admin.png',
    },
    { userId: 'usr_member', role: 'member' },
  ])
  resource.fetchOrganizationInvites.mockResolvedValue([])
  resource.removeOrganizationMember.mockResolvedValue(undefined)
})

describe('Organization overview presentation', () => {
  it('renders the roster, role badges and fallback names', async () => {
    renderManagement()
    await ready()
    expect(screen.getByRole('button', { name: 'owner@example.com' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'usr_member' })).toBeVisible()
    expect(screen.getByTitle('Owner')).toBeTruthy()
    expect(screen.getByTitle('Admin')).toBeTruthy()
    expect(screen.queryByTestId('org-pending-invites')).toBeNull()
  })
  it('keeps the rename draft and reports a server rename failure through toast only', async () => {
    resource.renameOrganization.mockRejectedValue(new Error('network'))
    const { user } = renderManagement()
    await ready()
    await user.click(screen.getByRole('button', { name: 'Rename Acme' }))
    const input = screen.getByRole('textbox', { name: 'Rename Acme' })
    await waitFor(() => expect(input).toHaveFocus())
    await user.clear(input)
    await user.type(input, 'Renamed{Enter}')
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        'Could not rename the organization. Please try again.',
      ),
    )
    expect(screen.queryByRole('alert')).toBeNull()
    expect(input).toHaveValue('Renamed')
    expect(resource.renameOrganization).toHaveBeenCalledWith('org_team', { name: 'Renamed' })
  })
  it('requires the typed organization name before permanent closure and dismisses on success', async () => {
    resource.closeOrganization.mockImplementation(async () => {
      resource.fetchOrganizations.mockResolvedValue([])
    })
    const { user, onDismiss } = renderManagement()
    await ready()
    await user.click(screen.getByRole('button', { name: 'Close organization' }))
    const confirm = screen.getByRole('button', { name: 'Confirm closure' })
    expect(confirm).toBeDisabled()
    await user.type(screen.getByPlaceholderText('Organization name'), 'Acme')
    await user.click(confirm)
    await waitFor(() => expect(onDismiss).toHaveBeenCalledTimes(1))
    expect(resource.closeOrganization).toHaveBeenCalledWith(
      'org_team',
      {},
      { idempotencyKey: expect.any(String) },
    )
  })
  it('reports closure rejection through toast only and never offers restoration', async () => {
    resource.closeOrganization.mockRejectedValue({ code: 'organization_closure_blocked' })
    const { user } = renderManagement()
    await ready()
    await user.click(screen.getByRole('button', { name: 'Close organization' }))
    await user.type(screen.getByPlaceholderText('Organization name'), 'Acme')
    await user.click(screen.getByRole('button', { name: 'Confirm closure' }))
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining('unsettled tasks or funds')),
    )
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Restore organization' })).toBeNull()
  })
  it('requires removal confirmation from the member menu', async () => {
    const { user } = renderManagement()
    await ready()
    await user.click(screen.getByRole('button', { name: 'usr_member' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Remove' }))
    expect(resource.removeOrganizationMember).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirm remove' }))
    await waitFor(() =>
      expect(resource.removeOrganizationMember).toHaveBeenCalledWith('org_team', 'usr_member'),
    )
  })
  it('confirms leaving independently from member invitation', async () => {
    resource.removeOrganizationMember.mockImplementation(async () => {
      resource.fetchOrganizations.mockResolvedValue([])
    })
    const { user, onDismiss } = renderManagement('usr_admin')
    await ready()
    await user.click(screen.getByRole('button', { name: 'Leave' }))
    expect(screen.getByRole('dialog', { name: 'Leave organization' })).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Confirm leave' }))
    await waitFor(() => expect(onDismiss).toHaveBeenCalledTimes(1))
    expect(resource.removeOrganizationMember).toHaveBeenCalledWith('org_team', 'usr_admin')
  })
})
