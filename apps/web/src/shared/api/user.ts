import { apiPut } from './client'
import { getUserInfo } from './generated-query/user/user'
import type { ApiResponse } from './types'

type UpdateUserProfilePayload = {
  avatar?: string
}

export const userApi = {
  getInfo: () => getUserInfo(),
  updateProfile: (payload: UpdateUserProfilePayload): Promise<ApiResponse<null>> =>
    apiPut<null, UpdateUserProfilePayload>('/user/profile', payload),
}
