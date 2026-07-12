/* eslint-disable vue/one-component-per-file -- Local stubs keep the wallet presentation test focused. */
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen, within } from '../../../test/render'
import type {
  UserCreditLedgerTransaction,
  UserCreditPosting,
  UserCreditWallet,
} from '../userCreditsResource'
import UserCreditsPane from './UserCreditsPane.vue'

const useUserCreditsMock = vi.hoisted(() => vi.fn())

vi.mock('../useUserCredits', () => ({
  useUserCredits: useUserCreditsMock,
}))

const ButtonStub = defineComponent({
  props: {
    loading: { type: Boolean, default: false },
    disabled: { type: Boolean, default: false },
  },
  emits: ['click'],
  template:
    '<button type="button" :disabled="disabled || loading" @click="$emit(\'click\')"><slot /></button>',
})

const TagStub = defineComponent({
  props: {
    value: { type: String, required: true },
  },
  template: '<span>{{ value }}</span>',
})

function makeWallet(overrides: Partial<UserCreditWallet> = {}): UserCreditWallet {
  return {
    user_id: 12,
    available_credits: 1280,
    reserved_credits: 20,
    status: 'active',
    ...overrides,
  }
}

function makeTransaction(
  commandType: UserCreditLedgerTransaction['command_type'],
  amountCredits: number,
  reason: string,
  postings: UserCreditPosting[],
  overrides: Partial<UserCreditLedgerTransaction> = {},
): UserCreditLedgerTransaction {
  return {
    sequence_id: amountCredits,
    transaction_id: `${commandType}-${reason}`,
    command_type: commandType,
    amount_credits: amountCredits,
    actor: { type: 'system', id: 'billing' },
    source: 'test',
    reason,
    business_reference: { type: 'task', id: reason },
    available_credits_after: 100,
    reserved_credits_after: 0,
    created_at: '2026-07-12T08:30:00Z',
    postings,
    ...overrides,
  }
}

function makeCreditsState(overrides: Record<string, unknown> = {}) {
  return {
    wallet: ref<UserCreditWallet | undefined>(makeWallet()),
    transactions: ref<UserCreditLedgerTransaction[]>([]),
    walletPending: ref(false),
    walletError: ref<unknown>(null),
    ledgerPending: ref(false),
    ledgerError: ref<unknown>(null),
    hasNextPage: ref(false),
    isFetchingNextPage: ref(false),
    fetchNextPage: vi.fn(),
    refresh: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  }
}

function renderPane() {
  return renderWithUser(UserCreditsPane, {
    props: { active: true },
    global: {
      stubs: {
        Button: ButtonStub,
        Tag: TagStub,
      },
    },
  })
}

function rowFor(reason: string) {
  const row = screen.getByText(reason).closest('li')
  if (!row) throw new Error(`Missing ledger row for ${reason}`)
  return within(row)
}

describe('UserCreditsPane', () => {
  beforeEach(() => {
    useUserCreditsMock.mockReset()
  })

  it('renders wallet totals and signs ledger amounts from the user wallet delta', () => {
    const transactions = [
      makeTransaction(
        'grant',
        50,
        '欢迎赠送',
        [
          { account_role: 'system_issuance', amount_credits: -50 },
          { account_role: 'user_available', amount_credits: 50 },
        ],
        { created_at: 'not-a-date' },
      ),
      makeTransaction('hold', 20, '任务 101 冻结', [
        { account_role: 'user_available', amount_credits: -20 },
        { account_role: 'user_reserved', amount_credits: 20 },
      ]),
      makeTransaction('release', 20, '任务 102 解冻', [
        { account_role: 'user_reserved', amount_credits: -20 },
        { account_role: 'user_available', amount_credits: 20 },
      ]),
      makeTransaction('capture', 20, '任务 103 生图', [
        { account_role: 'user_reserved', amount_credits: -20 },
        { account_role: 'system_revenue', amount_credits: 20 },
      ]),
      makeTransaction('correction', 4, '恢复管理员扣减', [
        { account_role: 'system_sink', amount_credits: -4 },
        { account_role: 'user_available', amount_credits: 4 },
      ]),
      makeTransaction('correction', 3, '撤销管理员赠送', [
        { account_role: 'user_available', amount_credits: -3 },
        { account_role: 'system_issuance', amount_credits: 3 },
      ]),
    ]
    useUserCreditsMock.mockReturnValue(
      makeCreditsState({ transactions: ref<UserCreditLedgerTransaction[]>(transactions) }),
    )

    renderPane()

    expect(screen.getByText('1,280')).toBeVisible()
    expect(screen.getByText('时间未知')).toBeVisible()
    expect(rowFor('欢迎赠送').getByText('+50')).toBeVisible()
    expect(rowFor('任务 101 冻结').getByText('20')).toBeVisible()
    expect(rowFor('任务 102 解冻').getByText('20')).toBeVisible()
    expect(rowFor('任务 103 生图').getByText('-20')).toBeVisible()
    expect(rowFor('恢复管理员扣减').getByText('+4')).toBeVisible()
    expect(rowFor('撤销管理员赠送').getByText('-3')).toBeVisible()
  })

  it('offers recovery for errors and delegates refresh and pagination', async () => {
    const refresh = vi.fn().mockResolvedValue(undefined)
    const fetchNextPage = vi.fn().mockResolvedValue(undefined)
    useUserCreditsMock.mockReturnValue(
      makeCreditsState({
        wallet: ref<UserCreditWallet | undefined>(undefined),
        walletError: ref(new Error('余额服务暂时不可用')),
        ledgerError: ref(new Error('流水服务暂时不可用')),
        hasNextPage: ref(true),
        refresh,
        fetchNextPage,
      }),
    )

    const { user } = renderPane()

    expect(screen.getAllByRole('alert')[0]).toHaveTextContent('点子余额加载失败')
    expect(screen.getByText('流水服务暂时不可用')).toBeVisible()

    await user.click(screen.getByRole('button', { name: '重新加载' }))
    await user.click(screen.getByRole('button', { name: '加载更多' }))

    expect(refresh).toHaveBeenCalledOnce()
    expect(fetchNextPage).toHaveBeenCalledOnce()
  })
})
