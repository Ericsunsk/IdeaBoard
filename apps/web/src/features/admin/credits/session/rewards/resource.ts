import {
  createControlRewardCode,
  deleteControlRewardCode,
  searchControlRewardCodes,
  setControlRewardCodeEnabled,
  updateControlRewardCode,
} from '@/api/generated-query/control/control-credits/control-credits'
import type {
  ControlCreatedRewardCode,
  ControlRewardCode,
  ControlRewardCodePage,
  ControlUpdatedRewardCode,
} from '@/api/generated-query/control/model'
import { requireContractData } from '@/api/contractKernel'
import { withSignal } from '@/core/utils/request'
import { controlCreditIdempotencyKey } from '@/api/idempotencyKey'
import {
  buildControlMutationHeaders,
  createControlError,
  withControlCopy,
  type ControlMutationOptions,
} from '../../../shared'

const REWARD_CODE_CONTROL_MESSAGES = {
  forbidden: '权限不足，无法操作兑换码',
  403: '权限不足，无法操作兑换码',
} as const

function requireRewardIdempotencyKey(key: string): string {
  const trimmed = key.trim()
  if (!trimmed) throw createControlError('control_idempotency_required', '缺少幂等键，请重试')
  return trimmed
}

function requireUnexpiredRewardExpiry(expiresAt: string): void {
  const trimmed = expiresAt.trim()
  if (!trimmed) return
  const parsed = new Date(trimmed)
  if (Number.isNaN(parsed.getTime())) {
    throw createControlError('control_expired_input', '过期时间格式无效')
  }
  if (parsed.getTime() <= Date.now()) {
    throw createControlError('control_expired_input', '过期时间不能早于当前时间')
  }
}
const RewardCodeStatus = {
  disabled: 'disabled',
  expired: 'expired',
  exhausted: 'exhausted',
  enabled: 'enabled',
} as const

type RewardCodeStatus = (typeof RewardCodeStatus)[keyof typeof RewardCodeStatus]

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

type CreateRewardCodeInput = {
  displayName: string
  code: string
  creditAmount: number
  maxRedemptions: number
  expiresAt: string
  idempotencyKey: string
}

type CreatedRewardCode = {
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

type UpdatedRewardCode = {
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

type RewardCodeMutationOptions = ControlMutationOptions

export type AdminRewardCodeResource = {
  search: (input: SearchRewardCodesInput) => Promise<RewardCodePage>
  create: (input: CreateRewardCodeInput) => Promise<CreatedRewardCode>
  update: (
    id: number,
    input: UpdateRewardCodeInput,
    options?: RewardCodeMutationOptions,
  ) => Promise<UpdatedRewardCode>
  setEnabled: (
    id: number,
    enabled: boolean,
    options?: RewardCodeMutationOptions,
  ) => Promise<RewardCode>
  delete: (id: number, options?: RewardCodeMutationOptions) => Promise<void>
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
  const criteria: {
    query?: string
    status?: RewardCodeStatus
    before_id?: number
    limit?: number
  } = {}
  if (input.query.trim()) criteria.query = input.query.trim()
  if (input.status != null) criteria.status = input.status
  if (input.beforeId != null) criteria.before_id = input.beforeId
  if (input.limit != null) criteria.limit = input.limit
  const body = await searchControlRewardCodes(criteria, withSignal(input.signal))
  const page = requireContractData<ControlRewardCodePage>(body, '加载兑换码失败')
  if (!Array.isArray(page.reward_codes) || typeof page.total !== 'number') {
    throw createControlError('control_failed', '加载兑换码失败')
  }
  const result: RewardCodePage = {
    rewardCodes: page.reward_codes.map(normalizeRewardCode),
    total: page.total,
  }
  if (page.next_before_id != null) result.nextBeforeId = page.next_before_id
  return result
}

async function createRewardCode(input: CreateRewardCodeInput): Promise<CreatedRewardCode> {
  const idempotencyKey = requireRewardIdempotencyKey(input.idempotencyKey)
  requireUnexpiredRewardExpiry(input.expiresAt)
  let body: unknown
  try {
    body = await createControlRewardCode(
      {
        display_name: input.displayName.trim(),
        code: input.code,
        credit_amount: input.creditAmount,
        max_redemptions: input.maxRedemptions,
        expires_at: input.expiresAt,
      },
      { headers: buildControlMutationHeaders({ idempotencyKey }) },
    )
  } catch (error) {
    throw withControlCopy(error, '创建兑换码失败', REWARD_CODE_CONTROL_MESSAGES)
  }
  const created = requireContractData<ControlCreatedRewardCode>(body, '创建兑换码失败')
  if (!created.reward_code || !created.plaintext) {
    throw createControlError('control_failed', '创建兑换码失败')
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
  options?: RewardCodeMutationOptions,
): Promise<UpdatedRewardCode> {
  requireUnexpiredRewardExpiry(input.expiresAt)
  let body: unknown
  try {
    const payload: {
      display_name: string
      credit_amount: number
      max_redemptions: number
      expires_at: string
      code?: string
    } = {
      display_name: input.displayName.trim(),
      credit_amount: input.creditAmount,
      max_redemptions: input.maxRedemptions,
      expires_at: input.expiresAt,
    }
    if (input.code) payload.code = input.code
    body = await updateControlRewardCode(id, payload, {
      headers: buildControlMutationHeaders(options, controlCreditIdempotencyKey('reward-update')),
    })
  } catch (error) {
    throw withControlCopy(error, '更新兑换码失败', REWARD_CODE_CONTROL_MESSAGES)
  }
  const updated = requireContractData<ControlUpdatedRewardCode>(body, '更新兑换码失败')
  if (!updated.reward_code) throw createControlError('control_failed', '更新兑换码失败')
  const result: UpdatedRewardCode = { rewardCode: normalizeRewardCode(updated.reward_code) }
  if (updated.plaintext != null) result.plaintext = updated.plaintext
  if (updated.replayed != null) result.replayed = updated.replayed
  return result
}

async function setRewardCodeEnabled(
  id: number,
  enabled: boolean,
  options?: RewardCodeMutationOptions,
): Promise<RewardCode> {
  let body: unknown
  try {
    body = await setControlRewardCodeEnabled(
      id,
      { enabled },
      {
        headers: buildControlMutationHeaders(
          options,
          controlCreditIdempotencyKey('reward-enabled'),
        ),
      },
    )
  } catch (error) {
    throw withControlCopy(
      error,
      enabled ? '启用兑换码失败' : '停用兑换码失败',
      REWARD_CODE_CONTROL_MESSAGES,
    )
  }
  return normalizeRewardCode(
    requireContractData<ControlRewardCode>(body, enabled ? '启用兑换码失败' : '停用兑换码失败'),
  )
}

async function deleteRewardCode(id: number, options?: RewardCodeMutationOptions): Promise<void> {
  if (options?.idempotencyKey != null) requireRewardIdempotencyKey(options?.idempotencyKey)
  try {
    const body = await deleteControlRewardCode(id, {
      headers: buildControlMutationHeaders({
        idempotencyKey: options?.idempotencyKey ?? controlCreditIdempotencyKey('reward-delete'),
      }),
    })
    // Delete resolves `{ data: <ControlDeletedRewardCode> }`; acknowledgement
    // only, no view mapping.
    requireContractData<Record<string, unknown>>(body, '删除兑换码失败')
  } catch (error) {
    throw withControlCopy(error, '删除兑换码失败', REWARD_CODE_CONTROL_MESSAGES)
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
  throw createControlError('control_failed', '未知的兑换码状态')
}

// withControlCopy is shared — see admin/shared/core/controlCopy.ts.
