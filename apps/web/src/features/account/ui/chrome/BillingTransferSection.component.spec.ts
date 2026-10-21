import { beforeEach, describe, expect, test, vi } from 'vitest'
import { defineComponent } from 'vue'
import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { setUserLocale } from '../../../../core/i18n'
import { renderWithUser, screen, waitFor, within } from '../../../../test/render'
import { mockAppToast } from '../../../../test/toast'
import type { BillingAccountScope } from '../../session/billing-account/resource'
import type { BillingTransferInviteSummary } from '../../session/billing-transfer/resource'
import BillingTransferSection from './BillingTransferSection.vue'
import WorkspaceSettingsDialog from './WorkspaceSettingsDialog.vue'

vi.mock('../../../../core/composables/useAppToast')
const transport = vi.hoisted(() => ({
  account: vi.fn(),
  managers: vi.fn(),
  roster: vi.fn(),
  invites: vi.fn(),
  initiate: vi.fn(),
  accept: vi.fn(),
  reject: vi.fn(),
  cancel: vi.fn(),
  detach: vi.fn(),
  budget: vi.fn(),
  selfQuota: vi.fn(),
  quotaList: vi.fn(),
  defaultQuota: vi.fn(),
}))
vi.mock('../../session/billing-account/resource', async (load) => ({
  ...(await load<typeof import('../../session/billing-account/resource')>()),
  fetchBillingAccount: transport.account,
}))
vi.mock('../../session/billing-transfer/resource', async (load) => ({
  ...(await load<typeof import('../../session/billing-transfer/resource')>()),
  fetchBillingManagers: transport.managers,
  fetchTransferInvites: transport.invites,
  initiateTransferInvite: transport.initiate,
  acceptTransferInvite: transport.accept,
  rejectTransferInvite: transport.reject,
  cancelTransferInvite: transport.cancel,
  detachBillingMember: transport.detach,
}))
vi.mock('../../session/budget/resource', async (load) => ({
  ...(await load<typeof import('../../session/budget/resource')>()),
  fetchWorkspaceBudget: transport.budget,
}))
vi.mock('../../session/quota/resource', async (load) => ({
  ...(await load<typeof import('../../session/quota/resource')>()),
  fetchSelfMemberQuota: transport.selfQuota,
  fetchMemberQuotaList: transport.quotaList,
  fetchDefaultMemberQuota: transport.defaultQuota,
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
    isLoading: Boolean,
  },
  emits: ['confirm', 'update:isVisible'],
  template:
    '<div v-if="isVisible" role="dialog" :aria-label="title"><p>{{ message }}</p><button :disabled="isLoading" @click="$emit(\'confirm\')">{{ confirmLabel }}</button><button :disabled="isLoading" @click="$emit(\'update:isVisible\', false)">Dismiss</button></div>',
})
const OWNER = 'usr_owner1'
const RECIPIENT = 'usr_member2'
const ADMIN = 'usr_admin3'
const ACCOUNT = 'ba_studio1'
const WORKSPACE = 'ws_business1'
function account(overrides: Partial<BillingAccountScope> = {}): BillingAccountScope {
  return {
    billingAccountId: ACCOUNT,
    workspaceId: WORKSPACE,
    isShared: true,
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
function renderSettings(currentUserId: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const slot = defineComponent({ template: '<slot />' })
  return renderWithUser(WorkspaceSettingsDialog, {
    props: {
      isOpen: true,
      currentUserId,
      selectedId: WORKSPACE,
      items: [{ publicId: WORKSPACE, name: 'Business', organizationId: null, isDefault: false }],
      onRetry: vi.fn(),
      onCreate: vi.fn(),
      onRename: vi.fn(),
      onDelete: vi.fn(),
    },
    global: {
      plugins: [[VueQueryPlugin, { queryClient: client }]],
      stubs: {
        UserConfirmDialog: ConfirmationStub,
        DialogRoot: slot,
        DialogPortal: slot,
        DialogOverlay: slot,
        DialogContent: slot,
        DialogTitle: slot,
        DialogDescription: slot,
        WorkspaceMemberSection: true,
      },
    },
  })
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
    transport.budget.mockResolvedValue({
      workspaceId: WORKSPACE,
      billingAccountId: ACCOUNT,
      period: 'monthly',
      limitCredits: 1000,
      usedCredits: 20,
      remainingCredits: 980,
      periodStart: null,
      periodEnd: null,
    })
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
    transport.defaultQuota.mockResolvedValue({
      workspaceId: WORKSPACE,
      billingAccountId: ACCOUNT,
      period: 'monthly',
      limitCredits: 1000,
    })
  })

  test('owner initiates for an existing workspace member and can cancel the pending invitation', async () => {
    const pending = invite()
    transport.initiate.mockImplementation(async () => {
      transport.invites.mockResolvedValue([pending])
      return pending
    })
    transport.cancel.mockImplementation(async () => {
      transport.invites.mockResolvedValue([invite({ status: 'cancelled' })])
      return invite({ status: 'cancelled' })
    })
    const view = renderSection()
    const recipient = await screen.findByLabelText('Recipient')
    expect(screen.getByText(`Funds account: ${ACCOUNT}`)).toBeVisible()
    expect(screen.getByText(/Default workspace is excluded/)).toBeVisible()
    expect(within(recipient).queryByRole('option', { name: 'owner@example.test' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Send handover invitation' })).toBeDisabled()
    await view.user.selectOptions(recipient, RECIPIENT)
    await view.user.click(screen.getByRole('button', { name: 'Send handover invitation' }))
    await waitFor(() =>
      expect(transport.initiate).toHaveBeenCalledWith(WORKSPACE, RECIPIENT, {
        idempotencyKey: expect.stringContaining('billing:transfer-invite:'),
      }),
    )
    expect(await screen.findByText('Pending')).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Accept handover' })).toBeNull()
    await confirmAction(view.user, 'Cancel invitation')
    await waitFor(() => expect(transport.cancel).toHaveBeenCalledOnce())
    expect(await screen.findByText('Cancelled')).toBeVisible()
  })

  test.each(['accept', 'reject'] as const)(
    'a non-manager recipient discovers and can %s the invitation',
    async (decision) => {
      transport.account.mockResolvedValue(account({ consumptionScope: 'member' }))
      transport.invites.mockResolvedValue([invite()])
      transport[decision].mockImplementation(async () => {
        const decided = invite({
          status: decision === 'accept' ? 'accepted' : 'rejected',
          scopeWorkspaceIds: decision === 'accept' ? [WORKSPACE, 'ws_business2'] : [],
        })
        transport.invites.mockResolvedValue([decided])
        return decided
      })
      const view = renderSection(RECIPIENT)
      const label = decision === 'accept' ? 'Accept handover' : 'Reject invitation'
      await screen.findByRole('button', { name: label })
      expect(transport.managers).not.toHaveBeenCalled()
      expect(screen.queryByLabelText('Recipient')).toBeNull()
      expect(screen.queryByRole('button', { name: 'Cancel invitation' })).toBeNull()
      await confirmAction(view.user, label)
      await waitFor(() =>
        expect(transport[decision]).toHaveBeenCalledWith(WORKSPACE, 'bti_pending1', {
          idempotencyKey: expect.any(String),
        }),
      )
      expect(view.invalidate).toHaveBeenCalledWith({ queryKey: ['workspace', 'billing-account'] })
      expect(view.invalidate).toHaveBeenCalledWith({ queryKey: ['workspaces'] })
    },
  )

  test('keeps the same idempotency key for a failed accept retry and retains confirmation until success', async () => {
    transport.account.mockResolvedValue(account({ consumptionScope: 'member' }))
    transport.invites.mockResolvedValue([invite()])
    transport.accept
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValue(invite({ status: 'accepted' }))
    const view = renderSection(RECIPIENT)
    await screen.findByRole('button', { name: 'Accept handover' })
    await confirmAction(view.user, 'Accept handover')
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.getByRole('dialog', { name: 'Accept handover' })).toBeVisible()
    await view.user.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Accept handover' }),
    )
    await waitFor(() => expect(transport.accept).toHaveBeenCalledTimes(2))
    expect(transport.accept.mock.calls[0]?.[2]).toEqual(transport.accept.mock.calls[1]?.[2])
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  test('hides recipient decisions for unrelated members and mismatched funds accounts', async () => {
    transport.account.mockResolvedValue(account({ consumptionScope: 'member' }))
    transport.invites.mockResolvedValue([
      invite({ toUserId: ADMIN }),
      invite({ publicId: 'bti_other', billingAccountId: 'ba_other' }),
    ])
    renderSection(RECIPIENT)
    await screen.findAllByText('Pending')
    expect(screen.queryByRole('button', { name: 'Accept handover' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Reject invitation' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Cancel invitation' })).toBeNull()
    expect(screen.queryByLabelText('Recipient')).toBeNull()
  })

  test('former owner can detach their Admin membership while the last Owner remains protected', async () => {
    transport.managers.mockResolvedValue([
      { userId: OWNER, role: 'admin' },
      { userId: RECIPIENT, role: 'owner' },
    ])
    transport.invites.mockResolvedValue([
      invite({ status: 'accepted', scopeWorkspaceIds: [WORKSPACE] }),
    ])
    transport.detach.mockImplementation(async () => {
      transport.account.mockResolvedValue(account({ consumptionScope: 'workspace' }))
      return undefined
    })
    const view = renderSection()
    await screen.findByRole('button', { name: 'Leave funds account' })
    expect(screen.queryByRole('button', { name: 'Remove financial access' })).toBeNull()
    expect(screen.queryByLabelText('Recipient')).toBeNull()
    await confirmAction(view.user, 'Leave funds account')
    await waitFor(() => expect(transport.detach).toHaveBeenCalledWith(WORKSPACE, OWNER))
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Leave funds account' })).toBeNull(),
    )
  })

  test('Owner confirms removing another financial Admin without exposing last-owner exit', async () => {
    transport.detach.mockResolvedValue(undefined)
    const view = renderSection()
    await screen.findByRole('button', { name: 'Remove financial access' })
    expect(screen.queryByRole('button', { name: 'Leave funds account' })).toBeNull()
    await view.user.click(screen.getByRole('button', { name: 'Remove financial access' }))
    expect(screen.getByRole('dialog')).toHaveTextContent('admin@example.test')
    expect(transport.detach).not.toHaveBeenCalled()
    await view.user.click(
      within(screen.getByRole('dialog')).getByRole('button', { name: 'Remove financial access' }),
    )
    await waitFor(() => expect(transport.detach).toHaveBeenCalledWith(WORKSPACE, ADMIN))
  })

  test('financial-role load error blocks management and exposes retry', async () => {
    transport.managers.mockRejectedValueOnce(new Error('offline'))
    const view = renderSection()
    expect(await screen.findByRole('alert')).toBeVisible()
    expect(screen.queryByLabelText('Recipient')).toBeNull()
    await view.user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByLabelText('Recipient')).toBeVisible()
  })

  test('in-flight initiation disables duplicate submits', async () => {
    let finish!: (value: BillingTransferInviteSummary) => void
    transport.initiate.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const view = renderSection()
    await view.user.selectOptions(await screen.findByLabelText('Recipient'), RECIPIENT)
    const submit = screen.getByRole('button', { name: 'Send handover invitation' })
    await view.user.click(submit)
    expect(submit).toBeDisabled()
    await view.user.click(submit)
    expect(transport.initiate).toHaveBeenCalledOnce()
    transport.invites.mockResolvedValue([invite()])
    finish(invite())
    expect(await screen.findByText('Pending')).toBeVisible()
  })

  test('accepting an addressed invite refreshes all Settings financial permissions for a nonmanager', async () => {
    transport.account.mockResolvedValue(account({ consumptionScope: 'member' }))
    transport.invites.mockResolvedValue([invite()])
    transport.accept.mockImplementation(async () => {
      transport.account.mockResolvedValue(account())
      transport.managers.mockResolvedValue([{ userId: RECIPIENT, role: 'owner' }])
      transport.invites.mockResolvedValue([invite({ status: 'accepted' })])
      return invite({ status: 'accepted', scopeWorkspaceIds: [WORKSPACE] })
    })
    const view = renderSettings(RECIPIENT)
    await screen.findByRole('button', { name: 'Accept handover' })
    expect(screen.queryByLabelText('Monthly limit (Ideas)')).toBeNull()
    expect(screen.queryByLabelText('Member')).toBeNull()
    expect(screen.queryByRole('button', { name: '200 / month' })).toBeNull()
    expect(transport.managers).not.toHaveBeenCalled()
    await confirmAction(view.user, 'Accept handover')
    expect(await screen.findByLabelText('Recipient')).toBeVisible()
    expect(await screen.findByLabelText('Monthly limit (Ideas)')).toBeVisible()
    expect(await screen.findByLabelText('Member')).toBeVisible()
    expect(await screen.findByRole('button', { name: '200 / month' })).toBeVisible()
  })

  test('leaving financial membership removes management from all Settings readers', async () => {
    transport.managers.mockResolvedValue([
      { userId: OWNER, role: 'admin' },
      { userId: RECIPIENT, role: 'owner' },
    ])
    transport.detach.mockImplementation(async () => {
      transport.account.mockResolvedValue(account({ consumptionScope: 'workspace' }))
    })
    const view = renderSettings(OWNER)
    await screen.findByRole('button', { name: 'Leave funds account' })
    expect(await screen.findByLabelText('Monthly limit (Ideas)')).toBeVisible()
    expect(await screen.findByLabelText('Member')).toBeVisible()
    expect(await screen.findByRole('button', { name: '200 / month' })).toBeVisible()
    await confirmAction(view.user, 'Leave funds account')
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Leave funds account' })).toBeNull()
      expect(screen.queryByLabelText('Monthly limit (Ideas)')).toBeNull()
      expect(screen.queryByLabelText('Member')).toBeNull()
      expect(screen.queryByRole('button', { name: '200 / month' })).toBeNull()
    })
    expect(screen.getByText(/Only billing managers can set the default/)).toBeVisible()
  })
})
