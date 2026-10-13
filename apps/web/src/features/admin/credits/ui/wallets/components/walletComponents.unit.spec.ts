import { describe, expect, it, vi } from 'vitest'
import { renderWithUser, screen } from '../../../../../../test/render'
import WalletOverviewCard from './WalletOverviewCard.vue'
import WalletAdjustPanel from './WalletAdjustPanel.vue'
import WalletLedgerTable from './WalletLedgerTable.vue'
import type { AdminCreditLedgerTransaction } from '../../../session/wallets/resource'

describe('Wallet Subcomponents', () => {
  it('renders WalletOverviewCard and triggers command', async () => {
    const onOpenCommand = vi.fn()
    const { user } = renderWithUser(WalletOverviewCard, {
      props: {
        availableCredits: 500,
        reservedCredits: 20,
        accountStatus: '正常',
        accountStatusVariant: 'success',
        activeRestrictions: [],
        isUserDisabledOrClosing: false,
        commandPending: false,
        hasWallet: true,
        onOpenCommand,
      },
    })

    expect(screen.getByText(/500/)).toBeVisible()
    expect(screen.getByText(/20/)).toBeVisible()
    const grantBtn = screen.getByRole('button', { name: /Grant|赠送/ })
    await user.click(grantBtn)
    expect(onOpenCommand).toHaveBeenCalledWith('grant')
  })

  it('renders WalletAdjustPanel with inputs and emits cancel', async () => {
    const onCancel = vi.fn()
    const onSubmit = vi.fn()
    const { user } = renderWithUser(WalletAdjustPanel, {
      props: {
        activeTab: 'grant',
        refundSource: null,
        amountCredits: 100,

        showErrors: false,
        validation: { amountError: '', valid: true },
        commandPending: false,
        onCancel,
        onSubmit,
      },
    })

    expect(screen.getByText(/Grant Idea Credits|赠送 Idea 额度/)).toBeVisible()
    const cancelBtn = screen.getByRole('button', { name: /Cancel|取消/ })
    await user.click(cancelBtn)
    expect(onCancel).toHaveBeenCalled()
  })

  it.each([
    { tab: 'grant', dot: 'bg-success' },
    { tab: 'deduction', dot: 'bg-destructive' },
    { tab: 'refund', dot: 'bg-success' },
  ] as const)('pins WalletAdjustPanel $tab dot to Pill semantic $dot', ({ tab, dot }) => {
    // Tab 圆点对齐 Pill 语义色：grant/refund→success（与 getCreditCommandVariant
    // 一致），deduction→destructive；refund 不再用 warning。
    const { unmount } = renderWithUser(WalletAdjustPanel, {
      props: {
        activeTab: tab,
        refundSource: null,
        amountCredits: null,

        showErrors: false,
        validation: { amountError: '', valid: true },
        commandPending: false,
      },
    })

    const dotEl = document.querySelector('span.size-2.rounded-pill')!
    expect(dotEl).toHaveClass(dot)
    if (tab === 'refund') {
      expect(dotEl).not.toHaveClass('bg-warning')
    }
    unmount()
  })

  it('renders WalletLedgerTable and emits openRefund', async () => {
    const onOpenRefund = vi.fn()
    const mockTransaction: AdminCreditLedgerTransaction = {
      sequence_id: 1,
      transaction_id: 'tx-1',
      command_type: 'capture',
      amount_credits: 50,
      available_credits_after: 450,
      reason: 'AI Task deduction',
      reserved_credits_after: 0,
      business_reference: { type: 'task', id: 'task-1' },
      created_at: '2026-09-20T12:00:00Z',

      charge: {
        amount_credits: 50,
        refunded_credits: 0,
        remaining_refundable_credits: 50,
      },
    }

    const { user } = renderWithUser(WalletLedgerTable, {
      props: {
        transactions: [mockTransaction],
        ledgerLoading: false,
        commandPending: false,
        hasNextPage: true,
        onOpenRefund,
      },
    })

    expect(screen.getByText('AI Task deduction')).toBeVisible()
    const refundBtn = screen.getByRole('button', { name: /Refund Idea for tx-1|退还 tx-1 的Idea/ })
    await user.click(refundBtn)
    expect(onOpenRefund).toHaveBeenCalledWith(mockTransaction)
  })
})
