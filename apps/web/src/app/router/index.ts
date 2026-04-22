import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { userApi } from '../../shared/api'
import { getStoredToken, getStoredUser, clearStoredAuth, setStoredAuth } from '../../shared/utils/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/user/home'
  },
  {
    path: '/user/login',
    name: 'UserLogin',
    component: () => import('../../pages/user/UserAuthPage.vue'),
    props: { mode: 'login' },
    meta: { guest: true }
  },
  {
    path: '/user/register',
    name: 'UserRegister',
    component: () => import('../../pages/user/UserAuthPage.vue'),
    props: { mode: 'register' },
    meta: { guest: true }
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
        path: 'picture',
        name: 'UserPicture',
        component: () => import('../../pages/user/UserPicturePage.vue'),
        meta: { requiresAuth: true },
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
        meta: { requiresAuth: true }
      }
    ]
  },
  {
    path: '/admin',
    component: () => import('../../pages/admin/AdminLayoutPage.vue'),
    children: [
      {
        path: '',
        redirect: '/admin/users'
      },
      {
        path: 'login',
        name: 'AdminLogin',
        component: () => import('../../pages/admin/AdminLoginPage.vue'),
        meta: { guest: true }
      },
      {
        path: 'users',
        name: 'AdminUsers',
        component: () => import('../../pages/admin/AdminUsersPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'accounts',
        name: 'AdminAccounts',
        component: () => import('../../pages/admin/AdminAccountsPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'monitor',
        name: 'AdminMonitor',
        component: () => import('../../pages/admin/AdminMonitorPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'style-cards',
        name: 'AdminStyleCards',
        component: () => import('../../pages/admin/AdminStyleCardsPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'drawing-records',
        name: 'AdminDrawingRecords',
        component: () => import('../../pages/admin/AdminDrawingRecordsPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'channels',
        name: 'AdminChannels',
        component: () => import('../../pages/admin/AdminChannelsPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'oss-config',
        name: 'AdminOSSConfig',
        component: () => import('../../pages/admin/AdminStoragePage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'seg-config',
        name: 'AdminSegConfig',
        component: () => import('../../pages/admin/AdminImageProcessingPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'face-library',
        name: 'AdminFaceLibrary',
        component: () => import('../../pages/admin/AdminFaceLibraryPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'bg-library',
        name: 'AdminBgLibrary',
        component: () => import('../../pages/admin/AdminBgLibraryPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'pattern-style-library',
        name: 'AdminPatternStyleLibrary',
        component: () => import('../../pages/admin/AdminPatternStyleLibraryPage.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'feature-prompts',
        name: 'AdminFeaturePrompts',
        component: () => import('../../pages/admin/AdminFeaturePromptsPage.vue'),
        meta: { requiresAuth: true }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach(async (to) => {
  let token = getStoredToken()
  let user = getStoredUser()
  const isAdminRoute = to.path.startsWith('/admin')

  if (token && !user) {
    clearStoredAuth()
    token = null
    user = null
  }

  if (token && (isAdminRoute || !!to.meta.requiresAuth)) {
    try {
      const res: any = await userApi.getInfo()
      if (res.code === 0 && res.data?.user) {
        const refreshedUser = res.data.user
        user = refreshedUser
        setStoredAuth(token, refreshedUser)
      }
    } catch (error: any) {
      const code = Number(error?.status ?? error?.code)
      if (code === 401 || code === 403) {
        clearStoredAuth()
        token = null
        user = null
      } else if (isAdminRoute) {
        return '/user/home'
      }
    }
  }

  const isLoggedIn = !!(token && user)

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
