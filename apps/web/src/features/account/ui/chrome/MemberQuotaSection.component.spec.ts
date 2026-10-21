import { beforeEach, describe, expect, test, vi } from 'vitest'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor } from '../../../../test/render'
import { mockAppToast, type AppToastMocks } from '../../../../test/toast'
import MemberQuotaSection from './MemberQuotaSection.vue'
import type { MemberQuotaSummary } from '../../session/quota/resource'

vi.mock('../../../../core/composables/useAppToast')

const quotaTransport = vi.hoisted(() => ({
  fetchSelf: vi.fn(),
  fetchList: vi.fn(),
  update: vi.fn(),
}))

vi.mock('../../session/quota/resource', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../session/quota/resource')>()
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

vi.mock('../../session/accountWorkspace', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../session/accountWorkspace')>()
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

function renderSection(consumptionScope: string | null = 'member') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return renderWithUser(MemberQuotaSection, {
    props: { workspaceId: 'ws_123', consumptionScope, isExpanded: true },
    global: {
      plugins: [[VueQueryPlugin, { queryClient }]],
    },
  })
}

describe('MemberQuotaSection', () => {
  let toast: AppToastMocks

  beforeEach(async () => {
    await setUserLocale('en')
    vi.clearAllMocks()
    toast = mockAppToast()
  })

  test('renders self usage with remaining for members', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(dailyQuota())
    renderSection('member')

    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
    expect(screen.getByText('75 Ideas left')).toBeVisible()
    expect(screen.getByText('Member quota')).toBeVisible()
    // Members see the read-only note instead of management controls.
    expect(screen.getByText(/Only billing managers can change member quotas/)).toBeVisible()
    expect(screen.queryByLabelText('Member')).toBeNull()
    expect(toast.success).not.toHaveBeenCalled()
  })

  test('warns when the self cap is reached', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(
      dailyQuota({ usedCredits: 100, remainingCredits: 0 }),
    )
    renderSection('member')

    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent(/Personal quota exhausted/)
  })

  test('renders unlimited self usage without a cap', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(unlimitedQuota())
    renderSection('member')

    expect(await screen.findByText('40 Ideas used')).toBeVisible()
    expect(screen.queryByText(/Ideas left/)).toBeNull()
  })

  test('billing managers see the account quota list', async () => {
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
    renderSection('account')

    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
    expect(screen.getByText('200 / 200 Ideas used')).toBeVisible()
    expect(screen.queryByText(/Only billing managers/)).toBeNull()
  })

  test('billing managers can set a member quota', async () => {
    quotaTransport.fetchSelf.mockResolvedValue(unlimitedQuota())
    quotaTransport.fetchList.mockResolvedValue([])
    quotaTransport.update.mockResolvedValue(dailyQuota({ userId: 'usr_a', limitCredits: 50 }))
    const view = renderSection('account')

    const memberTrigger = await screen.findByRole('combobox', { name: 'Member' })
    await view.user.click(memberTrigger)
    await view.user.click(await screen.findByRole('option', { name: 'a@example.com' }))

    const limitInput = (await screen.findByLabelText('Limit (Ideas)')) as HTMLInputElement
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
    const view = renderSection('account')

    await screen.findByRole('combobox', { name: 'Member' })
    await view.user.click(screen.getByRole('button', { name: 'Save' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(/Pick a member first/)
    expect(quotaTransport.update).not.toHaveBeenCalled()
  })

  test('retries self after a load failure', async () => {
    quotaTransport.fetchSelf
      .mockRejectedValueOnce(new Error('down'))
      .mockResolvedValueOnce(dailyQuota())
    const view = renderSection('member')

    expect(await screen.findByText('Could not load the quota.')).toBeVisible()
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('25 / 100 Ideas used')).toBeVisible()
  })
})
