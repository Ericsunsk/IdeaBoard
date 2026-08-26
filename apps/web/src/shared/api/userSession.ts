import { getUserInfo } from './generated-query/user/user'
import { resolveApiData } from './apiResponse'
import { i18n } from '../i18n'

export type { User } from './generated-query/model'

export async function fetchCurrentUserInfo() {
  const fallback = i18n.global.t('common.errors.sessionLoadFailed')
  const session = await resolveApiData(getUserInfo(), fallback)
  if (!session.user) throw new Error(fallback)
  return session
}
