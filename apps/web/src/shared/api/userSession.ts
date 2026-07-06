import { getUserInfo } from './generated-query/user/user'

export type { User } from './generated-query/model'

export function fetchCurrentUserInfo() {
  return getUserInfo()
}
