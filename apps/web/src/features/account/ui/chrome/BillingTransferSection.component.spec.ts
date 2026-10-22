import { beforeEach, describe, expect, test, vi } from 'vitest'
import { defineComponent } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import type { BillingAccountScope } from '../../session/billing-account/resource'
import type { BillingTransferInviteSummary } from '../../session/billing-transfer/resource'
import BillingTransferSection from './BillingTransferSection.vue'

vi.mock('../../../../core/composables/useAppToast')
const transport = vi.hoisted(() => ({
  account: vi.fn(),
  managers: vi.fn(),
  roster: vi.fn(),
  invites: vi.fn(),
  cancel: vi.fn(),
  selfQuota: vi.fn(),
  quotaList: vi.fn(),
}))
vi.mock('../../session/billing-account/resource', async (load) => ({
  ...(await load<typeof import('../../session/billing-account/resource')>()),
  fetchBillingAccount: transport.account,
}))
vi.mock('../../session/billing-transfer/resource', async (load) => ({
  ...(await load<typeof import('../../session/billing-transfer/resource')>()),
  fetchBillingManagers: transport.managers,
  fetchTransferInvites: transport.invites,
  cancelTransferInvite: transport.cancel,
}))
vi.mock('../../session/quota/resource', async (load) => ({
  ...(await load<typeof import('../../session/quota/resource')>()),
  fetchSelfMemberQuota: transport.selfQuota,
  fetchMemberQuotaList: transport.quotaList,
}))
vi.mock('../../session/accountWorkspace', async (load) => {
  const actual = await load<typeof import('../../session/accountWorkspace')>()
  return {
    ...actual,
    workspaceMembersQueryOptions: (id: () => string) => ({
      queryKey: ['test-workspace-members', id()],
      queryFn: () => transport.roster(id()),
    }),
  }
})
const ConfirmationStub = defineComponent({
  props: {
    isVisible: Boolean,
    title: { type: String, default: '' },
    message: { type: String, default: '' },
    confirmLabel: { type: String, default: '' },
    intent: { type: String, default: 'primary' },
    isLoading: Boolean,
  },
  emits: ['confirm', 'update:isVisible'],
  template:
    '<div v-if="isVisible" role="dialog" :aria-label="title" :data-intent="intent"><p>{{ message }}</p><button :disabled="isLoading" @click="$emit(\'confirm\')">{{ confirmLabel }}</button><button :disabled="isLoading" @click="$emit(\'update:isVisible\', false)">Dismiss</button></div>',
})
const OWNER = 'usr_owner1'
const RECIPIENT = 'usr_member2'
const ADMIN = 'usr_admin3'
const ACCOUNT = 'ba_studio1'
const WORKSPACE = 'ws_business1'
const LIVE_EXPIRY = '2999-01-01T00:00:00Z'
const LAPSED_EXPIRY = '2020-01-01T00:00:00Z'
function account(overrides: Partial<BillingAccountScope> = {}): BillingAccountScope {
  return {
    billingAccountId: ACCOUNT,
    workspaceId: WORKSPACE,
    isShared: true,
    canManageMemberQuotas: true,
    consumptionScope: 'account',
    ...overrides,
  }
}
function invite(
  overrides: Partial<BillingTransferInviteSummary> = {},
): BillingTransferInviteSummary {
  return {
    publicId: 'bti_pending1',
    billingAccountId: ACCOUNT,
    fromUserId: OWNER,
    toUserId: RECIPIENT,
    status: 'pending',
    scopeWorkspaceIds: [],
    expiresAt: LIVE_EXPIRY,
    ...overrides,
  }
}
function renderSection(currentUserId = OWNER) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const view = renderWithUser(BillingTransferSection, {
    props: { workspaceId: WORKSPACE, currentUserId },
    global: {
      plugins: [[VueQueryPlugin, { queryClient: client }]],
      stubs: { UserConfirmDialog: ConfirmationStub },
    },
  })
  return { ...view, client, invalidate }
}

async function confirmAction(user: ReturnType<typeof renderSection>['user'], label: string) {
  await user.click(screen.getByRole('button', { name: label }))
  const dialog = await screen.findByRole('dialog', { name: label })
  await user.click(within(dialog).getByRole('button', { name: label }))
}

