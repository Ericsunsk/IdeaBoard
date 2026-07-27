import {
  ADMIN_PROVIDER_ENDPOINTS_PATH,
  ADMIN_CREDITS_PATH,
  ADMIN_WORKFLOW_FEATURES_PATH,
  ADMIN_LOGIN_PATH,
  ADMIN_OSS_CONFIG_PATH,
  ADMIN_SETTINGS_PATH,
  ADMIN_STATISTICS_PATH,
  ADMIN_TASK_MONITOR_PATH,
  ADMIN_USERS_PATH,
} from '../../../shared/constants/routePaths'

export const ADMIN_DEFAULT_PATH = ADMIN_STATISTICS_PATH
export { ADMIN_LOGIN_PATH }

export type AdminNavigationKey =
  | 'users'
  | 'credits'
  | 'provider-endpoints'
  | 'statistics'
  | 'task-monitor'
  | 'workflow-features'
  | 'system-settings'
  | 'oss-config'

export type AdminNavigationRouteDef = {
  key: AdminNavigationKey
  path: `/admin/${string}`
  childPath: string
  label: string
  title: string
  routeName: string
}

export const adminNavigationRouteDefs: AdminNavigationRouteDef[] = [
  {
    key: 'statistics',
    path: ADMIN_STATISTICS_PATH,
    childPath: 'statistics',
    label: '数据仪表盘',
    title: '数据仪表盘',
    routeName: 'AdminStatistics',
  },
  {
    key: 'provider-endpoints',
    path: ADMIN_PROVIDER_ENDPOINTS_PATH,
    childPath: 'provider-endpoints',
    label: '供应商端点',
    title: '供应商端点',
    routeName: 'AdminProviderEndpoints',
  },
  {
    key: 'users',
    path: ADMIN_USERS_PATH,
    childPath: 'users',
    label: '用户管理',
    title: '用户管理',
    routeName: 'AdminUsers',
  },
  {
    key: 'credits',
    path: ADMIN_CREDITS_PATH,
    childPath: 'credits',
    label: '积分运营',
    title: '积分运营',
    routeName: 'AdminCredits',
  },
  {
    key: 'task-monitor',
    path: ADMIN_TASK_MONITOR_PATH,
    childPath: 'task-monitor',
    label: '任务监控',
    title: '任务监控',
    routeName: 'AdminTaskMonitor',
  },
  {
    key: 'workflow-features',
    path: ADMIN_WORKFLOW_FEATURES_PATH,
    childPath: 'workflow-features',
    label: '工作流配置',
    title: '工作流配置',
    routeName: 'AdminWorkflowFeatures',
  },
  {
    key: 'oss-config',
    path: ADMIN_OSS_CONFIG_PATH,
    childPath: 'oss-config',
    label: '系统存储',
    title: '系统存储',
    routeName: 'AdminOSSConfig',
  },
  {
    key: 'system-settings',
    path: ADMIN_SETTINGS_PATH,
    childPath: 'settings',
    label: '系统设置',
    title: '系统设置',
    routeName: 'AdminSystemSettings',
  },
]
