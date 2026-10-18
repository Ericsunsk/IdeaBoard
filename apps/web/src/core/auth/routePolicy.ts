/** Route invariants used by identity failure and safe-redirect policy. */
export const HOME_PATH = '/'
export const USER_ROOT_PATH = '/user'
export const USER_LOGIN_PATH = '/user/login'
export const USER_REGISTER_PATH = '/user/register'
export const USER_BOARDS_PATH = '/user/boards'
export const ADMIN_ROOT_PATH = '/admin'

export function isPathUnderRoute(path: string, rootPath: string): boolean {
  const routePath = path.split(/[?#]/, 1)[0] || '/'
  const normalizedRoot =
    rootPath.endsWith('/') && rootPath !== '/' ? rootPath.slice(0, -1) : rootPath
  return routePath === normalizedRoot || routePath.startsWith(`${normalizedRoot}/`)
}
