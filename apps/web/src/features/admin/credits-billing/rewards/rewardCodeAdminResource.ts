import {
  createAdminRewardCode,
  listAdminRewardCodes,
} from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type {
  CreatedRewardCode as GeneratedCreatedRewardCode,
  RewardCode as GeneratedRewardCode,
  RewardCodePage as GeneratedRewardCodePage,
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
  expiresAt?: string
  enabled: boolean
  status: GeneratedRewardCode['status']
  createdBy: number
  createdAt: string
  updatedAt: string
}

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

export type RewardCodePage = {
  rewardCodes: RewardCode[]
  nextBeforeId?: number
}

export type AdminRewardCodeResource = {
  list: (beforeId?: number) => Promise<RewardCodePage>
  create: (input: CreateRewardCodeInput) => Promise<CreatedRewardCode>
}

export function createAdminRewardCodeResource(): AdminRewardCodeResource {
  return {
    list: async (beforeId) =>
      toRewardCodePage(
        await resolveApiData(
          listAdminRewardCodes({ before_id: beforeId, limit: 20 }),
          '加载兑换码失败',
        ),
      ),
    create: async (input) =>
      toCreatedRewardCode(
        await resolveApiData(
          createAdminRewardCode({
            display_name: input.displayName.trim(),
            code: input.code.trim(),
            credit_amount: input.creditAmount,
            max_redemptions: input.maxRedemptions,
            idempotency_key: input.idempotencyKey,
            ...(shanghaiDateTimeInputToISO(input.expiresAt)
              ? { expires_at: shanghaiDateTimeInputToISO(input.expiresAt) }
              : {}),
          }),
          '创建兑换码失败',
        ),
      ),
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
    createdBy: value.created_by,
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

function toRewardCodePage(value: GeneratedRewardCodePage): RewardCodePage {
  return {
    rewardCodes: value.reward_codes.map(toRewardCode),
    nextBeforeId: value.next_before_id,
  }
}
