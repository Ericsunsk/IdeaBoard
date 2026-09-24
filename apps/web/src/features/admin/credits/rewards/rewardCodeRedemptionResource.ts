import { api } from '../../../../shared/api/client'
import {
  resolveApiData,
  type ApiResponseWithData,
} from '../../../../shared/api/apiResponse'

// Admin reward redemption boundary (issue #809). Transports through the
// central `api.request` against the surviving
// `/api/admin/rewards/codes/:id/redemptions*` routes with local view types.
export type GeneratedRedemption = {
  id: number
  reward_code_id: number
  user_public_id: string
  display_name: string
  credited_amount: number
  redeemed_at: string
}

export type GeneratedRedemptionDetail = GeneratedRedemption & {
  credit_transaction_id: string
  masked_code_hint: string
}

export type GeneratedRedemptionPage = {
  redemptions: GeneratedRedemption[]
  next_before_id?: number
}

export type RewardCodeRedemption = {
  id: number
  rewardCodeId: number
  userPublicId: string
  displayName: string
  creditedAmount: number
  redeemedAt: string
}

export type RewardCodeRedemptionDetail = RewardCodeRedemption & {
  creditTransactionId: string
  maskedCodeHint: string
}

export type RewardCodeRedemptionPage = {
  redemptions: RewardCodeRedemption[]
  nextBeforeId?: number | undefined
}

export type RewardCodeRedemptionResource = {
  list: (
    rewardCodeId: number,
    input?: {
      beforeId?: number | undefined
      limit?: number | undefined
      signal?: AbortSignal | undefined
    },
  ) => Promise<RewardCodeRedemptionPage>
  get: (
    rewardCodeId: number,
    redemptionId: number,
    input?: { signal?: AbortSignal },
  ) => Promise<RewardCodeRedemptionDetail>
}

function redemptionsUrl(rewardCodeId: number, beforeId?: number, limit?: number): string {
  const search = new URLSearchParams()
  if (beforeId !== undefined) search.append('before_id', String(beforeId))
  if (limit !== undefined) search.append('limit', String(limit))
  const query = search.toString()
  const base = `/admin/rewards/codes/${encodeURIComponent(String(rewardCodeId))}/redemptions`
  return query ? `${base}?${query}` : base
}

export function createRewardCodeRedemptionResource(): RewardCodeRedemptionResource {
  return {
    list: async (rewardCodeId, { beforeId, limit = 20, signal } = {}) => {
      const page: GeneratedRedemptionPage = await resolveApiData(
        api.request<ApiResponseWithData<GeneratedRedemptionPage>>(
          redemptionsUrl(rewardCodeId, beforeId, limit),
          {
            method: 'GET',
            ...(signal == null ? {} : { signal }),
          },
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
        api.request<ApiResponseWithData<GeneratedRedemptionDetail>>(
          `/admin/rewards/codes/${encodeURIComponent(String(rewardCodeId))}/redemptions/${encodeURIComponent(String(redemptionId))}`,
          {
            method: 'GET',
            ...(signal == null ? {} : { signal }),
          },
        ),
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
    userPublicId: value.user_public_id,
    displayName: value.display_name,
    creditedAmount: value.credited_amount,
    redeemedAt: value.redeemed_at,
  }
}
