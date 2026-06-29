import type { RouteRecordRaw } from 'vue-router'
import { adminNavigationRouteDefs, ADMIN_DEFAULT_PATH } from './adminNavigationMeta'
import type { AdminNavigationKey } from './adminNavigationMeta'
import { ADMIN_ROOT_PATH } from '../../../shared/constants/routePaths'

const adminPageComponents = {
  users: () => import('../users/AdminUsersPage.vue'),
  channels: () => import('../pages/channels/AdminChannelsPage.vue'),
  statistics: () => import('../pages/statistics/AdminStatisticsPage.vue'),
  'task-monitor': () => import('../pages/task-monitor/AdminTaskMonitorPage.vue'),
  'feature-prompts': () => import('../pages/prompt-config/AdminPromptConfigPage.vue'),
  'system-settings': () => import('../pages/system-settings/AdminSystemSettingsPage.vue'),
  'oss-config': () => import('../pages/storage/AdminStoragePage.vue'),
} satisfies Record<AdminNavigationKey, RouteRecordRaw['component']>

export const adminRoutes: RouteRecordRaw[] = [
  {
    path: ADMIN_ROOT_PATH,
    component: () => import('./AdminLayout.vue'),
    children: [
      {
        path: '',
        redirect: ADMIN_DEFAULT_PATH,
      },
      {
        path: 'login',
        name: 'AdminLogin',
        component: () => import('../pages/login/AdminLoginPage.vue'),
        meta: { guest: true },
      },
      ...adminNavigationRouteDefs.map((item) => ({
        path: item.childPath,
        name: item.routeName,
        component: adminPageComponents[item.key],
        meta: { requiresAuth: true, title: item.title },
      })),
    ],
  },
]
