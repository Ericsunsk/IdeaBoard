import {
  listUserRewardRedemptions,
  redeemUserReward,
} from '../../shared/api/generated-query/user-rewards/user-rewards'
import type {
  ListUserRewardRedemptionsParams,
  RewardRedeemResult,
  RewardRedemptionHistoryItem,
  RewardRedemptionHistoryPage,
} from '../../shared/api/generated-query/model'
import { resolveApiData } from '../../shared/api/apiResponse'

export type UserRewardRedeemResult = RewardRedeemResult
export type UserRewardRedemption = RewardRedemptionHistoryItem
export type UserRewardRedemptionPage = RewardRedemptionHistoryPage

export const userRewardRedemptionsQueryKeys = {
  all: ['user', 'rewards', 'redemptions'] as const,
}

export async function redeemUserRewardCode(code: string): Promise<UserRewardRedeemResult> {
  try {
    return await resolveApiData(redeemUserReward({ code }), '网络错误，请重试')
  } catch (error) {
    throw new Error(rewardRedemptionErrorMessage(error), { cause: error })
  }
}

function rewardRedemptionErrorMessage(error: unknown): string {
  const status =
    error && typeof error === 'object' && 'status' in error
      ? Number((error as { status?: unknown }).status)
      : 0
  switch (status) {
    case 400:
      return '参数错误'
    case 422:
      return '兑换码无效或已失效'
    case 409:
      return '你已兑换过该奖励'
    case 423:
      return '当前账户无法兑换奖励'
    case 429:
      return '操作频繁，请稍后重试'
    default:
      return '网络错误，请重试'
  }
}

export async function fetchUserRewardRedemptions(
  params: ListUserRewardRedemptionsParams,
  signal?: AbortSignal,
): Promise<UserRewardRedemptionPage> {
  return resolveApiData(listUserRewardRedemptions(params, { signal }), '加载兑换记录失败')
}
