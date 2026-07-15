import { redeemUserReward } from '../../shared/api/generated-query/user-rewards/user-rewards'
import type { RewardRedeemResult } from '../../shared/api/generated-query/model'
import { resolveApiData } from '../../shared/api/apiResponse'

export type UserRewardRedeemResult = RewardRedeemResult

export async function redeemUserRewardCode(code: string): Promise<UserRewardRedeemResult> {
  return resolveApiData(redeemUserReward({ code }), '网络错误，请重试')
}
