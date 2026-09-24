import { api } from '../../../../shared/api/client'
import { resolveApiData, type ApiResponseWithData } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

// User rewards boundary (issue #809). Transports through the central
// `api.request` against the surviving `/api/user/rewards/*` routes with
// local view types.
export type ListUserRewardRedemptionsParams = {
  before_id?: number
  limit?: number
}

export type RewardRedemption = {
  id: number
  reward_code_id: number
  user_public_id: string
  display_name: string
  credit_amount: number
  credit_transaction_id: string
  available_balance_after: number
  redeemed_at: string
}

export type RewardRedemptionHistoryItem = {
  id: number
  reward_code_display_name: string
  credited_amount: number
  masked_code_hint: string
  redeemed_at: string
}

export type RewardRedemptionHistoryPage = {
  redemptions: RewardRedemptionHistoryItem[]
  next_before_id?: number
}

export type RewardRedeemResult = {
  redemption: RewardRedemption
  history_item: RewardRedemptionHistoryItem
  credited_amount: number
  available_balance: number
  replayed: boolean
}

export type RewardRedemptionItem = RewardRedemptionHistoryItem
export type RewardRedemptionPageData = RewardRedemptionHistoryPage

export const rewardRedemptionsQueryKeys = {
  all: ['user', 'rewards', 'redemptions'] as const,
}

export async function redeemRewardCode(code: string): Promise<RewardRedeemResult> {
  try {
    return await resolveApiData(
      api.request<ApiResponseWithData<RewardRedeemResult>>('/user/rewards/redeem', {
        method: 'POST',
        body: { code } as unknown as Record<string, unknown>,
      }),
      i18n.global.t('account.errors.networkRetry'),
    )
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

function redemptionsUrl(params: ListUserRewardRedemptionsParams): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params ?? {})) {
    if (value !== undefined) search.append(key, String(value))
  }
  const query = search.toString()
  return query ? `/user/rewards/redemptions?${query}` : '/user/rewards/redemptions'
}

export async function fetchRewardRedemptions(
  params: ListUserRewardRedemptionsParams,
  signal?: AbortSignal,
): Promise<RewardRedemptionHistoryPage> {
  return resolveApiData(
    api.request<ApiResponseWithData<RewardRedemptionHistoryPage>>(redemptionsUrl(params), {
      method: 'GET',
      ...(signal == null ? {} : { signal }),
    }),
    i18n.global.t('account.errors.rewardsHistoryLoadFailed'),
  )
}
