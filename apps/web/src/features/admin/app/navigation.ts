import type { Component } from 'vue'
import {
  ChatBubble,
  Cube,
  DashboardSpeed,
  Group,
  Settings,
  StatsDownSquare,
  Wallet,
} from '@iconoir/vue'
import { ADMIN_DEFAULT_PATH, adminNavigationRouteDefs } from './navigationMeta'
import { isPathUnderRoute } from '../../../shared/constants/routePaths'
import type { AdminNavigationKey, AdminNavigationRouteDef } from './navigationMeta'

export { ADMIN_DEFAULT_PATH }
export type { AdminNavigationKey }

export type AdminNavigationItem = AdminNavigationRouteDef & {
  icon: Component
}

const adminNavigationIcons = {
  users: Group,
  credits: Wallet,
  statistics: DashboardSpeed,
  monitor: StatsDownSquare,
  models: Cube,
  'feature-prompts': ChatBubble,
  settings: Settings,
} satisfies Record<AdminNavigationKey, Component>

const adminNavigationItems: AdminNavigationItem[] = adminNavigationRouteDefs.map((item) => ({
  ...item,
  icon: adminNavigationIcons[item.key],
}))

export const adminNavigationByKey = adminNavigationItems.reduce(
  (acc, item) => {
    acc[item.key] = item
    return acc
  },
  {} as Record<AdminNavigationKey, AdminNavigationItem>,
)

export type AdminNavigationGroup = {
  title: string
  keys: AdminNavigationKey[]
}

export const adminNavigationGroups: AdminNavigationGroup[] = [
  { title: '业务监控', keys: ['statistics', 'monitor'] },
  { title: '运营管理', keys: ['users', 'credits'] },
  { title: '系统配置', keys: ['models', 'feature-prompts', 'settings'] },
]

export function findAdminNavigationItemByPath(path: string) {
  return adminNavigationItems.find((item) => isPathUnderRoute(path, item.path))
}

export function resolveAdminPageTitle(path: string) {
  return findAdminNavigationItemByPath(path)?.title ?? '管理后台'
}
