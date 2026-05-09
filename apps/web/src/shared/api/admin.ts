import api from './client'
import type { UserListResponse, User } from '../types'
import type { ApiResponse } from './types'

export const adminApi = {
  getRegistrationSetting: (): Promise<ApiResponse<{ enabled: boolean }>> =>
    api.get('/admin/settings/registration') as any,
  updateRegistrationSetting: (enabled: boolean): Promise<ApiResponse<{ enabled: boolean }>> =>
    api.put('/admin/settings/registration', { enabled }) as any,

  getRateLimitSettings: (): Promise<ApiResponse<{ window_seconds: number; max_requests: number }>> =>
    api.get('/admin/settings/rate-limit') as any,
  updateRateLimitSettings: (data: { window_seconds: number; max_requests: number }): Promise<ApiResponse<{ window_seconds: number; max_requests: number }>> =>
    api.put('/admin/settings/rate-limit', data) as any,

  getUsers: (params: { keyword?: string; page?: number; page_size?: number }): Promise<ApiResponse<UserListResponse>> =>
    api.get('/admin/users', { params }) as any,
  updateUserStatus: (id: number, status: number): Promise<ApiResponse<null>> =>
    api.put('/admin/users/' + id + '/status', { status }) as any,
  updateUserRole: (id: number, role: 'admin' | 'user'): Promise<ApiResponse<null>> =>
    api.put('/admin/users/' + id + '/role', { role }) as any,
  deleteUser: (id: number): Promise<ApiResponse<null>> =>
    api.delete('/admin/users/' + id) as any,
}
