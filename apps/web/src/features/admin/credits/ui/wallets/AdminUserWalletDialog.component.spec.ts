import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/core/i18n'
import { loadLocaleMessages } from '@/core/i18n/userLocale'
import { renderWithUser, screen, waitFor } from '@/test/render'
import type { Ref } from 'vue'
import type {
  AdminCreditLedgerTransaction,
  AdminWalletTarget,
} from '../../session/wallets/resource'
import AdminUserWalletDialog from './AdminUserWalletDialog.vue'

const feedback = vi.hoisted(() => ({
  walletLoadFailed: vi.fn(),
  ledgerLoadFailed: vi.fn(),
  commandSucceeded: vi.fn(),
  commandFailed: vi.fn(),
}))

const shared = vi.hoisted(
  (): {
    current: {
      walletQuery: { error: { value: unknown } }
      ledgerQuery: {
        error: { value: unknown }
        isFetching: Ref<boolean>
        dataUpdatedAt: Ref<number>
      }
      ledger: Ref<{ transactions: AdminCreditLedgerTransaction[]; nextCursor?: string } | undefined>
      ledgerDataCriteria: Ref<{ target: AdminWalletTarget | null; cursor?: string } | undefined>
      executeCommand: ReturnType<typeof vi.fn>
    } | null
  } => ({ current: null }),
)

vi.mock('../../session/shared/feedback', () => ({
  useAdminCreditsBillingOperationsFeedback: () => feedback,
}))

vi.mock('../../session/accounts/resource', async () => {
  const vue = await import('vue')
  return {
    useAdminBillingAccountDetailResource: () => ({
      query: {
        isLoading: vue.ref(false),
        isFetching: vue.ref(false),
        error: vue.ref(null),
        refetch: vi.fn(),
      },
      detail: vue.ref(null),
    }),
  }
})

const confirmMock = vi.hoisted(() => vi.fn())
vi.mock('../../../shared/session/feedback/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: confirmMock }),
}))

vi.mock('../../session/wallets/resource', async () => {
  const vue = await import('vue')
  const wallet = vue.ref(null)
  const walletError = vue.ref<unknown>(null)
  const ledgerError = vue.ref<unknown>(null)
  const api = {
    wallet,
    walletQuery: {
      isLoading: vue.ref(false),
      isFetching: vue.ref(false),
      error: walletError,
      refetch: vi.fn(),
    },
    ledgerQuery: {
      isLoading: vue.ref(false),
      isFetching: vue.ref(false),
      error: ledgerError,
      dataUpdatedAt: vue.ref(0),
      refetch: vi.fn(),
    },
    ledger: vue.ref<{ transactions: AdminCreditLedgerTransaction[]; nextCursor?: string }>(),
    ledgerDataCriteria: vue.ref<{ target: AdminWalletTarget | null; cursor?: string }>(),
    isCommandPending: vue.ref(false),
    isRestrictionPending: vue.ref(false),
    executeCommand: vi.fn(),
    restrictMutation: { mutateAsync: vi.fn(), isPending: vue.ref(false) },
    releaseMutation: { mutateAsync: vi.fn(), isPending: vue.ref(false) },
  }
  shared.current = api
  return {
    useAdminWalletResource: () => api,
  }
})

function renderDialog(id = makeUser().publicId) {
  return renderWithUser(AdminUserWalletDialog, {
    props: { isVisible: true, target: { ...makeUser(), kind: 'user', id } },
  })
}

