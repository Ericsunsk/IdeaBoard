import {
  createControlRewardCode,
  deleteControlRewardCode,
  searchControlRewardCodes,
  setControlRewardCodeEnabled,
  updateControlRewardCode,
} from '../../../../shared/api/generated-query/control/control-credits/control-credits'
import type {
  ControlCreatedRewardCode,
  ControlRewardCode,
  ControlRewardCodePage,
  ControlUpdatedRewardCode,
} from '../../../../shared/api/generated-query/control/model'
import {
  buildContractRequestHeaders,
  newRequestId,
  requireContractData,
} from '../../../../shared/api/contractKernel'
import { controlCreditIdempotencyKey } from '../../../../shared/api/idempotencyKey'
import { withControlCopy } from '../../shared/lib/controlCopy'

// Admin reward code boundary (W2 transports). Search/create/update/
// enable/delete ride the Control Plane reward-code operations
// (`/api/control/v1/rewards/codes*`) with the unified contract envelope
// (`{ data }` success / `{ error: { code, message, request_id } }`
// errors). Every mutation carries a header-only Idempotency-Key plus an
// operator reason collected through the shared confirm dialog, and emits
// a control audit record. The deleted `/api/admin/rewards/codes/*` routes
// are gone; this module is the single transport. View types and
// normalizers keep their shapes so the UI and specs keep working.
const RewardCodeStatus = {
  disabled: 'disabled',
  expired: 'expired',
  exhausted: 'exhausted',
  enabled: 'enabled',
} as const

type RewardCodeStatus = (typeof RewardCodeStatus)[keyof typeof RewardCodeStatus]

export type GeneratedRewardCode = {
  id: number
  display_name: string
  masked_code_hint: string
  credit_amount: number
  max_redemptions: number
  redemption_count: number
  expires_at?: string
  enabled: boolean
  status: RewardCodeStatus
  created_at: string
  updated_at: string
}

export type GeneratedCreatedRewardCode = {
  reward_code: GeneratedRewardCode
  plaintext: string
  replayed: boolean
}

export type GeneratedUpdatedRewardCode = {
  reward_code: GeneratedRewardCode
  plaintext?: string
  replayed?: boolean
}

export type GeneratedRewardCodePage = {
  reward_codes: GeneratedRewardCode[]
  total: number
  next_before_id?: number
}

export type RewardCode = {
  id: number
  displayName: string
  maskedCodeHint: string
  creditAmount: number
  maxRedemptions: number
  redemptionCount: number
  expiresAt?: string | undefined
  enabled: boolean
  status: RewardCodeStatus
  createdAt: string
  updatedAt: string
}

// 状态筛选哨兵（对齐用户管理页）：'all' 表示不过滤，落 query 前归一为 undefined。
export const REWARD_CODE_STATUS_FILTER_ALL = 'all' as const

export type RewardCodeStatusFilter = typeof REWARD_CODE_STATUS_FILTER_ALL | RewardCodeStatus

export type CreateRewardCodeInput = {
  displayName: string
  code: string
  creditAmount: number
  maxRedemptions: number
  expiresAt: string
  idempotencyKey: string
}

export type CreatedRewardCode = {
  rewardCode: RewardCode
  plaintext: string
  replayed: boolean
}

export type UpdateRewardCodeInput = {
  displayName: string
  creditAmount: number
  maxRedemptions: number
  expiresAt: string
  code?: string
}

export type UpdatedRewardCode = {
  rewardCode: RewardCode
  plaintext?: string | undefined
  replayed?: boolean | undefined
}

export type RewardCodePage = {
  rewardCodes: RewardCode[]
  total: number
  nextBeforeId?: number | undefined
}

export type SearchRewardCodesInput = {
  query: string
  status?: RewardCodeStatus | undefined
  beforeId?: number | undefined
  limit?: number
  signal?: AbortSignal
}

export type RewardCodeMutationOptions = {
  operatorReason: string
  idempotencyKey?: string | undefined
}

export type AdminRewardCodeResource = {
  search: (input: SearchRewardCodesInput) => Promise<RewardCodePage>
  create: (input: CreateRewardCodeInput, options: RewardCodeMutationOptions) => Promise<CreatedRewardCode>
  update: (
    id: number,
    input: UpdateRewardCodeInput,
    options: RewardCodeMutationOptions,
  ) => Promise<UpdatedRewardCode>
  setEnabled: (
    id: number,
    enabled: boolean,
    options: RewardCodeMutationOptions,
  ) => Promise<RewardCode>
  delete: (id: number, options: RewardCodeMutationOptions) => Promise<void>
}

export function createAdminRewardCodeResource(): AdminRewardCodeResource {
  return {
    search: searchRewardCodes,
    create: createRewardCode,
    update: updateRewardCode,
    setEnabled: setRewardCodeEnabled,
    delete: deleteRewardCode,
  }
}

async function searchRewardCodes(input: SearchRewardCodesInput): Promise<RewardCodePage> {
  const criteria: { query?: string; status?: RewardCodeStatus; before_id?: number; limit?: number } = {}
  if (input.query.trim()) criteria.query = input.query.trim()
  if (input.status != null) criteria.status = input.status
  if (input.beforeId != null) criteria.before_id = input.beforeId
  if (input.limit != null) criteria.limit = input.limit
  const body = await searchControlRewardCodes(criteria, {
    ...(input.signal == null ? {} : { signal: input.signal }),
  })
  const page = requireContractData<ControlRewardCodePage>(body, '加载兑换码失败')
  if (!Array.isArray(page.reward_codes) || typeof page.total !== 'number') {
    throw controlError('control_failed', '加载兑换码失败')
  }
  const result: RewardCodePage = {
    rewardCodes: page.reward_codes.map(normalizeRewardCode),
    total: page.total,
  }
  if (page.next_before_id != null) result.nextBeforeId = page.next_before_id
  return result
}

