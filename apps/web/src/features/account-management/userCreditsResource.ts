import {
  getUserCreditWallet,
  listUserCreditLedger,
} from '../../shared/api/generated-query/user-credits/user-credits'
import type {
  CreditLedgerPage,
  CreditLedgerTransaction,
  CreditLedgerTransactionCommandType,
  CreditPosting,
  CreditWalletDetails,
  ListUserCreditLedgerParams,
} from '../../shared/api/generated-query/model'

type ApiEnvelope<T> = { code?: number; msg?: string; data?: T }

export type UserCreditWallet = CreditWalletDetails
export type UserCreditLedgerPage = CreditLedgerPage
export type UserCreditLedgerTransaction = CreditLedgerTransaction
export type UserCreditLedgerCommandType = CreditLedgerTransactionCommandType
export type UserCreditPosting = CreditPosting

export const userCreditsQueryKeys = {
  all: ['user', 'credits'] as const,
  wallet: ['user', 'credits', 'wallet'] as const,
  ledger: ['user', 'credits', 'ledger'] as const,
}

export async function fetchUserCreditWallet(): Promise<UserCreditWallet> {
  return requireUserCreditsData(await getUserCreditWallet(), '加载Idea余额失败')
}

export async function fetchUserCreditLedger(
  params?: ListUserCreditLedgerParams,
): Promise<UserCreditLedgerPage> {
  return requireUserCreditsData(await listUserCreditLedger(params), '加载Idea流水失败')
}

function requireUserCreditsData<T>(response: ApiEnvelope<T>, fallbackMessage: string): T {
  if (response.code !== 0 || response.data == null) {
    throw new Error(response.msg || fallbackMessage)
  }
  return response.data
}
