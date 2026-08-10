import type { RouteRecordRaw } from 'vue-router'
import {
  ACCEPTABLE_USE_PATH,
  PRIVACY_POLICY_PATH,
  REFUND_POLICY_PATH,
  SHARE_JOIN_PATH,
  USER_AUTH_CALLBACK_PATH,
  TERMS_PATH,
  USER_HOME_PATH,
  USER_LOGIN_PATH as USER_LOGIN_ROUTE_PATH,
  USER_ROOT_PATH,
} from '../../../shared/constants/routePaths'

export const USER_DEFAULT_PATH = USER_HOME_PATH
export const USER_LOGIN_PATH = USER_LOGIN_ROUTE_PATH

export const userRoutes: RouteRecordRaw[] = [
  {
    path: TERMS_PATH,
    name: 'Terms',
    component: () => import('../../legal/pages/TermsPage.vue'),
    meta: { public: true },
  },
  {
    path: REFUND_POLICY_PATH,
    name: 'RefundPolicy',
    component: () => import('../../legal/pages/RefundPolicyPage.vue'),
    meta: { public: true },
  },
  {
    path: PRIVACY_POLICY_PATH,
    name: 'PrivacyPolicy',
    component: () => import('../../legal/pages/PrivacyPolicyPage.vue'),
    meta: { public: true },
  },
  {
    path: ACCEPTABLE_USE_PATH,
    name: 'AcceptableUse',
    component: () => import('../../legal/pages/AcceptableUsePage.vue'),
    meta: { public: true },
  },
  {
    path: SHARE_JOIN_PATH,
    name: 'ShareJoin',
    component: () => import('../pages/UserShareJoinPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: USER_ROOT_PATH,
    component: () => import('../layout/UserLayout.vue'),
    children: [
      {
        path: 'home',
        name: 'UserHome',
        component: () => import('../../canvas/pages/CanvasLandingPage.vue'),
      },
      {
        path: 'login',
        name: 'UserLogin',
        component: () => import('../../canvas/pages/CanvasLandingPage.vue'),
        meta: { guest: true },
      },
      {
        path: 'register',
        name: 'UserRegister',
        component: () => import('../../canvas/pages/CanvasLandingPage.vue'),
        meta: { guest: true },
      },
      {
        path: USER_AUTH_CALLBACK_PATH.slice(USER_ROOT_PATH.length + 1),
        name: 'UserAuthCallback',
        component: () => import('../../auth/pages/UserAuthCallbackPage.vue'),
        meta: { public: true },
      },
      {
        path: 'boards',
        name: 'UserBoards',
        component: () => import('../../board-catalog/pages/BoardsPage.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'canvas',
        name: 'UserCanvas',
        component: () => import('../../canvas/pages/CanvasPage.vue'),
        meta: { requiresAuth: true },
      },
    ],
  },
]
