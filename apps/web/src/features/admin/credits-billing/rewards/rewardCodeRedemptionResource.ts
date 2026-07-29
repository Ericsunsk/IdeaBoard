import {
  getAdminRewardCodeRedemption,
  listAdminRewardCodeRedemptions,
} from '../../../../shared/api/generated-query/admin-rewards/admin-rewards'
import type {
  AdminRewardCodeRedemptionDetail as GeneratedRedemptionDetail,
  AdminRewardCodeRedemptionPage as GeneratedRedemptionPage,
  AdminRewardCodeRedemptionSummary as GeneratedRedemption,
} from '../../../../shared/api/generated-query/model'
import { resolveApiData } from '../../../../shared/api/apiResponse'

export type RewardCodeRedemption = {
  id: number
  rewardCodeId: number
  userId: number
  creditedAmount: number
  redeemedAt: string
}

export type RewardCodeRedemptionDetail = RewardCodeRedemption & {
  creditTransactionId: string
  maskedCodeHint: string
}

export type RewardCodeRedemptionPage = {
  redemptions: RewardCodeRedemption[]
  nextBeforeId?: number
}

export type RewardCodeRedemptionResource = {
  list: (
    rewardCodeId: number,
    input?: { beforeId?: number; limit?: number; signal?: AbortSignal },
  ) => Promise<RewardCodeRedemptionPage>
  get: (
    rewardCodeId: number,
    redemptionId: number,
    input?: { signal?: AbortSignal },
  ) => Promise<RewardCodeRedemptionDetail>
}

export function createRewardCodeRedemptionResource(): RewardCodeRedemptionResource {
  return {
    list: async (rewardCodeId, { beforeId, limit = 20, signal } = {}) => {
      const page: GeneratedRedemptionPage = await resolveApiData(
        listAdminRewardCodeRedemptions(
          rewardCodeId,
          {
            ...(beforeId !== undefined ? { before_id: beforeId } : {}),
            limit,
          },
          signal ? { signal } : undefined,
        ),
        '加载兑换记录失败',
      )
      return {
        redemptions: page.redemptions.map(toRedemption),
        nextBeforeId: page.next_before_id,
      }
    },
    get: async (rewardCodeId, redemptionId, { signal } = {}) => {
      const detail: GeneratedRedemptionDetail = await resolveApiData(
        getAdminRewardCodeRedemption(rewardCodeId, redemptionId, signal ? { signal } : undefined),
        '加载兑换详情失败',
      )
      return {
        ...toRedemption(detail),
        creditTransactionId: detail.credit_transaction_id,
        maskedCodeHint: detail.masked_code_hint,
      }
    },
  }
}

function toRedemption(value: GeneratedRedemption): RewardCodeRedemption {
  return {
    id: value.id,
    rewardCodeId: value.reward_code_id,
    userId: value.user_id,
    creditedAmount: value.credited_amount,
    redeemedAt: value.redeemed_at,
  }
}
