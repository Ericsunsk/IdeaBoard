import { apiGet, apiPost, apiPut, apiDelete } from './client'
import type { ApiResponse } from './types'

export interface ApiAccount {
  id: number
  name: string
  channel_id: number
  platform: string
  api_type: string
  api_key: string
  proxy_url: string
  model_set: string
  max_concurrent: number
  current_concurrent: number
  fail_count: number
  weight: number
  timeout: number
  extra_config: string
  status: number
  created_at: string
  updated_at: string
}

export interface CreateAccountRequest {
  name: string
  platform: string
  api_type: string
  api_key: string
  model_set?: string
  proxy_url?: string
  max_concurrent?: number
  timeout?: number
  weight?: number
  extra_config?: string
}

export interface UpdateAccountRequest {
  name?: string
  platform?: string
  api_type?: string
  api_key?: string
  model_set?: string
  proxy_url?: string
  max_concurrent?: number
  timeout?: number
  weight?: number
  extra_config?: string
  status?: number
}

export interface AccountListResponse {
  list: ApiAccount[]
  total: number
}

export const createAccount = (data: CreateAccountRequest): Promise<ApiResponse<{ account: ApiAccount }>> =>
  apiPost<{ account: ApiAccount }>('/admin/accounts', data)

export const updateAccount = (id: number, data: UpdateAccountRequest): Promise<ApiResponse<null>> =>
  apiPut<null>('/admin/accounts/' + id, data)

export const deleteAccount = (id: number): Promise<ApiResponse<null>> =>
  apiDelete<null>('/admin/accounts/' + id)

export const listAccounts = (params?: { keyword?: string; platform?: string; page?: number; page_size?: number }): Promise<ApiResponse<AccountListResponse>> =>
  apiGet<AccountListResponse>('/admin/accounts', { params })

export const startAccount = (id: number): Promise<ApiResponse<null>> =>
  apiPost<null>('/admin/accounts/' + id + '/start')

export const stopAccount = (id: number): Promise<ApiResponse<null>> =>
  apiPost<null>('/admin/accounts/' + id + '/stop')

export const accountApi = {
  createAccount,
  updateAccount,
  deleteAccount,
  listAccounts,
  startAccount,
  stopAccount,
}

export default accountApi
