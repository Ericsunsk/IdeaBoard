import { formatIdeaAmount } from '@/core/i18n/userIdeaCurrency'

export type AdminCreditCommandKind = 'grant' | 'deduction' | 'refund' | 'correction'

type AdminCreditCommandDraft = {
  amountCredits: number | null
}

// Exactly one target is required: userPublicId addresses the target user's
// deterministic initial personal Billing Account, billingAccountId names an
// explicit shared/organization account (#923). The two are never mixed, so a
// correction/refund can never reference another account's facts.
export type AdminCreditCommandInput = AdminCreditCommandDraft & {
  kind: AdminCreditCommandKind
  // Control user id (usr_...). The control ledger resolves it to the
  // internal wallet; raw numeric ids never cross the boundary.
  userPublicId?: string | undefined
  // Stable funds identity (ba_...) for explicit account operations.
  billingAccountId?: string | undefined
  chargeTransactionId?: string | undefined
  originalTransactionId?: string | undefined
  remainingRefundableCredits?: number | undefined
  originalAmountCredits?: number | undefined
}

export type CreditCommandValidation = {
  amountError: string
  targetError: string
  valid: boolean
}

const MAX_CREDIT_AMOUNT = 9_000_000_000_000_000

function validateCommandTarget(input: {
  userPublicId?: string | undefined
  billingAccountId?: string | undefined
}): string {
  const hasUser = (input.userPublicId ?? '').trim().length > 0
  const hasAccount = (input.billingAccountId ?? '').trim().length > 0
  if (hasUser === hasAccount) return '请选择用户或资金账户中的其一作为操作目标'
  return ''
}

export function validateAdminCreditCommand(
  input: AdminCreditCommandInput,
): CreditCommandValidation {
  const targetError = validateCommandTarget(input)
  const amountError =
    validateCommandAmount(input) || validateCommandLink(input) || validateCorrectionLink(input)
  return { amountError, targetError, valid: !amountError && !targetError }
}

function validateCommandAmount(input: AdminCreditCommandInput): string {
  const amount = input.amountCredits
  if (amount == null || !Number.isSafeInteger(amount) || amount < 1) {
    return '请输入大于 0 的整数Idea数量'
  }
  if (amount > MAX_CREDIT_AMOUNT) {
    return 'Idea数量超出单次操作上限'
  }
  if (
    input.kind === 'refund' &&
    (input.remainingRefundableCredits == null || amount > input.remainingRefundableCredits)
  ) {
    return `退款不能超过可退余额 ${formatIdeaAmount(input.remainingRefundableCredits ?? 0)}`
  }
  if (
    input.kind === 'correction' &&
    (input.originalAmountCredits == null || amount > input.originalAmountCredits)
  ) {
    return `修正不能超过原交易金额 ${formatIdeaAmount(input.originalAmountCredits ?? 0)}`
  }
  return ''
}

function validateCommandLink(input: AdminCreditCommandInput): string {
  if (input.kind === 'refund' && !input.chargeTransactionId?.trim()) {
    return '退款必须从原 Charge 流水发起'
  }
  return ''
}

function validateCorrectionLink(input: AdminCreditCommandInput): string {
  if (input.kind === 'correction' && !input.originalTransactionId?.trim()) {
    return '修正必须从原交易流水发起'
  }
  return ''
}

export function isRefundableCharge(transaction: {
  command_type: string
  charge?: { remaining_refundable_credits?: number | null } | null
}): boolean {
  return (
    transaction.command_type === 'capture' &&
    (transaction.charge?.remaining_refundable_credits ?? 0) > 0
  )
}

export function isCorrectableTransaction(transaction: {
  command_type: string
  amount_credits: number
}): boolean {
  return (
    transaction.command_type === 'grant' ||
    transaction.command_type === 'deduction' ||
    transaction.command_type === 'refund'
  )
}
