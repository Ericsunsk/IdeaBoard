import {
  createAdminRewardCode,
  deleteAdminRewardCode,
  searchAdminRewardCodes,
  setAdminRewardCodeEnabled,
  updateAdminRewardCode,
} from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type {
  CreatedRewardCode as GeneratedCreatedRewardCode,
  RewardCode as GeneratedRewardCode,
  RewardCodePage as GeneratedRewardCodePage,
  UpdatedRewardCode as GeneratedUpdatedRewardCode,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'
import { shanghaiDateTimeInputToISO } from './rewardCodeTime'

export type RewardCode = {
  id: number
  displayName: string
  maskedCodeHint: string
  creditAmount: number
  maxRedemptions: number
  redemptionCount: number
  expiresAt?: string | undefined
  enabled: boolean
  status: GeneratedRewardCode['status']
  createdAt: string
  updatedAt: string
}

export type RewardCodeStatus = RewardCode['status']

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
          searchAdminRewardCodes(
            {
              ...(query.trim() ? { query: query.trim() } : {}),
              ...(status ? { status } : {}),
              ...(beforeId !== undefined ? { before_id: beforeId } : {}),
              limit,
            },
            signal ? { signal } : undefined,
          ),
          '加载兑换码失败',
        ),
      ),
    create: async (input) => {
      const expiresAt = shanghaiDateTimeInputToISO(input.expiresAt)
      return toCreatedRewardCode(
        await resolveApiData(
          createAdminRewardCode({
            display_name: input.displayName.trim(),
            code: input.code.trim(),
            credit_amount: input.creditAmount,
            max_redemptions: input.maxRedemptions,
            idempotency_key: input.idempotencyKey,
            ...(expiresAt ? { expires_at: expiresAt } : {}),
          }),
          '创建兑换码失败',
        ),
      )
    },
    update: async (id, input) =>
      toUpdatedRewardCode(
        await resolveApiData(
          updateAdminRewardCode(id, {
            display_name: input.displayName.trim(),
            credit_amount: input.creditAmount,
            max_redemptions: input.maxRedemptions,
            expires_at: shanghaiDateTimeInputToISO(input.expiresAt) ?? null,
            ...(input.code?.trim() ? { code: input.code.trim() } : {}),
          }),
          '更新兑换码失败',
        ),
      ),
    setEnabled: async (id, enabled) =>
      toRewardCode(
        await resolveApiData(
          setAdminRewardCodeEnabled(id, { enabled }),
          enabled ? '启用兑换码失败' : '停用兑换码失败',
        ),
      ),
    delete: async (id) => {
      await resolveApiData(deleteAdminRewardCode(id), '删除兑换码失败')
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
