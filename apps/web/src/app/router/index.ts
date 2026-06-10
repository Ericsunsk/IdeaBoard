import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useUserStore } from '../stores/user'
import { dismissAppToasts } from '../../shared/composables/useAppToast'
import { adminRoutes } from '../../features/admin/app/adminRoutes'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/user/home',
  },
  {
    path: '/share',
    name: 'ShareJoin',
    component: () => import('../../features/user/pages/UserShareJoinPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/user',
    component: () => import('../../features/user/pages/UserLayoutPage.vue'),
    children: [
      {
        path: 'home',
        name: 'UserHome',
        component: () => import('../../features/user/pages/UserHomePage.vue'),
      },
      {
        path: 'login',
        name: 'UserLogin',
        component: () => import('../../features/user/pages/UserHomePage.vue'),
        meta: { guest: true },
      },
      {
        path: 'register',
        name: 'UserRegister',
        component: () => import('../../features/user/pages/UserHomePage.vue'),
        meta: { guest: true },
      },
      {
        path: 'boards',
        name: 'UserBoards',
        component: () => import('../../features/user/pages/UserBoardsPage.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'center',
        name: 'UserCenter',
        component: () => import('../../features/user/pages/UserCenterPage.vue'),
        meta: { requiresAuth: true },
      },
    ],
  },
  {
    path: '/user/workspace',
    name: 'UserWorkspace',
    component: () => import('../../features/user/pages/UserWorkspacePage.vue'),
    meta: { requiresAuth: true },
  },
  ...adminRoutes,
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const userStore = useUserStore()
  const isAdminRoute = to.path.startsWith('/admin')
  // Only force-refresh session when >25 s have elapsed since last validation;
  // this honours the 30 s TTL instead of bypassing it on every navigation.
  const SESSION_FRESH_THRESHOLD_MS = 25_000
  const needsFreshSession =
    isAdminRoute && Date.now() - userStore.sessionValidatedAt > SESSION_FRESH_THRESHOLD_MS
  const shouldRefreshSession = needsFreshSession || !!to.meta.requiresAuth || !!to.meta.guest
  const user = shouldRefreshSession
    ? await userStore.ensureSession({ fresh: needsFreshSession })
    : userStore.user

  const isLoggedIn = !!user

  if (to.meta.requiresAuth && !isLoggedIn) {
    if (isAdminRoute) {
      return { path: '/admin/login', query: { redirect: to.fullPath } }
    } else {
      return { path: '/user/login', query: { redirect: to.fullPath } }
    }
  }

  if (to.meta.guest && isLoggedIn) {
    if (isAdminRoute) {
      return user?.role === 'admin' ? '/admin/users' : '/user/home'
    } else {
      return '/user/home'
    }
  }

  if (isAdminRoute && user && user.role !== 'admin') {
    return '/user/home'
  }

  return true
})

router.afterEach((to, from) => {
  if (to.fullPath !== from.fullPath) {
    dismissAppToasts()
  }
})

export default router
