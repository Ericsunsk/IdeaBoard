import {
  createAdminAccount,
  deleteAdminAccount,
  listAdminAccounts,
  startAdminAccount,
  stopAdminAccount,
  updateAdminAccount,
} from './generated-query/admin-accounts/admin-accounts'
import type {
  AdminAccount as GeneratedAdminAccount,
  AdminAccountCreateRequest as GeneratedAdminAccountCreateRequest,
  AdminAccountUpdateRequest as GeneratedAdminAccountUpdateRequest,
  ListAdminAccountsParams,
} from './generated-query/model'
import type { ApiResponse } from './types'

export type ApiAccount = GeneratedAdminAccount

export type CreateAccountRequest = GeneratedAdminAccountCreateRequest

export type UpdateAccountRequest = GeneratedAdminAccountUpdateRequest

export interface AccountListResponse {
  list: ApiAccount[]
  total: number
}

export const createAccount = (
  data: CreateAccountRequest,
): Promise<ApiResponse<{ account: ApiAccount }>> => createAdminAccount(data)

export const updateAccount = (id: number, data: UpdateAccountRequest): Promise<ApiResponse<null>> =>
  updateAdminAccount(id, data)

export const deleteAccount = (id: number): Promise<ApiResponse<null>> => deleteAdminAccount(id)

export const listAccounts = (
  params?: ListAdminAccountsParams,
): Promise<ApiResponse<AccountListResponse>> => listAdminAccounts(params)

export const startAccount = (id: number): Promise<ApiResponse<null>> => startAdminAccount(id)

export const stopAccount = (id: number): Promise<ApiResponse<null>> => stopAdminAccount(id)

export const accountApi = {
  createAccount,
  updateAccount,
  deleteAccount,
  listAccounts,
  startAccount,
  stopAccount,
}

export default accountApi
