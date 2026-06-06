export const ADMIN_DEFAULT_PATH = '/admin/statistics'
export const ADMIN_LOGIN_PATH = '/admin/login'

export type AdminNavigationKey =
  | 'users'
  | 'channels'
  | 'statistics'
  | 'task-monitor'
  | 'feature-prompts'
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
    path: '/admin/statistics',
    childPath: 'statistics',
    label: '数据仪表盘',
    title: '数据仪表盘',
    routeName: 'AdminStatistics',
  },
  {
    key: 'channels',
    path: '/admin/channels',
    childPath: 'channels',
    label: '渠道商管理',
    title: '渠道商管理',
    routeName: 'AdminChannels',
  },
  {
    key: 'users',
    path: '/admin/users',
    childPath: 'users',
    label: '用户管理',
    title: '用户管理',
    routeName: 'AdminUsers',
  },
  {
    key: 'task-monitor',
    path: '/admin/task-monitor',
    childPath: 'task-monitor',
    label: '任务监控',
    title: '任务监控',
    routeName: 'AdminTaskMonitor',
  },
  {
    key: 'feature-prompts',
    path: '/admin/feature-prompts',
    childPath: 'feature-prompts',
    label: '系统提示词',
    title: '系统提示词',
    routeName: 'AdminFeaturePrompts',
  },
  {
    key: 'oss-config',
    path: '/admin/oss-config',
    childPath: 'oss-config',
    label: '系统存储',
    title: '系统存储',
    routeName: 'AdminOSSConfig',
  },
]