async function createRewardCode(
  input: CreateRewardCodeInput,
  options: RewardCodeMutationOptions,
): Promise<CreatedRewardCode> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  const idempotencyKey = input.idempotencyKey.trim() || controlCreditIdempotencyKey('reward-create')
  let body: unknown
  try {
    body = await createControlRewardCode(
      {
        display_name: input.displayName.trim(),
        code: input.code,
        credit_amount: input.creditAmount,
        max_redemptions: input.maxRedemptions,
        expires_at: input.expiresAt,
        reason: operatorReason,
      },
      { headers: mutationHeaders(idempotencyKey, operatorReason) },
    )
  } catch (error) {
    throw withControlCopy(error, '创建兑换码失败')
  }
  const created = requireContractData<ControlCreatedRewardCode>(body, '创建兑换码失败')
  if (!created.reward_code || !created.plaintext) {
    throw controlError('control_failed', '创建兑换码失败')
  }
  return {
    rewardCode: normalizeRewardCode(created.reward_code),
    plaintext: created.plaintext,
    replayed: created.replayed === true,
  }
}

async function updateRewardCode(
  id: number,
  input: UpdateRewardCodeInput,
  options: RewardCodeMutationOptions,
): Promise<UpdatedRewardCode> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  let body: unknown
  try {
    const payload: {
      display_name: string
      credit_amount: number
      max_redemptions: number
      expires_at: string
      code?: string
      reason: string
    } = {
      display_name: input.displayName.trim(),
      credit_amount: input.creditAmount,
      max_redemptions: input.maxRedemptions,
      expires_at: input.expiresAt,
      reason: operatorReason,
    }
    if (input.code) payload.code = input.code
    body = await updateControlRewardCode(id, payload, {
      headers: mutationHeaders(
        options.idempotencyKey ?? controlCreditIdempotencyKey('reward-update'),
        operatorReason,
      ),
    })
  } catch (error) {
    throw withControlCopy(error, '更新兑换码失败')
  }
  const updated = requireContractData<ControlUpdatedRewardCode>(body, '更新兑换码失败')
  if (!updated.reward_code) throw controlError('control_failed', '更新兑换码失败')
  const result: UpdatedRewardCode = { rewardCode: normalizeRewardCode(updated.reward_code) }
  if (updated.plaintext != null) result.plaintext = updated.plaintext
  if (updated.replayed != null) result.replayed = updated.replayed
  return result
}

async function setRewardCodeEnabled(
  id: number,
  enabled: boolean,
  options: RewardCodeMutationOptions,
): Promise<RewardCode> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  let body: unknown
  try {
    body = await setControlRewardCodeEnabled(
      id,
      { enabled, reason: operatorReason },
      {
        headers: mutationHeaders(
          options.idempotencyKey ?? controlCreditIdempotencyKey('reward-enabled'),
          operatorReason,
        ),
      },
    )
  } catch (error) {
    throw withControlCopy(error, enabled ? '启用兑换码失败' : '停用兑换码失败')
  }
  return normalizeRewardCode(
    requireContractData<ControlRewardCode>(body, enabled ? '启用兑换码失败' : '停用兑换码失败'),
  )
}

async function deleteRewardCode(id: number, options: RewardCodeMutationOptions): Promise<void> {
  const operatorReason = requireOperatorReason(options.operatorReason)
  try {
    const body = await deleteControlRewardCode(id, {
      headers: mutationHeaders(
        options.idempotencyKey ?? controlCreditIdempotencyKey('reward-delete'),
        operatorReason,
      ),
    })
    // Delete resolves `{ data: <ControlDeletedRewardCode> }`; acknowledgement
    // only, no view mapping.
    requireContractData<Record<string, unknown>>(body, '删除兑换码失败')
  } catch (error) {
    throw withControlCopy(error, '删除兑换码失败')
  }
}

function normalizeRewardCode(code: ControlRewardCode): RewardCode {
  const result: RewardCode = {
    id: code.id,
    displayName: code.display_name,
    maskedCodeHint: code.masked_code_hint,
    creditAmount: code.credit_amount,
    maxRedemptions: code.max_redemptions,
    redemptionCount: code.redemption_count,
    enabled: code.enabled,
    status: normalizeRewardCodeStatus(code.status),
    createdAt: code.created_at,
    updatedAt: code.updated_at,
  }
  if (typeof code.expires_at === 'string' && code.expires_at) {
    result.expiresAt = code.expires_at
  }
  return result
}

function normalizeRewardCodeStatus(status: ControlRewardCode['status']): RewardCodeStatus {
  if (
    status === RewardCodeStatus.disabled ||
    status === RewardCodeStatus.expired ||
    status === RewardCodeStatus.exhausted ||
    status === RewardCodeStatus.enabled
  ) {
    return status
  }
  throw controlError('control_failed', '未知的兑换码状态')
}

function mutationHeaders(idempotencyKey: string, operatorReason: string) {
  return buildContractRequestHeaders({
    requestId: newRequestId(),
    idempotencyKey,
    operatorReason,
  })
}

function requireOperatorReason(reason: string): string {
  const trimmed = reason.trim()
  if (!trimmed) throw controlError('control_reason_required', '需要填写操作原因')
  return trimmed
}

function controlError(
  code: string,
  message: string,
  requestId?: string | undefined,
): Error & { code?: string | undefined; requestId?: string | undefined } {
  const error = new Error(message) as Error & {
    code?: string | undefined
    requestId?: string | undefined
  }
  error.code = code
  if (requestId) error.requestId = requestId
  return error
}

// withControlCopy is shared — see admin/shared/lib/controlCopy.ts.