describe('AdminUserWalletDialog load failures', () => {
  beforeAll(async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
  })

  afterAll(() => {
    ;(i18n.global.locale as { value: string }).value = 'en'
  })

  beforeEach(() => {
    vi.clearAllMocks()
    confirmMock.mockResolvedValue(true)
    if (shared.current) {
      shared.current.executeCommand.mockResolvedValue({ wasReplayed: false })
      shared.current.ledger.value = undefined
      shared.current.ledgerDataCriteria.value = undefined
      shared.current.walletQuery.error.value = null
      shared.current.ledgerQuery.error.value = null
    }
  })

  it('reports a wallet load failure through toast only without an inline alert', async () => {
    renderWithUser(AdminUserWalletDialog, {
      props: {
        isVisible: true,
        target: { ...makeUser(), kind: 'user', id: makeUser().publicId },
      },
    })

    expect(await screen.findByRole('dialog')).toBeVisible()
    // Overview card stays mounted with the caller-provided balance fallback.
    expect(screen.getByText('500 Ideas')).toBeVisible()

    shared.current!.walletQuery.error.value = new Error('wallet unavailable')

    await waitFor(() => expect(feedback.walletLoadFailed).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByText('500 Ideas')).toBeVisible()
  })

  it('reports a ledger load failure through toast only without an inline alert', async () => {
    renderWithUser(AdminUserWalletDialog, {
      props: {
        isVisible: true,
        target: { ...makeUser(), kind: 'user', id: makeUser().publicId },
      },
    })

    expect(await screen.findByRole('dialog')).toBeVisible()

    shared.current!.ledgerQuery.error.value = new Error('ledger unavailable')

    await waitFor(() => expect(feedback.ledgerLoadFailed).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('uses the reviewed command snapshot even if the amount changes while confirming', async () => {
    let resolveConfirmation!: (confirmed: boolean) => void
    confirmMock.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveConfirmation = resolve
      }),
    )
    const { user } = renderDialog('usr_one')
    await user.click(await screen.findByRole('button', { name: '赠送额度' }))
    const amount = screen.getByRole('spinbutton', { name: 'Idea数量' })
    await user.type(amount, '25')
    await user.click(screen.getByRole('button', { name: '核对操作' }))
    await waitFor(() => expect(confirmMock).toHaveBeenCalledTimes(1))
    await user.clear(amount)
    await user.type(amount, '99')
    resolveConfirmation(true)
    await waitFor(() =>
      expect(shared.current!.executeCommand).toHaveBeenCalledWith(
        expect.objectContaining({ kind: 'grant', userPublicId: 'usr_one', amountCredits: 25 }),
      ),
    )
  })

  it('cancels pending confirmation if the target changes', async () => {
    let resolveConfirmation!: (confirmed: boolean) => void
    confirmMock.mockReturnValue(
      new Promise<boolean>((resolve) => {
        resolveConfirmation = resolve
      }),
    )
    const { user, rerender } = renderDialog('usr_one')
    await user.click(await screen.findByRole('button', { name: '赠送额度' }))
    await user.type(screen.getByRole('spinbutton', { name: 'Idea数量' }), '25')
    await user.click(screen.getByRole('button', { name: '核对操作' }))
    await waitFor(() => expect(confirmMock).toHaveBeenCalledTimes(1))
    await rerender({ target: { kind: 'billingAccount', id: 'ba_two', displayName: 'Shared' } })
    resolveConfirmation(true)
    await waitFor(() => expect(screen.queryByRole('spinbutton')).not.toBeInTheDocument())
    expect(shared.current!.executeCommand).not.toHaveBeenCalled()
  })

  it('never appends an old target page and deduplicates rows across cursors', async () => {
    const { user, rerender } = renderDialog('usr_one')
    await screen.findByRole('dialog')
    const current = shared.current!
    const transaction = (id: string, reason: string): AdminCreditLedgerTransaction => ({
      transaction_id: id,
      sequence_id: 1,
      command_type: 'capture',
      amount_credits: 1,
      created_at: '2026-10-21T00:00:00Z',
      reason,
    })
    current.ledgerDataCriteria.value = { target: { kind: 'user', id: 'usr_one' } }
    current.ledger.value = {
      transactions: [transaction('txn_one', 'First target')],
      nextCursor: 'cursor_two',
    }
    await screen.findByText('First target')
    await user.click(screen.getByRole('button', { name: '加载更多 Ledger' }))
    current.ledgerDataCriteria.value = {
      target: { kind: 'user', id: 'usr_one' },
      cursor: 'cursor_two',
    }
    current.ledger.value = {
      transactions: [transaction('txn_one', 'First target'), transaction('txn_two', 'Second page')],
    }
    await screen.findByText('Second page')
    expect(screen.getAllByText('First target')).toHaveLength(1)
    await rerender({ target: { kind: 'billingAccount', id: 'ba_two', displayName: 'Shared' } })
    current.ledgerQuery.dataUpdatedAt.value++
    await waitFor(() => expect(screen.queryByText('Second page')).not.toBeInTheDocument())
    current.ledgerDataCriteria.value = { target: { kind: 'billingAccount', id: 'ba_two' } }
    current.ledger.value = { transactions: [transaction('txn_three', 'Shared target')] }
    await screen.findByText('Shared target')
    expect(screen.queryByText('First target')).not.toBeInTheDocument()
  })
})

function makeUser() {
  return {
    publicId: '00000000-0000-0000-0000-000000000001',
    displayName: '测试用户',
    status: 1,
    availableCredits: 500,
  }
}
