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
  return resolveApiData(redeemUserReward({ code }), '网络错误，请重试')
}

export async function fetchUserRewardRedemptions(
  params: ListUserRewardRedemptionsParams,
  signal?: AbortSignal,
): Promise<UserRewardRedemptionPage> {
  return resolveApiData(listUserRewardRedemptions(params, { signal }), '加载兑换记录失败')
}
