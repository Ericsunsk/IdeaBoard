import { getUserCreditWallet } from '../../shared/api/generated-query/user-credits/user-credits'
import type { CreditWallet } from '../../shared/api/generated-query/model'
import { resolveApiData } from '../../shared/api/apiResponse'
import { i18n } from '../../shared/i18n'

export type UserCreditWallet = CreditWallet

export const userCreditsQueryKeys = {
  all: ['user', 'credits'] as const,
  wallet: ['user', 'credits', 'wallet'] as const,
}

export async function fetchUserCreditWallet(): Promise<UserCreditWallet> {
  return resolveApiData(getUserCreditWallet(), i18n.global.t('account.errors.walletLoadFailed'))
}
