import type {
  ControlCreditCommandRequest,
  ControlCreditCorrectionRequest,
  ControlCreditRefundRequest,
} from '../../../../../api/generated-query/control/model'
import { formatIdeaAmount } from '../../../../../core/i18n/userIdeaCurrency'
import { createUniqueId } from '../../../../../core/utils/ids'

export type AdminCreditCommandKind = 'grant' | 'deduction' | 'refund'

type AdminCreditCommandDraft = {
  amountCredits: number | null
}

export type AdminCreditCommandInput = AdminCreditCommandDraft & {
  kind: AdminCreditCommandKind
  // Control user id (usr_...). The control ledger resolves it to the
  // internal wallet; raw numeric ids never cross the boundary.
  userPublicId: string
  chargeTransactionId?: string | undefined
  remainingRefundableCredits?: number | undefined
}

type AdminCreditCommandRequest =
  ControlCreditCommandRequest | ControlCreditRefundRequest | ControlCreditCorrectionRequest

export type BuiltAdminCreditCommand = {
  body: AdminCreditCommandRequest
  idempotencyKey: string
}

export type CreditCommandValidation = {
  amountError: string
  valid: boolean
}

const MAX_CREDIT_AMOUNT = 9_000_000_000_000_000

export function validateAdminCreditCommand(
  input: AdminCreditCommandInput,
): CreditCommandValidation {
  let amountError = ''
  const amount = input.amountCredits
  if (amount == null || !Number.isSafeInteger(amount) || amount < 1) {
    amountError = '请输入大于 0 的整数Idea数量'
  } else if (amount > MAX_CREDIT_AMOUNT) {
    amountError = 'Idea数量超出单次操作上限'
  } else if (
    input.kind === 'refund' &&
    (input.remainingRefundableCredits == null || amount > input.remainingRefundableCredits)
  ) {
    amountError = `退款不能超过可退余额 ${formatIdeaAmount(input.remainingRefundableCredits ?? 0)}`
  }

  if (input.kind === 'refund' && !input.chargeTransactionId?.trim()) {
    amountError = '退款必须从原 Charge 流水发起'
  }

  return { amountError, valid: !amountError }
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

export function buildAdminCreditCommandRequest(
  input: AdminCreditCommandInput,
  idFactory: () => string = () => createUniqueId(),
): BuiltAdminCreditCommand {
  const validation = validateAdminCreditCommand(input)
  if (!validation.valid || input.amountCredits == null) {
    throw new Error(validation.amountError || 'Idea操作参数无效')
  }

  const operationId = idFactory()
    .replace(/[^a-zA-Z0-9_-]/g, '')
    .slice(0, 96)
  if (!operationId) throw new Error('无法生成Idea操作标识')
  const body: AdminCreditCommandRequest = {
    user_id: input.userPublicId,
    amount_credits: input.amountCredits,
    business_reference_id: `control-ui:${input.kind}:${operationId}`,
  }

  if (input.kind === 'refund') {
    ;(body as ControlCreditRefundRequest).charge_transaction_id =
      input.chargeTransactionId as string
  }

  return {
    body,
    idempotencyKey: `control-ui:${input.kind}:${operationId}`,
  }
}
