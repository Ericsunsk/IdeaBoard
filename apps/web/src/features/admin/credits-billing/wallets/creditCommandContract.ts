import type {
  AdminCreditDeductionRequest,
  AdminCreditGrantRequest,
  AdminCreditRefundRequest,
} from '../../../../shared/api/generated-query/model'
import { formatIdeaAmount } from '../../../../shared/utils/ideaCurrency'

export type AdminCreditCommandKind = 'grant' | 'deduction' | 'refund'

export type AdminCreditCommandDraft = {
  amountCredits: number | null
  reason: string
}

export type AdminCreditCommandInput = AdminCreditCommandDraft & {
  kind: AdminCreditCommandKind
  userPublicId: string
  chargeTransactionId?: string
  remainingRefundableCredits?: number
}

export type AdminCreditCommandRequest =
  AdminCreditGrantRequest | AdminCreditDeductionRequest | AdminCreditRefundRequest

export type CreditCommandValidation = {
  amountError: string
  reasonError: string
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

  const reason = input.reason.trim()
  const reasonError = reason ? '' : '请填写操作原因'
  const hasRefundSource = input.kind !== 'refund' || Boolean(input.chargeTransactionId?.trim())

  return {
    amountError,
    reasonError: hasRefundSource ? reasonError : '退款必须从原 Charge 流水发起',
    valid: !amountError && !reasonError && hasRefundSource,
  }
}

export function buildAdminCreditCommandRequest(
  input: AdminCreditCommandInput,
  idFactory: () => string = () => crypto.randomUUID(),
): AdminCreditCommandRequest {
  const validation = validateAdminCreditCommand(input)
  if (!validation.valid || input.amountCredits == null) {
    throw new Error(validation.amountError || validation.reasonError || 'Idea操作参数无效')
  }

  const operationId = idFactory()
    .replace(/[^a-zA-Z0-9_-]/g, '')
    .slice(0, 96)
  if (!operationId) throw new Error('无法生成Idea操作标识')
  const common = {
    user_public_id: input.userPublicId,
    amount_credits: input.amountCredits,
    reason: input.reason.trim(),
    idempotency_key: `admin-ui:${input.kind}:${operationId}`,
    business_reference_id: `admin-ui:${input.kind}:${operationId}`,
  }

  if (input.kind === 'refund') {
    return {
      ...common,
      charge_transaction_id: input.chargeTransactionId as string,
    }
  }
  return common
}
