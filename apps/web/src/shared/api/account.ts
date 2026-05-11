import { getAdminAccounts } from './generated/admin-accounts/admin-accounts'
import type {
  AdminAccount as GeneratedAdminAccount,
  AdminAccountCreateRequest as GeneratedAdminAccountCreateRequest,
  AdminAccountUpdateRequest as GeneratedAdminAccountUpdateRequest,
} from './generated/model'
import type { ApiResponse } from './types'

export type ApiAccount = GeneratedAdminAccount

export type CreateAccountRequest = GeneratedAdminAccountCreateRequest

export type UpdateAccountRequest = GeneratedAdminAccountUpdateRequest

export interface AccountListResponse {
  list: ApiAccount[]
  total: number
}

const adminAccountsApi = getAdminAccounts()

export const createAccount = (
  data: CreateAccountRequest,
): Promise<ApiResponse<{ account: ApiAccount }>> => adminAccountsApi.createAdminAccount(data)

export const updateAccount = (id: number, data: UpdateAccountRequest): Promise<ApiResponse<null>> =>
  adminAccountsApi.updateAdminAccount(id, data)

export const deleteAccount = (id: number): Promise<ApiResponse<null>> =>
  adminAccountsApi.deleteAdminAccount(id)

export const listAccounts = (params?: {
  keyword?: string
  platform?: string
  page?: number
  page_size?: number
}): Promise<ApiResponse<AccountListResponse>> => adminAccountsApi.listAdminAccounts(params)

export const startAccount = (id: number): Promise<ApiResponse<null>> =>
  adminAccountsApi.startAdminAccount(id)

export const stopAccount = (id: number): Promise<ApiResponse<null>> =>
  adminAccountsApi.stopAdminAccount(id)

export const accountApi = {
  createAccount,
  updateAccount,
  deleteAccount,
  listAccounts,
  startAccount,
  stopAccount,
}

export default accountApi
