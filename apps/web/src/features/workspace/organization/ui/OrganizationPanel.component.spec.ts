import { describe, expect, it, vi } from 'vitest'
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
      onRestoreOrganization?: (organizationId: string) => Promise<{ status?: string }>
    },
  ) {
    return renderWithUser(OrganizationPanel, {
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
        ...(overrides?.onRestoreOrganization
          ? { onRestoreOrganization: overrides.onRestoreOrganization }
          : {}),
      },
    })
  }

  it('closes with a reason after an explicit confirm step', async () => {
    const onCloseOrganization = vi.fn(async () => ({ status: 'closed' }))
    const { user } = renderClosure({ onCloseOrganization })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    await user.click(screen.getByRole('button', { name: 'Close organization' }))
    const reason = screen.getByPlaceholderText('Reason (optional)')
    await user.type(reason, 'winding down')
    await user.click(screen.getByRole('button', { name: 'Confirm closure' }))
    await waitFor(() =>
      expect(onCloseOrganization).toHaveBeenCalledWith('org_c3', { reason: 'winding down' }),
    )
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Restore organization' })).toBeTruthy(),
    )
  })

  it('surfaces the blocked copy when closure is refused', async () => {
    const onCloseOrganization = vi.fn(async () => {
      throw { code: 'organization_closure_blocked' }
    })
    const { user } = renderClosure({ onCloseOrganization })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    await user.click(screen.getByRole('button', { name: 'Close organization' }))
    await user.click(screen.getByRole('button', { name: 'Confirm closure' }))
    await waitFor(() => expect(screen.getByText(/unsettled tasks or funds/)).toBeTruthy())
  })

  it('hides member management and offers restore while closed', async () => {
    const onRestoreOrganization = vi.fn(async () => ({ status: 'active' }))
    const { user } = renderClosure({ organizationStatus: 'closed', onRestoreOrganization })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    expect(screen.queryByPlaceholderText('usr_…')).toBeNull()
    expect(screen.getByText('Closed')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Restore organization' }))
    await waitFor(() => expect(onRestoreOrganization).toHaveBeenCalledWith('org_c3'))
  })

  it('hides the danger zone from non-owners', async () => {
    renderClosure({
      actorRole: 'member',
      currentUserId: 'usr_member',
      onCloseOrganization: vi.fn(async () => ({ status: 'closed' })),
      onRestoreOrganization: vi.fn(async () => ({ status: 'active' })),
    })
    await waitFor(() => expect(screen.getByText('usr_admin')).toBeTruthy())
    expect(screen.queryByRole('button', { name: 'Close organization' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Restore organization' })).toBeNull()
  })
})
