import { getUserInfo } from './generated-query/user/user'
import { resolveApiData } from './apiResponse'

export type { User } from './generated-query/model'

export async function fetchCurrentUserInfo() {
  const session = await resolveApiData(getUserInfo(), '获取用户信息失败')
  if (!session.user) throw new Error('获取用户信息失败')
  return session
}
