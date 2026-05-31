import type { RouteRecordRaw } from 'vue-router'
import { adminNavigationItems, ADMIN_DEFAULT_PATH } from './adminNavigation'

const adminPageComponents = {
  users: () => import('../pages/users/AdminUsersPage.vue'),
  channels: () => import('../pages/channels/AdminChannelsPage.vue'),
  statistics: () => import('../pages/statistics/AdminStatisticsPage.vue'),
  'task-monitor': () => import('../pages/task-monitor/AdminTaskMonitorPage.vue'),
  'feature-prompts': () => import('../pages/prompt-config/AdminPromptConfigPage.vue'),
  'oss-config': () => import('../pages/storage/AdminStoragePage.vue'),
} satisfies Record<string, RouteRecordRaw['component']>

export const adminRoutes: RouteRecordRaw[] = [
  {
    path: '/admin',
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
      ...adminNavigationItems.map((item) => ({
        path: item.childPath,
        name: item.routeName,
        component: adminPageComponents[item.key],
        meta: { requiresAuth: true, title: item.title },
      })),
    ],
  },
]
