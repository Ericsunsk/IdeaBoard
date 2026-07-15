export const SHARE_JOIN_PATH = '/share'

export const USER_ROOT_PATH = '/user'
export const USER_HOME_PATH = '/user/home'
export const USER_LOGIN_PATH = '/user/login'
export const USER_REGISTER_PATH = '/user/register'
export const USER_BOARDS_PATH = '/user/boards'
export const USER_CANVAS_PATH = '/user/canvas'

export const ADMIN_ROOT_PATH = '/admin'
export const ADMIN_STATISTICS_PATH = '/admin/statistics'
export const ADMIN_PROVIDER_ENDPOINTS_PATH = '/admin/provider-endpoints'
export const ADMIN_USERS_PATH = '/admin/users'
export const ADMIN_CREDITS_PATH = '/admin/credits'
export const ADMIN_CREDIT_WALLETS_PATH = '/admin/credits/wallets'
export const ADMIN_CREDIT_PRICING_PATH = '/admin/credits/pricing'
export const ADMIN_CREDIT_RECONCILIATION_PATH = '/admin/credits/reconciliation'
export const ADMIN_REWARD_CAMPAIGNS_PATH = '/admin/credits/reward-campaigns'
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
