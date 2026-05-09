import { apiGet } from './client'
import type { User } from '../types'
import type { ApiResponse } from './types'

export const userApi = {
  getInfo: (): Promise<ApiResponse<{ user: User }>> =>
    apiGet<{ user: User }>('/user/info'),
}
