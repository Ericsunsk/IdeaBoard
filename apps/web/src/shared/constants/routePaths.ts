export const SHARE_JOIN_PATH = '/share'

export const USER_ROOT_PATH = '/user'
export const USER_HOME_PATH = '/user/home'
export const USER_LOGIN_PATH = '/user/login'
export const USER_REGISTER_PATH = '/user/register'
export const USER_BOARDS_PATH = '/user/boards'
export const USER_CENTER_PATH = '/user/center'
export const USER_WORKSPACE_PATH = '/user/workspace'

export const ADMIN_ROOT_PATH = '/admin'
export const ADMIN_STATISTICS_PATH = '/admin/statistics'
export const ADMIN_CHANNELS_PATH = '/admin/channels'
export const ADMIN_USERS_PATH = '/admin/users'
export const ADMIN_TASK_MONITOR_PATH = '/admin/task-monitor'
export const ADMIN_FEATURE_PROMPTS_PATH = '/admin/feature-prompts'
export const ADMIN_OSS_CONFIG_PATH = '/admin/oss-config'
export const ADMIN_SETTINGS_PATH = '/admin/settings'
export const ADMIN_LOGIN_PATH = '/admin/login'

export function isPathUnderRoute(path: string, rootPath: string): boolean {
  const routePath = path.split(/[?#]/, 1)[0] || '/'
  const normalizedRoot =
    rootPath.endsWith('/') && rootPath !== '/' ? rootPath.slice(0, -1) : rootPath
  return routePath === normalizedRoot || routePath.startsWith(`${normalizedRoot}/`)
}
