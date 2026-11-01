import { beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '@/core/i18n'
import { renderWithUser, screen, waitFor } from '@/test/render'
import { mockAppToast, expectToastFeedback, type AppToastMocks } from '@/test/toast'
import WorkspaceLimitsSection from './WorkspaceLimitsSection.vue'
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
  entries: [
    { userId: 'usr_a', role: 'member', email: 'a@example.com' },
    { userId: 'usr_b', role: 'admin', email: 'b@example.com' },
  ],
}))

vi.mock('@/features/account/session/accountWorkspace', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('@/features/account/session/accountWorkspace')>()
  return {
    ...actual,
    workspaceMembersQueryOptions: () => ({
      queryKey: ['test-quota-roster'],
      queryFn: async () => rosterTransport.entries,
    }),
  }
})

function dailyQuota(overrides: Partial<MemberQuotaSummary> = {}): MemberQuotaSummary {
  return {
    billingAccountId: 'ba_1',
    userId: 'usr_a',
    period: 'daily',
    limitCredits: 100,
    usedCredits: 25,
    remainingCredits: 75,
    periodStart: '2026-10-21T00:00:00Z',
    periodEnd: '2026-10-22T00:00:00Z',
    ...overrides,
  }
}

function unlimitedQuota(overrides: Partial<MemberQuotaSummary> = {}): MemberQuotaSummary {
  return {
    billingAccountId: 'ba_1',
    userId: 'usr_a',
    period: 'unlimited',
    limitCredits: null,
    usedCredits: 40,
    remainingCredits: null,
    periodStart: null,
    periodEnd: null,
    ...overrides,
  }
}

function renderSection(canManage = false) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithUser(WorkspaceLimitsSection, {
    props: { workspaceId: 'ws_123', canManage, isExpanded: true },
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

async function expandManagement(view: ReturnType<typeof renderSection>) {
  await view.user.click(screen.getByRole('button', { name: 'Manage limits' }))
  expect(screen.getByRole('button', { name: 'Hide management' })).toBeVisible()
}

describe('WorkspaceLimitsSection', () => {
  let toast: AppToastMocks

  beforeEach(async () => {
    await setUserLocale('en')
    vi.clearAllMocks()
    toast = mockAppToast()
    quotaTransport.fetchSelf.mockResolvedValue(unlimitedQuota())
    quotaTransport.fetchList.mockResolvedValue([])
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
      screen.getByText(
        /Only the Personal account holder and organization owners or admins can change member quotas/,
      ),
    ).toBeVisible()
    expect(screen.queryByText(/Only the Personal account holder can set the default/)).toBeNull()
    expect(screen.queryByLabelText('Member')).toBeNull()
    expect(screen.queryByLabelText('Period')).toBeNull()
    expect(toast.success).not.toHaveBeenCalled()
  })

  test('hides the management toggle from members', async () => {
    renderSection()

    await screen.findByText('Usage limits')
    expect(screen.queryByRole('button', { name: 'Manage limits' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Hide management' })).toBeNull()
  })

  test('does not request manager data when the server denies management', async () => {
    renderSection(false)

    expect(await screen.findByText('40 Ideas used')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Manage limits' })).toBeNull()
    expect(quotaTransport.fetchList).not.toHaveBeenCalled()
  })

  test('keeps management forms collapsed by default for authorized quota managers', async () => {
    renderSection(true)

    await screen.findByText('Usage limits')
    expect(screen.getByRole('button', { name: 'Manage limits' })).toBeVisible()
    expect(screen.queryByLabelText('Member')).toBeNull()
    expect(screen.queryByLabelText('Limit (Ideas)')).toBeNull()
    expect(screen.queryByText(/Only the Personal account holder/)).toBeNull()
  })

  test('toggles management forms open and closed for authorized quota managers', async () => {
    const view = renderSection(true)

    await screen.findByText('Usage limits')
    const toggle = screen.getByRole('button', { name: 'Manage limits' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')

    await expandManagement(view)
    expect(await screen.findByRole('combobox', { name: 'Member' })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Hide management' })).toHaveAttribute(
      'aria-expanded',
      'true',
    )

    await view.user.click(screen.getByRole('button', { name: 'Hide management' }))
    expect(screen.queryByRole('combobox', { name: 'Member' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Manage limits' })).toBeVisible()
  })

  test('renders self quota usage with remaining for members', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(dailyQuota())
    renderSection()

    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
    expect(screen.getByText('75 Ideas left')).toBeVisible()
  })

  test('warns when the self cap is reached', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(
      dailyQuota({ usedCredits: 100, remainingCredits: 0 }),
    )
    renderSection()

    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent(/Personal quota exhausted/)
  })

  test('renders unlimited self usage without a cap', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(unlimitedQuota())
    renderSection()

    expect(await screen.findByText('40 Ideas used')).toBeVisible()
    expect(screen.queryByText('75 Ideas left')).toBeNull()
  })

  test('authorized quota managers see the account quota list', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(unlimitedQuota())
    quotaTransport.fetchList.mockResolvedValue([
      dailyQuota(),
      {
        ...dailyQuota(),
        userId: 'usr_b',
        period: 'fixed',
        limitCredits: 200,
        usedCredits: 200,
        remainingCredits: 0,
        periodStart: null,
        periodEnd: null,
      },
    ])
    const view = renderSection(true)
    await screen.findByText('Usage limits')
    await expandManagement(view)

    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
    expect(screen.getByText('200 / 200 Ideas used')).toBeVisible()
    expect(screen.queryByText(/Only the Personal account holder/)).toBeNull()
  })

  test('authorized quota managers can set a member quota', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(unlimitedQuota())
    quotaTransport.fetchList.mockResolvedValue([])
    quotaTransport.update.mockResolvedValue(dailyQuota({ userId: 'usr_a', limitCredits: 50 }))
    const view = renderSection(true)
    await screen.findByText('Usage limits')
    await expandManagement(view)

    const memberTrigger = await screen.findByRole('combobox', { name: 'Member' })
    await view.user.click(memberTrigger)
    await view.user.click(await screen.findByRole('option', { name: 'a@example.com' }))

    const limitInput = (await screen.findAllByLabelText('Limit (Ideas)'))[0] as HTMLInputElement
    await view.user.clear(limitInput)
    await view.user.type(limitInput, '50')
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() =>
      expect(quotaTransport.update).toHaveBeenCalledWith('ws_123', 'usr_a', {
        period: 'daily',
        limitCredits: 50,
      }),
    )
    expect(toast.success).toHaveBeenCalledWith('Member quota updated.')
  })

  test('requires a member pick before transport', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(unlimitedQuota())
    quotaTransport.fetchList.mockResolvedValue([])
    const view = renderSection(true)
    await screen.findByText('Usage limits')
    await expandManagement(view)

    await screen.findByRole('combobox', { name: 'Member' })
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/Pick a member first/)
    expect(quotaTransport.update).not.toHaveBeenCalled()
  })

  test('toasts self quota load failures with a neutral retry entry', async () => {
    quotaTransport.fetchSelf
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce(dailyQuota())
    const view = renderSection()

    await expectToastFeedback(toast, 'Could not load the quota.')
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
  })
})
