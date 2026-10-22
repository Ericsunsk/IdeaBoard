import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import OrganizationPanel from './OrganizationPanel.vue'
import type { OrganizationMemberSummary } from '../../selection/workspaceResource'

const MEMBERS: OrganizationMemberSummary[] = [
  { userId: 'usr_owner', role: 'owner' },
  { userId: 'usr_admin', role: 'admin' },
  { userId: 'usr_member', role: 'member' },
]

function renderManagement(overrides?: {
  actorRole?: 'owner' | 'admin' | 'member'
  currentUserId?: string
}) {
  return renderWithUser(OrganizationPanel, {
    props: {
      organizationId: 'org_c3',
      organizationName: 'Acme',
      actorRole: overrides?.actorRole ?? 'owner',
      currentUserId: overrides?.currentUserId ?? 'usr_owner',
      canManage: true,
      canLeave: true,
      onLoadMembers: vi.fn(async () => MEMBERS),
      onInviteMember: vi.fn(async () => ({ userId: 'usr_new', role: 'member' as const })),
      onUpdateMember: vi.fn(async (_org: string, memberId: string) => ({
        userId: memberId,
        role: 'member' as const,
      })),
      onRemoveMember: vi.fn(async () => {}),
    },
  })
}

describe('OrganizationPanel', () => {
  it('lists organization members with the management note', async () => {
    renderManagement()
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    expect(screen.getByText('usr_member')).toBeTruthy()
  })

  it('hides the admin invite option for non-owners', async () => {
    renderManagement({ actorRole: 'admin', currentUserId: 'usr_admin' })
    await waitFor(() => expect(screen.getByText('usr_member')).toBeTruthy())
    const options = screen.getAllByRole('option').map((o) => (o as HTMLOptionElement).value)
    expect(options).not.toContain('owner')
  })
})

// Keep the shared typed-confirm controls real and replace only the Reka
// shell, as in UserConfirmDialog specs (jsdom does not load overlay CSS).
const DialogRootStub = defineComponent({
  props: { open: { type: Boolean, required: true } },
  template: '<div v-if="open"><slot /></div>',
})
const DialogPassthroughStub = defineComponent({ template: '<slot />' })
const DialogContentStub = defineComponent({
  template: '<section role="dialog"><slot /></section>',
})

describe('OrganizationPanel closure (955)', () => {
  function renderClosure(
    overrides?: Partial<Parameters<typeof renderManagement>[0]> & {
      organizationStatus?: 'active' | 'closed'
      onCloseOrganization?: (
        organizationId: string,
        input: { reason?: string | undefined },
      ) => Promise<{
        status?: string
      }>
    },
  ) {
    return renderWithUser(OrganizationPanel, {
      global: {
        stubs: {
          DialogRoot: DialogRootStub,
          DialogPortal: DialogPassthroughStub,
          DialogOverlay: DialogPassthroughStub,
          DialogContent: DialogContentStub,
          DialogDescription: DialogPassthroughStub,
          DialogTitle: DialogPassthroughStub,
        },
      },
      props: {
        organizationId: 'org_c3',
        organizationName: 'Acme',
        organizationStatus: overrides?.organizationStatus ?? 'active',
        actorRole: overrides?.actorRole ?? 'owner',
        currentUserId: overrides?.currentUserId ?? 'usr_owner',
        canManage: true,
        canLeave: true,
        onLoadMembers: vi.fn(async () => MEMBERS),
        onInviteMember: vi.fn(async () => ({ userId: 'usr_new', role: 'member' as const })),
        onUpdateMember: vi.fn(async (_org: string, memberId: string) => ({
          userId: memberId,
          role: 'member' as const,
        })),
        onRemoveMember: vi.fn(async () => {}),
        ...(overrides?.onCloseOrganization
          ? { onCloseOrganization: overrides.onCloseOrganization }
          : {}),
      },
    })
  }

  it('requires the organization name before permanently closing with a reason', async () => {
    const onCloseOrganization = vi.fn(async () => ({ status: 'closed' }))
    const { user } = renderClosure({ onCloseOrganization })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    await user.type(screen.getByPlaceholderText('Reason (optional)'), 'winding down')
    await user.click(screen.getByRole('button', { name: 'Close organization' }))
    const confirm = screen.getByRole('button', { name: 'Confirm closure' })
    expect(confirm).toBeDisabled()
    expect(onCloseOrganization).not.toHaveBeenCalled()
    await user.type(screen.getByPlaceholderText('Organization name'), 'Acme')
    await user.click(confirm)
    await waitFor(() =>
      expect(onCloseOrganization).toHaveBeenCalledWith('org_c3', { reason: 'winding down' }),
    )
    await waitFor(() => expect(screen.getByText('Permanently closed')).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Restore organization' })).toBeNull()
  })

  it('surfaces the blocked copy when closure is refused', async () => {
    const onCloseOrganization = vi.fn(async () => {
      throw { code: 'organization_closure_blocked' }
    })
    const { user } = renderClosure({ onCloseOrganization })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    await user.click(screen.getByRole('button', { name: 'Close organization' }))
    await user.type(screen.getByPlaceholderText('Organization name'), 'Acme')
    await user.click(screen.getByRole('button', { name: 'Confirm closure' }))
    await waitFor(() => expect(screen.getByText(/unsettled tasks or funds/)).toBeTruthy())
  })

  it('shows permanent closure without a restoration action', async () => {
    renderClosure({
      organizationStatus: 'closed',
      onCloseOrganization: vi.fn(async () => ({ status: 'closed' })),
    })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    expect(screen.queryByPlaceholderText('usr_…')).toBeNull()
    expect(screen.getByText('Permanently closed')).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Close organization' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Restore organization' })).toBeNull()
  })

  it('cancels the confirmation without closing', async () => {
    const onCloseOrganization = vi.fn(async () => ({ status: 'closed' }))
    const { user } = renderClosure({ onCloseOrganization })
    await user.click(screen.getByRole('button', { name: 'Close organization' }))
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onCloseOrganization).not.toHaveBeenCalled()
    expect(screen.queryByPlaceholderText('Organization name')).toBeNull()
  })

  it('hides the danger zone from non-owners', async () => {
    renderClosure({
      actorRole: 'member',
      currentUserId: 'usr_member',
      onCloseOrganization: vi.fn(async () => ({ status: 'closed' })),
    })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Close organization' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Restore organization' })).toBeNull()
  })
})
