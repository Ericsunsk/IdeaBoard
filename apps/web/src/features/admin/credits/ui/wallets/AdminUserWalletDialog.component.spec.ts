import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '../../../../../core/i18n'
import { loadLocaleMessages } from '../../../../../core/i18n/userLocale'
import { renderWithUser, screen, waitFor } from '../../../../../test/render'
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
      ledgerQuery: { error: { value: unknown } }
    } | null
  } => ({ current: null }),
)

vi.mock('../../session/shared/feedback', () => ({
  useAdminCreditsBillingOperationsFeedback: () => feedback,
}))

vi.mock('../../../shared/session/feedback/useAdminConfirm', () => ({
  useAdminConfirm: () => ({ confirm: vi.fn() }),
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
    ledger: vue.ref(undefined),
    ledgerDataCriteria: vue.ref(undefined),
    commandPending: vue.ref(false),
    executeCommand: vi.fn(),
  }
  shared.current = api
  return { useAdminCreditWalletResource: () => api }
})

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
    if (shared.current) {
      shared.current.walletQuery.error.value = null
      shared.current.ledgerQuery.error.value = null
    }
  })

  it('reports a wallet load failure through toast only without an inline alert', async () => {
    renderWithUser(AdminUserWalletDialog, {
      props: { visible: true, user: makeUser() },
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
      props: { visible: true, user: makeUser() },
    })

    expect(await screen.findByRole('dialog')).toBeVisible()

    shared.current!.ledgerQuery.error.value = new Error('ledger unavailable')

    await waitFor(() => expect(feedback.ledgerLoadFailed).toHaveBeenCalledTimes(1))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
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
