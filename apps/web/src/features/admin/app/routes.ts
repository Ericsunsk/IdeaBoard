import type { RouteRecordRaw } from 'vue-router'
import { adminNavigationRouteDefs, ADMIN_DEFAULT_PATH } from './navigationMeta'
import type { AdminNavigationKey } from './navigationMeta'
import {
  ADMIN_CREDIT_PRICING_PATH,
  ADMIN_ROOT_PATH,
  ADMIN_SETTINGS_GENERAL_PATH,
} from '../../../shared/constants/routePaths'

const adminPageComponents = {
  users: () => import('../users/AdminUsersPage.vue'),
  credits: () => import('../credits/presentation/AdminCreditsBillingLayout.vue'),
  statistics: () => import('../statistics/pages/AdminStatisticsPage.vue'),
  'monitor': () => import('../monitor/presentation/pages/AdminMonitorPage.vue'),
  models: () => import('../models/presentation/pages/AdminModelsPage.vue'),
  'feature-prompts': () => import('../prompts/pages/AdminPromptConfigPage.vue'),
  'settings': () => import('../settings/AdminSettingsLayout.vue'),
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
      ...adminNavigationRouteDefs
        .filter((item) => item.key !== 'credits' && item.key !== 'settings')
        .map((item) => ({
          path: item.childPath,
          name: item.routeName,
          component: adminPageComponents[item.key],
          meta: { requiresAuth: true, title: item.title },
        })),
      {
        path: 'credits',
        component: adminPageComponents.credits,
        meta: { requiresAuth: true, title: '积分运营', hidePageHeader: true },
        children: [
          { path: '', redirect: ADMIN_CREDIT_PRICING_PATH },
          {
            path: 'pricing',
            name: 'AdminCreditPricing',
            component: () => import('../credits/pricing/AdminBillingPricingPage.vue'),
            meta: { requiresAuth: true, title: '功能定价' },
          },
          {
            path: 'health',
            name: 'AdminCreditHealth',
            component: () => import('../credits/health/AdminCreditHealthPage.vue'),
            meta: { requiresAuth: true, title: '账本健康' },
          },
          {
            path: 'reward-codes',
            name: 'AdminRewardCodes',
            component: () => import('../credits/rewards/AdminRewardCodesPage.vue'),
          },
          {
            path: 'payment-orders',
            name: 'AdminPaymentOrders',
            component: () => import('../payments/orders/AdminPaymentOrdersPage.vue'),
            meta: { requiresAuth: true, title: '支付订单' },
          },
        ],
      },
      {
        path: 'settings',
        component: adminPageComponents['settings'],
        meta: { requiresAuth: true, title: '系统设置', hidePageHeader: true },
        children: [
          { path: '', redirect: ADMIN_SETTINGS_GENERAL_PATH },
          {
            path: 'general',
            name: 'AdminSettingsGeneral',
            component: () => import('../settings/pages/AdminSettingsGeneralPage.vue'),
            meta: { requiresAuth: true, title: '网站设置' },
          },
          {
            path: 'storage',
            name: 'AdminSettingsStorage',
            component: () => import('../settings/pages/AdminSettingsStoragePage.vue'),
            meta: { requiresAuth: true, title: '存储配置' },
          },
          {
            path: 'mail',
            name: 'AdminSettingsMail',
            component: () => import('../settings/pages/AdminSettingsMailPage.vue'),
            meta: { requiresAuth: true, title: '邮件服务' },
          },
          {
            path: 'security',
            name: 'AdminSettingsSecurity',
            component: () => import('../settings/pages/AdminSettingsSecurityPage.vue'),
            meta: { requiresAuth: true, title: '安全与网络' },
          },
        ],
      },
    ],
  },
]
