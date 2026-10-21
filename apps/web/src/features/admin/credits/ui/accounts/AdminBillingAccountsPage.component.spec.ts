import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { computed, ref } from 'vue'
import { i18n } from '../../../../../core/i18n'
import { loadLocaleMessages } from '../../../../../core/i18n/userLocale'
import { renderWithUser, screen, within } from '../../../../../test/render'
import AdminBillingAccountsPage from './AdminBillingAccountsPage.vue'

vi.mock('../../session/accounts/resource', () => {
  const accounts = ref([
    {
      billing_account_id: 'ba_orgpool00000000000000000000000001',
      owner_type: 'organization',
      organization_id: 'org_00000000000000000000000000000001',
      created_at: '2026-09-23T00:00:00Z',
      updated_at: '2026-09-23T00:00:00Z',
    },
  ])
  return {
    useAdminBillingAccountListResource: () => ({
      query: {
        isLoading: ref(false),
        isFetching: ref(false),
        error: ref(null),
        refetch: vi.fn(),
      },
      accounts: computed(() => accounts.value),
      nextCursor: computed(() => undefined),
      hasMore: computed(() => false),
      dataCriteria: computed(() => ({ ownerScope: 'all' as const })),
    }),
    useAdminBillingAccountDetailResource: () => ({
      query: {
        isLoading: ref(false),
        isFetching: ref(false),
        error: ref(null),
        refetch: vi.fn(),
      },
      detail: ref(null),
    }),
  }
})

vi.mock('../../session/wallets/resource', async () => {
  const vue = await import('vue')
  return {
    useAdminWalletResource: () => ({
      wallet: vue.ref({
        billing_account_id: 'ba_orgpool00000000000000000000000001',
        owner_type: 'organization',
        status: 'active',
        available_credits: 5000,
        reserved_credits: 0,
        active_restrictions: [],
      }),
      walletQuery: {
        isLoading: vue.ref(false),
        isFetching: vue.ref(false),
        error: vue.ref(null),
        refetch: vi.fn(),
      },
      ledgerQuery: {
        isLoading: vue.ref(false),
        isFetching: vue.ref(false),
        error: vue.ref(null),
        dataUpdatedAt: vue.ref(0),
        refetch: vi.fn(),
      },
      ledger: vue.ref(undefined),
      ledgerDataCriteria: vue.ref(undefined),
      isCommandPending: vue.ref(false),
      isRestrictionPending: vue.ref(false),
      executeCommand: vi.fn(),
      restrictMutation: { mutateAsync: vi.fn(), isPending: vue.ref(false) },
      releaseMutation: { mutateAsync: vi.fn(), isPending: vue.ref(false) },
    }),
  }
})

describe('AdminBillingAccountsPage', () => {
  beforeAll(async () => {
    await loadLocaleMessages('zh-CN')
    ;(i18n.global.locale as { value: string }).value = 'zh-CN'
  })

  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the platform-admin note and the explicit account rows', async () => {
    renderWithUser(AdminBillingAccountsPage)

    expect(await screen.findByText('资金账户')).toBeVisible()
    expect(screen.getByText(/平台 Admin 以平台运维身份执行操作/, { exact: false })).toBeVisible()
    expect(screen.getByText('ba_orgpool00000000000000000000000001')).toBeVisible()
    expect(screen.getAllByText('组织')[0]).toBeVisible()
  })

  it('opens the account wallet dialog without user linkage', async () => {
    const { user } = renderWithUser(AdminBillingAccountsPage)

    await user.click(
      await screen.findByRole('button', { name: '查看 ba_orgpool00000000000000000000000001' }),
    )
    const dialog = await screen.findByRole('dialog')
    expect(dialog).toBeVisible()
    const titles = within(dialog).getAllByText('ba_orgpool00000000000000000000000001 的资金账户')
    expect(titles.length).toBeGreaterThan(0)
  })
})
