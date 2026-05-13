import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { userApi } from '../../shared/api/user'
import { getStoredUser, clearStoredAuth, setStoredAuth } from '../../shared/utils/auth'

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
  {
    path: '/admin',
    component: () => import('../../pages/admin/AdminLayoutPage.vue'),
    children: [
      {
        path: '',
        redirect: '/admin/users',
      },
      {
        path: 'login',
        name: 'AdminLogin',
        component: () => import('../../pages/admin/AdminLoginPage.vue'),
        meta: { guest: true },
      },
      {
        path: 'users',
        name: 'AdminUsers',
        component: () => import('../../pages/admin/AdminUsersPage.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'statistics',
        name: 'AdminStatistics',
        component: () => import('../../pages/admin/AdminUsageStatisticsPage.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'channels',
        name: 'AdminChannels',
        component: () => import('../../pages/admin/AdminChannelsPage.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'oss-config',
        name: 'AdminOSSConfig',
        component: () => import('../../pages/admin/AdminStoragePage.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'feature-prompts',
        name: 'AdminFeaturePrompts',
        component: () => import('../../pages/admin/AdminFeaturePromptsPage.vue'),
        meta: { requiresAuth: true },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Session cache: avoid hitting /api/user/info on every navigation.
// Stale-while-revalidate: use cached value for 30s, then refresh in background.
let sessionCacheUser: any = null
let sessionCacheAt = 0
const SESSION_CACHE_TTL_MS = 30_000

router.beforeEach(async (to) => {
  let user = getStoredUser()
  const isAdminRoute = to.path.startsWith('/admin')
  const requiresFreshAuth = !!to.meta.requiresAuth || !!to.meta.guest
  const shouldRefreshSession = isAdminRoute || !!to.meta.requiresAuth || !!to.meta.guest
  const hadStoredUser = !!user

  if (shouldRefreshSession) {
    const now = Date.now()
    // Admin routes always bypass the cache to ensure fresh auth state.
    const cacheValid =
      !isAdminRoute && sessionCacheUser !== null && now - sessionCacheAt < SESSION_CACHE_TTL_MS
    if (cacheValid) {
      user = sessionCacheUser
    } else {
      try {
        const res: any = await userApi.getInfo()
        if (res.code === 0 && res.data?.user) {
          const refreshedUser = res.data.user
          user = refreshedUser
          sessionCacheUser = refreshedUser
          sessionCacheAt = now
          setStoredAuth(refreshedUser)
        }
      } catch (error: any) {
        sessionCacheUser = null
        const code = Number(error?.status ?? error?.code)
        if (code === 401 || code === 403) {
          clearStoredAuth()
          user = null
        } else {
          if (requiresFreshAuth && !hadStoredUser) {
            clearStoredAuth()
            user = null
          }
          if (isAdminRoute && !hadStoredUser) {
            return '/user/home'
          }
        }
      }
    }
  }

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

export default router
