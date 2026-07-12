import type { RouteRecordRaw } from 'vue-router'
import { adminNavigationRouteDefs, ADMIN_DEFAULT_PATH } from './adminNavigationMeta'
import type { AdminNavigationKey } from './adminNavigationMeta'
import { ADMIN_ROOT_PATH } from '../../../shared/constants/routePaths'
import { ADMIN_CREDIT_WALLETS_PATH } from '../../../shared/constants/routePaths'

const adminPageComponents = {
  users: () => import('../users/AdminUsersPage.vue'),
  credits: () => import('../credits-billing/AdminCreditsBillingLayout.vue'),
  'provider-endpoints': () => import('../pages/provider-endpoints/AdminProviderEndpointsPage.vue'),
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
      ...adminNavigationRouteDefs.filter((item) => item.key !== 'credits').map((item) => ({
        path: item.childPath,
        name: item.routeName,
        component: adminPageComponents[item.key],
        meta: { requiresAuth: true, title: item.title },
      })),
      {
        path: 'credits',
        component: adminPageComponents.credits,
        meta: { requiresAuth: true, title: '点子与计费' },
        children: [
          { path: '', redirect: ADMIN_CREDIT_WALLETS_PATH },
          {
            path: 'wallets',
            name: 'AdminCreditWallets',
            component: () =>
              import('../credits-billing/wallets/AdminCreditWalletsPage.vue'),
          },
          {
            path: 'pricing',
            name: 'AdminCreditPricing',
            component: () =>
              import('../credits-billing/pricing/AdminBillingPricingPage.vue'),
          },
          {
            path: 'reconciliation',
            name: 'AdminCreditReconciliation',
            component: () =>
              import('../credits-billing/reconciliation/AdminCreditReconciliationPage.vue'),
          },
        ],
      },
    ],
  },
]
