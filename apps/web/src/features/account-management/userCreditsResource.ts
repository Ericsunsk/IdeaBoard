import { getUserCreditWallet } from '../../shared/api/generated-query/user-credits/user-credits'
import type { CreditWalletDetails } from '../../shared/api/generated-query/model'

type ApiEnvelope<T> = { code?: number; msg?: string; data?: T }

export type UserCreditWallet = CreditWalletDetails

export const userCreditsQueryKeys = {
  all: ['user', 'credits'] as const,
  wallet: ['user', 'credits', 'wallet'] as const,
}

export async function fetchUserCreditWallet(): Promise<UserCreditWallet> {
  return requireUserCreditsData(await getUserCreditWallet(), '加载Idea余额失败')
}

function requireUserCreditsData<T>(response: ApiEnvelope<T>, fallbackMessage: string): T {
  if (response.code !== 0 || response.data == null) {
    throw new Error(response.msg || fallbackMessage)
  }
  return response.data
}
