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
    component: () => import('../../pages/user/UserShareJoinPage.vue'),
    meta: { requiresAuth: true },
  },
  {
    path: '/user',
    component: () => import('../../pages/user/UserLayoutPage.vue'),
    children: [
      {
        path: 'home',
        name: 'UserHome',
        component: () => import('../../pages/user/UserHomePage.vue'),
      },
      {
        path: 'login',
        name: 'UserLogin',
        component: () => import('../../pages/user/UserHomePage.vue'),
        meta: { guest: true },
      },
      {
        path: 'register',
        name: 'UserRegister',
        component: () => import('../../pages/user/UserHomePage.vue'),
        meta: { guest: true },
      },
      {
        path: 'boards',
        name: 'UserBoards',
        component: () => import('../../pages/user/UserBoardsPage.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'center',
        name: 'UserCenter',
        component: () => import('../../pages/user/UserCenterPage.vue'),
        meta: { requiresAuth: true },
      },
    ],
  },
  {
    path: '/user/workspace',
    name: 'UserWorkspace',
    component: () => import('../../pages/user/UserWorkspacePage.vue'),
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
  const shouldRefreshSession = isAdminRoute || !!to.meta.requiresAuth || !!to.meta.guest
  const user = shouldRefreshSession
    ? await userStore.ensureSession({ fresh: isAdminRoute })
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
