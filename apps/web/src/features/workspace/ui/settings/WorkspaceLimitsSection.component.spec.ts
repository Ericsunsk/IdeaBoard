import { beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '@/core/i18n'
import { renderWithUser, screen, waitFor } from '@/test/render'
import { mockAppToast, expectToastFeedback, type AppToastMocks } from '@/test/toast'
import { makeMemberQuotaSummary, makeUnlimitedMemberQuota } from '@/test/factories'
import WorkspaceLimitsSection from './WorkspaceLimitsSection.vue'
import WorkspaceSettingsDialog from './WorkspaceSettingsDialog.vue'
import type { MemberQuotaSummary } from '@/features/account/session/quota/resource'

vi.mock('@/core/composables/useAppToast')

const quotaTransport = vi.hoisted(() => ({
  fetchSelf: vi.fn(),
  fetchList: vi.fn(),
  update: vi.fn(),
}))

vi.mock('@/features/account/session/quota/resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/account/session/quota/resource')>()
  return {
    ...actual,
    fetchSelfMemberQuota: quotaTransport.fetchSelf,
    fetchMemberQuotaList: quotaTransport.fetchList,
    updateMemberQuota: quotaTransport.update,
  }
})

const rosterTransport = vi.hoisted(() => ({
  fetchMembers: vi.fn(),
  entries: [
    { userId: 'usr_a', role: 'member', email: 'a@example.com' },
    { userId: 'usr_b', role: 'admin', email: 'b@example.com' },
    { userId: 'usr_c', role: 'member', email: 'c@example.com' },
    { userId: 'usr_owner', role: 'owner', email: 'owner@example.com' },
  ],
}))

const organizationTransport = vi.hoisted(() => ({ fetchMembers: vi.fn() }))
const billingTransport = vi.hoisted(() => ({ fetchAccount: vi.fn() }))

vi.mock('@/features/account/session/billing-account/resource', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/account/session/billing-account/resource')>()
  return { ...actual, fetchBillingAccount: billingTransport.fetchAccount }
})

vi.mock('../../selection/workspaceResource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../selection/workspaceResource')>()
  return { ...actual, fetchOrganizationMembers: organizationTransport.fetchMembers }
})

vi.mock('@/features/account/session/accountWorkspace', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/account/session/accountWorkspace')>()
  return {
    ...actual,
    workspaceMembersQueryOptions: () => ({
      queryKey: ['test-quota-roster'],
      queryFn: rosterTransport.fetchMembers,
    }),
  }
})

function managedQuotas(): MemberQuotaSummary[] {
  return [
    makeMemberQuotaSummary(),
    makeMemberQuotaSummary({
      userId: 'usr_b',
      period: 'fixed',
      limitCredits: 200,
      usedCredits: 170,
      remainingCredits: 30,
    }),
    makeUnlimitedMemberQuota({ userId: 'usr_c', usedCredits: 40 }),
    makeMemberQuotaSummary({
      userId: 'usr_d',
      limitCredits: 200,
      usedCredits: 200,
      remainingCredits: 0,
    }),
    makeUnlimitedMemberQuota({ userId: 'usr_owner', usedCredits: 7 }),
  ]
}

