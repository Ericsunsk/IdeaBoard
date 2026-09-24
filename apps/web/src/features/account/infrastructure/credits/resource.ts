import { api } from '../../../../shared/api/client'
import { resolveApiData, type ApiResponseWithData } from '../../../../shared/api/apiResponse'
import { i18n } from '../../../../shared/i18n'

// User credit wallet boundary (issue #809). Transports through the central
// `api.request` against the surviving `GET /api/user/credits/wallet` route
// with local view types.
export type CreditWallet = {
  public_id: string
  display_name: string
  available_credits: number
  reserved_credits: number
}

export type CreditWalletSummary = CreditWallet

export const creditsQueryKeys = {
  all: ['user', 'credits'] as const,
  wallet: ['user', 'credits', 'wallet'] as const,
}

export async function fetchCreditWallet(): Promise<CreditWallet> {
  return resolveApiData(
    api.request<ApiResponseWithData<CreditWallet>>('/user/credits/wallet', { method: 'GET' }),
    i18n.global.t('account.errors.walletLoadFailed'),
  )
}
