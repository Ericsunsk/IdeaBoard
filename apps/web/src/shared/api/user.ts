import api from './client'
import type { User } from '../types'
import type { ApiResponse } from './types'

export const userApi = {
  getInfo: (): Promise<ApiResponse<{ user: User }>> =>
    api.get('/user/info') as any,
}
