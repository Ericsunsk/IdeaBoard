import type { Component } from 'vue'
import { ChatBubble, Cloud, DashboardSpeed, Group, Link, List } from '@iconoir/vue'

export const ADMIN_DEFAULT_PATH = '/admin/users'
export const ADMIN_LOGIN_PATH = '/admin/login'

export type AdminNavigationKey =
  | 'users'
  | 'channels'
  | 'statistics'
  | 'task-monitor'
  | 'feature-prompts'
  | 'oss-config'

export type AdminNavigationItem = {
  key: AdminNavigationKey
  path: `/admin/${string}`
  childPath: string
  label: string
  title: string
  icon: Component
  routeName: string
}

export const adminNavigationItems: AdminNavigationItem[] = [
  {
    key: 'users',
    path: '/admin/users',
    childPath: 'users',
    label: '用户管理',
    title: '用户管理',
    icon: Group,
    routeName: 'AdminUsers',
  },
  {
    key: 'channels',
    path: '/admin/channels',
    childPath: 'channels',
    label: '渠道商管理',
    title: '渠道商管理',
    icon: Link,
    routeName: 'AdminChannels',
  },
  {
    key: 'statistics',
    path: '/admin/statistics',
    childPath: 'statistics',
    label: '数据仪表盘',
    title: '数据仪表盘',
    icon: DashboardSpeed,
    routeName: 'AdminStatistics',
  },
  {
    key: 'task-monitor',
    path: '/admin/task-monitor',
    childPath: 'task-monitor',
    label: '任务监控',
    title: '任务监控',
    icon: List,
    routeName: 'AdminTaskMonitor',
  },
  {
    key: 'feature-prompts',
    path: '/admin/feature-prompts',
    childPath: 'feature-prompts',
    label: '系统提示词',
    title: '系统提示词',
    icon: ChatBubble,
    routeName: 'AdminFeaturePrompts',
  },
  {
    key: 'oss-config',
    path: '/admin/oss-config',
    childPath: 'oss-config',
    label: '系统存储',
    title: '系统存储',
    icon: Cloud,
    routeName: 'AdminOSSConfig',
  },
]

export const adminNavigationByKey = adminNavigationItems.reduce(
  (acc, item) => {
    acc[item.key] = item
    return acc
  },
  {} as Record<AdminNavigationKey, AdminNavigationItem>,
)

export function findAdminNavigationItemByPath(path: string) {
  return adminNavigationItems.find((item) => path === item.path || path.startsWith(`${item.path}/`))
}
