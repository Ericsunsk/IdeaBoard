import {
  ADMIN_CREDITS_PATH,
  ADMIN_FEATURE_PROMPTS_PATH,
  ADMIN_MODELS_PATH,
  ADMIN_SETTINGS_PATH,
  ADMIN_STATISTICS_PATH,
  ADMIN_MONITOR_PATH,
  ADMIN_USERS_PATH,
} from '../routePaths'

export const ADMIN_DEFAULT_PATH = ADMIN_STATISTICS_PATH

export type AdminNavigationKey =
  'users' | 'credits' | 'statistics' | 'monitor' | 'models' | 'feature-prompts' | 'settings'

export type AdminNavigationRouteDef = {
  key: AdminNavigationKey
  path: `/admin/${string}`
  childPath: string
  label: string
  title: string
  subtitle: string
  routeName: string
}

export const adminNavigationRouteDefs: AdminNavigationRouteDef[] = [
  {
    key: 'statistics',
    path: ADMIN_STATISTICS_PATH,
    childPath: 'statistics',
    label: '数据仪表盘',
    title: '数据仪表盘',
    subtitle: '调用量、成功率与模型健康一览',
    routeName: 'AdminStatistics',
  },
  {
    key: 'users',
    path: ADMIN_USERS_PATH,
    childPath: 'users',
    label: '用户管理',
    title: '用户管理',
    subtitle: '搜索用户、管理角色与封禁状态',
    routeName: 'AdminUsers',
  },
  {
    key: 'credits',
    path: ADMIN_CREDITS_PATH,
    childPath: 'credits',
    label: '积分运营',
    title: '积分运营',
    subtitle: '积分账本、定价与对账',
    routeName: 'AdminCredits',
  },
  {
    key: 'monitor',
    path: ADMIN_MONITOR_PATH,
    childPath: 'monitor',
    label: '任务监控',
    title: '任务监控',
    subtitle: '任务执行轨迹、耗时与错误诊断',
    routeName: 'AdminMonitor',
  },
  {
    key: 'models',
    path: ADMIN_MODELS_PATH,
    childPath: 'models',
    label: '模型管理',
    title: '模型管理',
    subtitle: '模型接入、映射规则与路由策略',
    routeName: 'AdminModels',
  },
  {
    key: 'feature-prompts',
    path: ADMIN_FEATURE_PROMPTS_PATH,
    childPath: 'feature-prompts',
    label: '系统提示词',
    title: '系统提示词',
    subtitle: '各功能系统提示词模板',
    routeName: 'AdminFeaturePrompts',
  },
  {
    key: 'settings',
    path: ADMIN_SETTINGS_PATH,
    childPath: 'settings',
    label: '系统设置',
    title: '系统设置',
    subtitle: '网站设置、存储、邮件服务与安全策略',
    routeName: 'AdminSettings',
  },
]
