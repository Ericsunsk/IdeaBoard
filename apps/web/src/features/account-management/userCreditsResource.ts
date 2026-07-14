import { getUserCreditWallet } from '../../shared/api/generated-query/user-credits/user-credits'
import type { CreditWalletDetails } from '../../shared/api/generated-query/model'
import { requireApiData } from '../../shared/api/apiResponse'

export type UserCreditWallet = CreditWalletDetails

export const userCreditsQueryKeys = {
  all: ['user', 'credits'] as const,
  wallet: ['user', 'credits', 'wallet'] as const,
}

export async function fetchUserCreditWallet(): Promise<UserCreditWallet> {
  return requireApiData(await getUserCreditWallet(), '加载Idea余额失败')
}
