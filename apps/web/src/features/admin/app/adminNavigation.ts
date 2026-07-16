import type { Component } from 'vue'
import {
  ChatBubble,
  DashboardSpeed,
  Database,
  Group,
  Link,
  Settings,
  StatsDownSquare,
  Wallet,
} from '@iconoir/vue'
import {
  ADMIN_DEFAULT_PATH,
  ADMIN_LOGIN_PATH,
  adminNavigationRouteDefs,
} from './adminNavigationMeta'
import { isPathUnderRoute } from '../../../shared/constants/routePaths'
import type { AdminNavigationKey, AdminNavigationRouteDef } from './adminNavigationMeta'

export { ADMIN_DEFAULT_PATH, ADMIN_LOGIN_PATH }
export type { AdminNavigationKey }

export type AdminNavigationItem = AdminNavigationRouteDef & {
  icon: Component
}

const adminNavigationIcons = {
  users: Group,
  credits: Wallet,
  'provider-endpoints': Link,
  statistics: DashboardSpeed,
  'task-monitor': StatsDownSquare,
  'feature-prompts': ChatBubble,
  'system-settings': Settings,
  'oss-config': Database,
} satisfies Record<AdminNavigationKey, Component>

export const adminNavigationItems: AdminNavigationItem[] = adminNavigationRouteDefs.map((item) => ({
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

export function findAdminNavigationItemByPath(path: string) {
  return adminNavigationItems.find((item) => isPathUnderRoute(path, item.path))
}

export function resolveAdminPageTitle(path: string, routeTitle: unknown) {
  if (typeof routeTitle === 'string' && routeTitle.trim()) return routeTitle.trim()
  return findAdminNavigationItemByPath(path)?.title ?? '管理后台'
}