function renderSection(canManage = false, currentUserId: string | null = null) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = renderWithUser(WorkspaceLimitsSection, {
    props: {
      workspaceId: 'ws_123',
      // Org-only contract: required organization id, never null (ADR-0042 §2).
      organizationId: 'org_1',
      canManage,
      isExpanded: true,
      // Owner actor by default for manager reads so the Org matrix (Owner
      // sets Admin, Owner/Admin set Member) exercises the enabled path;
      // pass an explicit id to cover Admin/member actors.
      currentUserId: currentUserId ?? (canManage ? 'usr_owner' : 'usr_a'),
    },
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
  return { ...view, queryClient }
}

async function openAdjustDialog(view: ReturnType<typeof renderSection>, rowName: string) {
  expect(await screen.findByText('a@example.com')).toBeVisible()
  await view.user.click(screen.getByRole('button', { name: rowName }))
  expect(await screen.findByText('Adjust member quota')).toBeVisible()
}

describe('WorkspaceLimitsSection', () => {
  // Org-only card (ADR-0042 §2): callers mount this section only for
  // Organization workspaces with a required organizationId; no Personal
  // empty state lives inside the component. All assertions below cover Org.
  let toast: AppToastMocks

  beforeEach(async () => {
    await setUserLocale('en')
    vi.clearAllMocks()
    toast = mockAppToast()
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue([])
    organizationTransport.fetchMembers.mockResolvedValue(rosterTransport.entries)
    rosterTransport.fetchMembers.mockResolvedValue(rosterTransport.entries)
  })

  test('renders the usage overview with quota blocks for members', async () => {
    renderSection()

    expect(await screen.findByText('Usage limits')).toBeVisible()
    expect(await screen.findByText('40 Ideas used')).toBeVisible()
    expect(screen.getByText('Member quota')).toBeVisible()
    expect(screen.queryByText('Default quota for new members')).toBeNull()
    // Collaborator usage charges the shared account and aggregates across
    // its workspaces, attributed to the real operator (#952).
    expect(
      screen.getByText(
        'Collaborator usage charges the shared account balance and is summed across every workspace of the account, attributed to the real operator.',
      ),
    ).toBeVisible()
    // Members see the read-only notes instead of management controls.
    expect(
      screen.getByText(/Only organization owners or admins can change member quotas/),
    ).toBeVisible()
    expect(screen.queryByRole('table', { name: 'Member usage table' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Set quota' })).toBeNull()
    expect(toast.success).not.toHaveBeenCalled()
  })

  test('hides the table and adjust entry from members', async () => {
    renderSection()

    await screen.findByText('Usage limits')
    expect(screen.queryByRole('table', { name: 'Member usage table' })).toBeNull()
    expect(screen.queryByText('Set first quota')).toBeNull()
    expect(screen.queryByRole('button', { name: /Adjust/ })).toBeNull()
  })

  test('does not request manager data when the server denies management', async () => {
    renderSection(false)

    expect(await screen.findByText('40 Ideas used')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Set quota' })).toBeNull()
    expect(quotaTransport.fetchList).not.toHaveBeenCalled()
    expect(organizationTransport.fetchMembers).not.toHaveBeenCalled()
  })

  test('renders self quota usage with remaining for members', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeMemberQuotaSummary())
    renderSection()

    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
    expect(screen.getByText('75 Ideas left')).toBeVisible()
  })

  test('warns when the self cap is reached', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(
      makeMemberQuotaSummary({ usedCredits: 100, remainingCredits: 0 }),
    )
    renderSection()

    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent(/Quota exhausted/)
  })

  test('renders unlimited self usage without a cap', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    renderSection()

    expect(await screen.findByText('40 Ideas used')).toBeVisible()
    expect(screen.queryByText('75 Ideas left')).toBeNull()
  })

  test('renders only the exhausted alert for authorized quota managers', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    renderSection(true)

    // Only non-derivable signal above the table; zero hides the whole line.
    expect(await screen.findByText(/1 exhausted/)).toBeVisible()
    expect(screen.queryByText('受管成员')).toBeNull()
    expect(screen.queryByText('本期已用')).toBeNull()
    expect(screen.queryByText('442 Ideas')).toBeNull()
    expect(screen.getByText('Exhausted')).toBeVisible()
    expect(screen.queryByText(/Only organization owners or admins/)).toBeNull()
  })

  test('renders the member usage table with status badges', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    renderSection(true)

    expect(await screen.findByRole('table', { name: 'Member usage table' })).toBeVisible()
    expect(await screen.findByText('a@example.com')).toBeVisible()
    for (const header of ['Member', 'Period', 'Used', 'Remaining', 'Status', 'Actions']) {
      expect(screen.getByText(header)).toBeVisible()
    }
    // Healthy capped quotas render as a textless neutral badge; only problem
    // states carry text.
    expect(screen.queryByText('Normal')).toBeNull()
    expect(screen.getByText('Running low')).toBeVisible()
    expect(screen.getAllByText('Unlimited')).toHaveLength(4)
    expect(screen.getByText('Exhausted')).toBeVisible()
    expect(screen.getByTitle('Less than 20% remaining')).toBeVisible()
  })

  test('disables the adjust action on owner rows', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    renderSection(true)

    expect(await screen.findByRole('table', { name: 'Member usage table' })).toBeVisible()
    expect(await screen.findByText('owner@example.com')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Adjust owner@example.com' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Adjust a@example.com' })).toBeEnabled()
  })

  test('keeps server-granted management visible but blocks writes for an unknown Org manager', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    // Organization role facts constrain targets; the server capability owns
    // visibility even when stale facts no longer identify a manager.
    renderSection(true, 'usr_a')

    expect(await screen.findByRole('table', { name: 'Member usage table' })).toBeVisible()
    expect(await screen.findByText('a@example.com')).toBeVisible()
    expect(screen.getByText('Actions')).toBeVisible()
    expect(screen.getByRole('button', { name: 'Set quota' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Adjust a@example.com' })).toBeDisabled()
    expect(quotaTransport.update).not.toHaveBeenCalled()
  })

  test('blocks Admin quota writes on Owner/Admin rows without transport', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    const view = renderSection(true, 'usr_b')

    expect(await screen.findByRole('table', { name: 'Member usage table' })).toBeVisible()
    expect(await screen.findByText('owner@example.com')).toBeVisible()
    // Owner sets Admin, Owner/Admin set Member: the Admin actor adjusts
    // Member rows only.
    expect(screen.getByRole('button', { name: 'Adjust owner@example.com' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Adjust b@example.com' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Adjust a@example.com' })).toBeEnabled()

    // Disabled clicks never open the dialog nor send.
    await view.user.click(screen.getByRole('button', { name: 'Adjust owner@example.com' }))
    expect(screen.queryByText('Adjust member quota')).toBeNull()
    await view.user.click(screen.getByRole('button', { name: 'Adjust b@example.com' }))
    expect(screen.queryByText('Adjust member quota')).toBeNull()
    expect(quotaTransport.update).not.toHaveBeenCalled()
  })

  test('rejects forbidden quota targets without transport', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    const view = renderSection(true, 'usr_owner')

    expect(await screen.findByRole('table', { name: 'Member usage table' })).toBeVisible()
    // The create entry waits for the roster-resolved manager gate.
    await view.user.click(await screen.findByRole('button', { name: 'Set quota' }))
    expect(await screen.findByText('Set member quota')).toBeVisible()

    // Owner rows stay unpickable for every actor.
    await view.user.click(screen.getByRole('combobox', { name: 'Member' }))
    expect(await screen.findByRole('option', { name: 'owner@example.com' })).toHaveAttribute(
      'aria-disabled',
      'true',
    )
    expect(screen.getByRole('option', { name: 'b@example.com' })).not.toHaveAttribute(
      'aria-disabled',
      'true',
    )
    await view.user.keyboard('{Escape}')

    // Arm an admin target as the Owner, then drop to the Admin actor: the same
    // target turns forbidden and the save must fail closed without transport.
    await view.user.click(screen.getByRole('combobox', { name: 'Member' }))
    await view.user.click(await screen.findByRole('option', { name: 'b@example.com' }))
    await view.rerender({
      workspaceId: 'ws_123',
      organizationId: 'org_1',
      canManage: true,
      isExpanded: true,
      currentUserId: 'usr_b',
    })
    const limitInput = screen.getByLabelText('Limit (Ideas)') as HTMLInputElement
    await view.user.clear(limitInput)
    await view.user.type(limitInput, '150')
    await view.user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('Could not save the quota. Please try again.'),
    )
    expect(quotaTransport.update).not.toHaveBeenCalled()
    expect(screen.getByText('Set member quota')).toBeVisible()
  })

  test('shows a skeleton overview while the manager list loads', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockReturnValueOnce(new Promise(() => {}))
    renderSection(true)

    expect(await screen.findByRole('status', { name: 'Loading…' })).toBeVisible()
    expect(screen.queryByText('受管成员')).toBeNull()
  })

  test('shows an empty state with a create-first action', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue([])
    const view = renderSection(true)

    expect(await screen.findByText('No member quotas')).toBeVisible()
    await view.user.click(screen.getByRole('button', { name: 'Set first quota' }))

    expect(await screen.findByText('Set member quota')).toBeVisible()
    await waitFor(() =>
      expect(document.activeElement?.getAttribute('id')).toBe('member-quota-edit-member'),
    )
  })

  test('recovers from list load failures with an inline retry', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce(managedQuotas())
    const view = renderSection(true)

    expect(await screen.findByText('Could not load the quota.')).toBeVisible()
    expect(screen.queryByRole('table', { name: 'Member usage table' })).toBeNull()
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByRole('table', { name: 'Member usage table' })).toBeVisible()
  })

  test('authorized quota managers adjust a member quota through the dialog', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    quotaTransport.update.mockResolvedValue(
      makeMemberQuotaSummary({ userId: 'usr_a', limitCredits: 150 }),
    )
    const view = renderSection(true)
    await screen.findByRole('table', { name: 'Member usage table' })
    await openAdjustDialog(view, 'Adjust a@example.com')

    const limitInput = screen.getByLabelText('Limit (Ideas)') as HTMLInputElement
    expect(limitInput.value).toBe('100')
    await view.user.clear(limitInput)
    await view.user.type(limitInput, '150')
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(quotaTransport.update).toHaveBeenCalledWith('ws_123', 'usr_a', {
        period: 'daily',
        limitCredits: 150,
      }),
    )
    expect(toast.success).toHaveBeenCalledWith('Member quota updated.')
    await waitFor(() => expect(screen.queryByText('Adjust member quota')).toBeNull())
  })

  test('switching to unlimited goes through the shared confirm before transport', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    quotaTransport.update.mockResolvedValue(makeUnlimitedMemberQuota({ userId: 'usr_a' }))
    const view = renderSection(true)
    await screen.findByRole('table', { name: 'Member usage table' })
    await openAdjustDialog(view, 'Adjust a@example.com')

    const periodTrigger = screen.getByRole('combobox', { name: 'Period' })
    await view.user.click(periodTrigger)
    await view.user.click(await screen.findByRole('option', { name: 'Unlimited' }))
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('button', { name: 'Confirm switch' })).toBeVisible()
    await view.user.click(screen.getByRole('button', { name: 'Confirm switch' }))

    await waitFor(() =>
      expect(quotaTransport.update).toHaveBeenCalledWith('ws_123', 'usr_a', {
        period: 'unlimited',
      }),
    )
    expect(toast.success).toHaveBeenCalledWith('Member quota updated.')
  })

  test('cancelling the unlimited confirm keeps the dialog open without transport', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    const view = renderSection(true)
    await screen.findByRole('table', { name: 'Member usage table' })
    await openAdjustDialog(view, 'Adjust a@example.com')

    const periodTrigger = screen.getByRole('combobox', { name: 'Period' })
    await view.user.click(periodTrigger)
    await view.user.click(await screen.findByRole('option', { name: 'Unlimited' }))
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('button', { name: 'Confirm switch' })).toBeVisible()
    await view.user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(quotaTransport.update).not.toHaveBeenCalled()
    expect(screen.getByText('Adjust member quota')).toBeVisible()
  })

  test('requires a member pick before transport', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(makeUnlimitedMemberQuota({ usedCredits: 40 }))
    quotaTransport.fetchList.mockResolvedValue([])
    const view = renderSection(true)
    await screen.findByText('No member quotas')
    await view.user.click(screen.getByRole('button', { name: 'Set first quota' }))
    await screen.findByText('Set member quota')

    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/Pick a member first/)
    expect(quotaTransport.update).not.toHaveBeenCalled()
  })

  test('toasts self quota load failures with a neutral retry entry', async () => {
    quotaTransport.fetchSelf
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce(makeMemberQuotaSummary())
    const view = renderSection()

    await expectToastFeedback(toast, 'Could not load the quota.')
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
  })

  test('keeps cold Settings manager reads to four while billing authority resolves asynchronously', async () => {
    let resolveAccount!: (account: unknown) => void
    billingTransport.fetchAccount.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveAccount = resolve
        }),
    )
    quotaTransport.fetchList.mockResolvedValue(managedQuotas())
    renderWithUser(WorkspaceSettingsDialog, {
      props: {
        isOpen: true,
        items: [{ publicId: 'ws_123', name: 'Team', organizationId: 'org_1', isDefault: false }],
        currentUserId: 'usr_owner',
        onRetry: vi.fn(),
        onCreate: vi.fn(),
        onRename: vi.fn(),
        onDelete: vi.fn(),
      },
      global: {
        plugins: [[VueQueryPlugin, { queryClient: new QueryClient() }]],
        stubs: {
          DialogRoot: { props: ['open'], template: '<div v-if="open"><slot /></div>' },
          DialogPortal: { template: '<slot />' },
          DialogOverlay: true,
          DialogContent: { template: '<section><slot /></section>' },
          DialogTitle: { template: '<slot />' },
          DialogDescription: { template: '<slot />' },
          WorkspaceMemberSection: true,
          WorkspaceCreateDialog: true,
        },
      },
    })
    await waitFor(() => expect(billingTransport.fetchAccount).toHaveBeenCalledTimes(1))
    expect(quotaTransport.fetchSelf).not.toHaveBeenCalled()
    expect(quotaTransport.fetchList).not.toHaveBeenCalled()
    expect(organizationTransport.fetchMembers).not.toHaveBeenCalled()
    resolveAccount({
      workspaceId: 'ws_123',
      billingAccountId: 'ba_1',
      isShared: true,
      consumptionScope: 'account',
      canManageMemberQuotas: true,
    })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Adjust a@example.com' })).toBeEnabled(),
    )
    expect(billingTransport.fetchAccount).toHaveBeenCalledTimes(1)
    expect(rosterTransport.fetchMembers).toHaveBeenCalledTimes(1)
    expect(quotaTransport.fetchList).toHaveBeenCalledTimes(1)
    expect(organizationTransport.fetchMembers).toHaveBeenCalledTimes(1)
    expect(quotaTransport.fetchSelf).not.toHaveBeenCalled()
  })
})
