import { apiDelete, apiGet, apiPut } from './client'
import type { UserListResponse, User } from '../types'
import type { ApiResponse } from './types'

export const adminApi = {
  getRegistrationSetting: (): Promise<ApiResponse<{ enabled: boolean }>> =>
    apiGet<{ enabled: boolean }>('/admin/settings/registration'),
  updateRegistrationSetting: (enabled: boolean): Promise<ApiResponse<{ enabled: boolean }>> =>
    apiPut<{ enabled: boolean }, { enabled: boolean }>('/admin/settings/registration', { enabled }),

  getRateLimitSettings: (): Promise<ApiResponse<{ window_seconds: number; max_requests: number }>> =>
    apiGet<{ window_seconds: number; max_requests: number }>('/admin/settings/rate-limit'),
  updateRateLimitSettings: (data: { window_seconds: number; max_requests: number }): Promise<ApiResponse<{ window_seconds: number; max_requests: number }>> =>
    apiPut<{ window_seconds: number; max_requests: number }, { window_seconds: number; max_requests: number }>('/admin/settings/rate-limit', data),

  getUsers: (params: { keyword?: string; page?: number; page_size?: number }): Promise<ApiResponse<UserListResponse>> =>
    apiGet<UserListResponse>('/admin/users', { params }),
  updateUserStatus: (id: number, status: number): Promise<ApiResponse<null>> =>
    apiPut<null, { status: number }>('/admin/users/' + id + '/status', { status }),
  updateUserRole: (id: number, role: 'admin' | 'user'): Promise<ApiResponse<null>> =>
    apiPut<null, { role: 'admin' | 'user' }>('/admin/users/' + id + '/role', { role }),
  deleteUser: (id: number): Promise<ApiResponse<null>> =>
    apiDelete<null>('/admin/users/' + id),
}
