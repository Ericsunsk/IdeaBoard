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
import { i18n } from '../../shared/i18n'

export type UserRewardRedeemResult = RewardRedeemResult
export type UserRewardRedemption = RewardRedemptionHistoryItem
export type UserRewardRedemptionPage = RewardRedemptionHistoryPage

export const userRewardRedemptionsQueryKeys = {
  all: ['user', 'rewards', 'redemptions'] as const,
}

export async function redeemUserRewardCode(code: string): Promise<UserRewardRedeemResult> {
  try {
    return await resolveApiData(redeemUserReward({ code }), i18n.global.t('account.errors.networkRetry'))
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
      return i18n.global.t('account.errors.redeemInvalidRequest')
    case 422:
      return i18n.global.t('account.errors.redeemCodeInvalid')
    case 409:
      return i18n.global.t('account.errors.redeemAlreadyUsed')
    case 423:
      return i18n.global.t('account.errors.redeemNotAllowed')
    case 429:
      return i18n.global.t('account.errors.tooManyRequests')
    default:
      return i18n.global.t('account.errors.networkRetry')
  }
}

export async function fetchUserRewardRedemptions(
  params: ListUserRewardRedemptionsParams,
  signal?: AbortSignal,
): Promise<UserRewardRedemptionPage> {
  return resolveApiData(
      listUserRewardRedemptions(params, { signal }),
      i18n.global.t('account.errors.rewardsHistoryLoadFailed'),
    )
}
