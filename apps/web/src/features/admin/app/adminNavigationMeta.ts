import {
  ADMIN_CHANNELS_PATH,
  ADMIN_FEATURE_PROMPTS_PATH,
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
  | 'channels'
  | 'statistics'
  | 'task-monitor'
  | 'feature-prompts'
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
    key: 'channels',
    path: ADMIN_CHANNELS_PATH,
    childPath: 'channels',
    label: '渠道商管理',
    title: '渠道商管理',
    routeName: 'AdminChannels',
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
    key: 'task-monitor',
    path: ADMIN_TASK_MONITOR_PATH,
    childPath: 'task-monitor',
    label: '任务监控',
    title: '任务监控',
    routeName: 'AdminTaskMonitor',
  },
  {
    key: 'feature-prompts',
    path: ADMIN_FEATURE_PROMPTS_PATH,
    childPath: 'feature-prompts',
    label: '系统提示词',
    title: '系统提示词',
    routeName: 'AdminFeaturePrompts',
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