describe('BillingTransferSection', () => {
  beforeEach(async () => {
    await setUserLocale('en')
    vi.resetAllMocks()
    mockAppToast()
    transport.account.mockResolvedValue(account())
    transport.managers.mockResolvedValue([
      { userId: OWNER, role: 'owner' },
      { userId: ADMIN, role: 'admin' },
    ])
    transport.roster.mockResolvedValue([
      { userId: OWNER, email: 'owner@example.test', role: 'owner' },
      { userId: RECIPIENT, email: 'recipient@example.test', role: 'member' },
      { userId: ADMIN, email: 'admin@example.test', role: 'admin' },
    ])
    transport.invites.mockResolvedValue([])
    transport.selfQuota.mockResolvedValue({
      billingAccountId: ACCOUNT,
      userId: RECIPIENT,
      period: 'monthly',
      limitCredits: 1000,
      usedCredits: 20,
      remainingCredits: 980,
      periodStart: null,
      periodEnd: null,
    })
    transport.quotaList.mockResolvedValue([])
  })

  test('tracks invitations with no initiation controls', async () => {
    transport.invites.mockResolvedValue([invite()])
    renderSection()
    expect(await screen.findByText('Pending')).toBeVisible()
    // Initiation moved to member rows: no recipient picker or send control here.
    expect(screen.queryByLabelText('Recipient')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Transfer' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Accept handover' })).toBeNull()
  })

  test('cancels a pending invitation behind a danger confirm', async () => {
    transport.invites.mockResolvedValue([invite()])
    transport.cancel.mockImplementation(async () => {
      transport.invites.mockResolvedValue([invite({ status: 'cancelled' })])
      return invite({ status: 'cancelled' })
    })
    const view = renderSection()
    await screen.findByRole('button', { name: 'Cancel invitation' })
    await view.user.click(screen.getByRole('button', { name: 'Cancel invitation' }))
    const dialog = await screen.findByRole('dialog', { name: 'Cancel invitation' })
    expect(dialog).toHaveAttribute('data-intent', 'danger')
    await view.user.click(within(dialog).getByRole('button', { name: 'Cancel invitation' }))
    await waitFor(() =>
      expect(transport.cancel).toHaveBeenCalledWith(WORKSPACE, 'bti_pending1', {
        idempotencyKey: expect.stringContaining('billing:transfer-cancel:'),
      }),
    )
    expect(await screen.findByText('Cancelled')).toBeVisible()
  })

  test('shows a lapsed invitation as expired with no decisions', async () => {
    transport.invites.mockResolvedValue([invite({ expiresAt: LAPSED_EXPIRY })])
    renderSection()
    expect(await screen.findByText('Expired')).toBeVisible()
    expect(screen.queryByText('Pending')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Cancel invitation' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Accept handover' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Reject invitation' })).toBeNull()
  })

  test('notifies the addressed recipient without initiator controls', async () => {
    transport.account.mockResolvedValue(account({ consumptionScope: 'member' }))
    transport.invites.mockResolvedValue([invite()])
    renderSection(RECIPIENT)
    expect(
      await screen.findByText('You have a pending handover. Respond in the member list above.'),
    ).toBeVisible()
    expect(transport.managers).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: 'Cancel invitation' })).toBeNull()
  })

  test('keeps accepted history without actions', async () => {
    transport.invites.mockResolvedValue([invite({ status: 'accepted' })])
    renderSection()
    expect(await screen.findByText('Accepted')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Cancel invitation' })).toBeNull()
  })

  test('keeps the same idempotency key for a failed cancel retry', async () => {
    transport.invites.mockResolvedValue([invite()])
    transport.cancel
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValue(invite({ status: 'cancelled' }))
    const view = renderSection()
    await screen.findByRole('button', { name: 'Cancel invitation' })
    await confirmAction(view.user, 'Cancel invitation')
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.getByRole('dialog', { name: 'Cancel invitation' })).toBeVisible()
    await view.user.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Cancel invitation' }),
    )
    await waitFor(() => expect(transport.cancel).toHaveBeenCalledTimes(2))
    expect(transport.cancel.mock.calls[0]?.[2]).toEqual(transport.cancel.mock.calls[1]?.[2])
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  test('financial-role load error blocks tracking and exposes retry', async () => {
    transport.invites.mockResolvedValue([invite()])
    transport.managers.mockRejectedValueOnce(new Error('offline'))
    const view = renderSection()
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.queryByText('Pending')).toBeNull()
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Pending')).toBeVisible()
  })
})
