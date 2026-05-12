import { getUserInfo } from './generated-query/user/user'

export const userApi = {
  getInfo: () => getUserInfo(),
}
