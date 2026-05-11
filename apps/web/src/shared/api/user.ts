import { getUser } from './generated/user/user'
import type { GetUserInfoResult } from './generated/user/user'

export const userApi = {
  getInfo: (): Promise<GetUserInfoResult> =>
    getUser().getUserInfo(),
}
