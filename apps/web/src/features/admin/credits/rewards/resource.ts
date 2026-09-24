import { api } from '../../../../shared/api/client'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'
import { shanghaiDateTimeInputToISO } from './rewardCodeTime'

// Admin reward code boundary (issue #809). The generated `admin-rewards`
// client is gone but the backend admin plane still serves
// `/api/admin/rewards/codes/*` with the shared envelope, so this boundary
// transports through the central `api.request` with local view types.
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

export type AdminRewardCodeResource = {
  search: (input: SearchRewardCodesInput) => Promise<RewardCodePage>
  create: (input: CreateRewardCodeInput) => Promise<CreatedRewardCode>
  update: (id: number, input: UpdateRewardCodeInput) => Promise<UpdatedRewardCode>
  setEnabled: (id: number, enabled: boolean) => Promise<RewardCode>
  delete: (id: number) => Promise<void>
}

export function createAdminRewardCodeResource(): AdminRewardCodeResource {
  return {
    search: async ({ query, status, beforeId, limit = 20, signal }) =>
      toRewardCodePage(
        await resolveApiData(
          api.request<ApiResponseWithData<GeneratedRewardCodePage>>(
            '/admin/rewards/codes/search',
            {
              method: 'POST',
              body: {
                ...(query.trim() ? { query: query.trim() } : {}),
                ...(status ? { status } : {}),
                ...(beforeId !== undefined ? { before_id: beforeId } : {}),
                limit,
              } as unknown as Record<string, unknown>,
              ...(signal == null ? {} : { signal }),
            },
          ),
          '加载兑换码失败',
        ),
      ),
    create: async (input) => {
      const expiresAt = shanghaiDateTimeInputToISO(input.expiresAt)
      return toCreatedRewardCode(
        await resolveApiData(
          api.request<ApiResponseWithData<GeneratedCreatedRewardCode>>(
            '/admin/rewards/codes',
            {
              method: 'POST',
              body: {
                display_name: input.displayName.trim(),
                code: input.code.trim(),
                credit_amount: input.creditAmount,
                max_redemptions: input.maxRedemptions,
                idempotency_key: input.idempotencyKey,
                ...(expiresAt ? { expires_at: expiresAt } : {}),
              } as unknown as Record<string, unknown>,
            },
          ),
          '创建兑换码失败',
        ),
      )
    },
    update: async (id, input) =>
      toUpdatedRewardCode(
        await resolveApiData(
          api.request<ApiResponseWithData<GeneratedUpdatedRewardCode>>(
            `/admin/rewards/codes/${encodeURIComponent(String(id))}`,
            {
              method: 'PATCH',
              body: {
                display_name: input.displayName.trim(),
                credit_amount: input.creditAmount,
                max_redemptions: input.maxRedemptions,
                expires_at: shanghaiDateTimeInputToISO(input.expiresAt) ?? null,
                ...(input.code?.trim() ? { code: input.code.trim() } : {}),
              } as unknown as Record<string, unknown>,
            },
          ),
          '更新兑换码失败',
        ),
      ),
    setEnabled: async (id, enabled) =>
      toRewardCode(
        await resolveApiData(
          api.request<ApiResponseWithData<GeneratedRewardCode>>(
            `/admin/rewards/codes/${encodeURIComponent(String(id))}/enabled`,
            {
              method: 'PATCH',
              body: { enabled } as unknown as Record<string, unknown>,
            },
          ),
          enabled ? '启用兑换码失败' : '停用兑换码失败',
        ),
      ),
    delete: async (id) => {
      await resolveApiData(
        api.request<ApiResponseWithData<unknown>>(
          `/admin/rewards/codes/${encodeURIComponent(String(id))}`,
          { method: 'DELETE' },
        ),
        '删除兑换码失败',
      )
    },
  }
}

function toRewardCode(value: GeneratedRewardCode): RewardCode {
  return {
    id: value.id,
    displayName: value.display_name,
    maskedCodeHint: value.masked_code_hint,
    creditAmount: value.credit_amount,
    maxRedemptions: value.max_redemptions,
    redemptionCount: value.redemption_count,
    expiresAt: value.expires_at,
    enabled: value.enabled,
    status: value.status,
    createdAt: value.created_at,
    updatedAt: value.updated_at,
  }
}

function toCreatedRewardCode(value: GeneratedCreatedRewardCode): CreatedRewardCode {
  return {
    rewardCode: toRewardCode(value.reward_code),
    plaintext: value.plaintext,
    replayed: value.replayed,
  }
}

function toUpdatedRewardCode(value: GeneratedUpdatedRewardCode): UpdatedRewardCode {
  return {
    rewardCode: toRewardCode(value.reward_code),
    plaintext: value.plaintext,
    replayed: value.replayed,
  }
}

function toRewardCodePage(value: GeneratedRewardCodePage): RewardCodePage {
  return {
    rewardCodes: value.reward_codes.map(toRewardCode),
    total: value.total,
    nextBeforeId: value.next_before_id,
  }
}
