import type { RouteRecordRaw } from 'vue-router'
import {
  ACCEPTABLE_USE_PATH,
  PRIVACY_POLICY_PATH,
  REFUND_POLICY_PATH,
  TERMS_PATH,
} from '../../legal/routePaths'
import {
  SHARE_JOIN_PATH,
  USER_AUTH_CALLBACK_PATH,
  USER_BOARDS_PATH,
  USER_ROOT_PATH,
} from '../routePaths'

export const WORKSPACE_DEFAULT_PATH = USER_BOARDS_PATH

export const workspaceRoutes: RouteRecordRaw[] = [
  {
    path: TERMS_PATH,
    name: 'Terms',
    component: () => import('../../legal/pages/TermsPage.vue'),
    meta: { public: true, seoKey: 'terms' },
  },
  {
    path: REFUND_POLICY_PATH,
    name: 'RefundPolicy',
    component: () => import('../../legal/pages/RefundPolicyPage.vue'),
    meta: { public: true, seoKey: 'refund' },
  },
  {
    path: PRIVACY_POLICY_PATH,
    name: 'PrivacyPolicy',
    component: () => import('../../legal/pages/PrivacyPolicyPage.vue'),
    meta: { public: true, seoKey: 'privacy' },
  },
  {
    path: ACCEPTABLE_USE_PATH,
    name: 'AcceptableUse',
    component: () => import('../../legal/pages/AcceptableUsePage.vue'),
    meta: { public: true, seoKey: 'acceptable-use' },
  },
  {
    path: SHARE_JOIN_PATH,
    name: 'ShareJoin',
    component: () => import('../pages/WorkspaceShareJoinPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: USER_ROOT_PATH,
    component: () => import('../layout/WorkspaceLayout.vue'),
    children: [
      {
        path: 'login',
        name: 'UserLogin',
        component: () => import('../../canvas/presentation/pages/CanvasLandingPage.vue'),
        meta: { guest: true },
      },
      {
        path: 'register',
        name: 'UserRegister',
        component: () => import('../../canvas/presentation/pages/CanvasLandingPage.vue'),
        meta: { guest: true },
      },
      {
        path: USER_AUTH_CALLBACK_PATH.slice(USER_ROOT_PATH.length + 1),
        name: 'UserAuthCallback',
        component: () => import('../../auth').then((module) => module.loadUserAuthCallbackPage()),
        meta: { public: true },
      },
      {
        path: 'boards',
        name: 'UserBoards',
        component: () => import('../../board').then((module) => module.BoardsPage),
        meta: { requiresAuth: true },
      },
      {
        path: 'account',
        redirect: { path: USER_BOARDS_PATH, query: { accountTab: 'profile' } },
        meta: { requiresAuth: true },
      },
      {
        path: 'settings',
        redirect: { path: USER_BOARDS_PATH, query: { accountTab: 'security' } },
        meta: { requiresAuth: true },
      },
      {
        path: 'billing',
        redirect: { path: USER_BOARDS_PATH, query: { accountTab: 'billing' } },
        meta: { requiresAuth: true },
      },
      {
        path: 'recharge',
        redirect: { path: USER_BOARDS_PATH, query: { accountTab: 'recharge' } },
        meta: { requiresAuth: true },
      },
      {
        path: 'search',
        name: 'UserSearch',
        component: () => import('../../search').then((module) => module.loadSearchPage()),
        meta: { public: true },
      },
      {
        path: 'canvas',
        name: 'UserCanvas',
        component: () => import('../../canvas/presentation/pages/CanvasPage.vue'),
        meta: { requiresAuth: true },
      },
    ],
  },
]
