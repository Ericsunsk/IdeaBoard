import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { userApi } from '../api'
import { getStoredToken, getStoredUser, clearStoredAuth, setStoredAuth } from '../utils/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/user/home'
  },
  {
    path: '/user/login',
    name: 'UserLogin',
    component: () => import('../views/user/Auth.vue'),
    props: { mode: 'login' },
    meta: { guest: true }
  },
  {
    path: '/user/register',
    name: 'UserRegister',
    component: () => import('../views/user/Auth.vue'),
    props: { mode: 'register' },
    meta: { guest: true }
  },
  {
    path: '/user',
    component: () => import('../views/user/Layout.vue'),
    children: [
      {
        path: 'home',
        name: 'UserHome',
        component: () => import('../views/user/HomeBlank.vue'),
      },
      {
        path: 'picture',
        name: 'UserPicture',
        component: () => import('../views/user/Home.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'boards',
        name: 'UserBoards',
        component: () => import('../views/user/CanvasBoards.vue'),
        meta: { requiresAuth: true },
      },
      {
        path: 'center',
        name: 'UserCenter',
        component: () => import('../views/user/Center.vue'),
        meta: { requiresAuth: true }
      }
    ]
  },
  {
    path: '/admin',
    component: () => import('../views/admin/Layout.vue'),
    children: [
      {
        path: '',
        redirect: '/admin/users'
      },
      {
        path: 'login',
        name: 'AdminLogin',
        component: () => import('../views/admin/Login.vue'),
        meta: { guest: true }
      },
      {
        path: 'users',
        name: 'AdminUsers',
        component: () => import('../views/admin/Users.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'accounts',
        name: 'AdminAccounts',
        component: () => import('../views/admin/Accounts.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'monitor',
        name: 'AdminMonitor',
        component: () => import('../views/admin/Monitor.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'style-cards',
        name: 'AdminStyleCards',
        component: () => import('../views/admin/StyleCards.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'drawing-records',
        name: 'AdminDrawingRecords',
        component: () => import('../views/admin/DrawingRecords.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'channels',
        name: 'AdminChannels',
        component: () => import('../views/admin/Channels.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'oss-config',
        name: 'AdminOSSConfig',
        component: () => import('../views/admin/OSSConfig.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'seg-config',
        name: 'AdminSegConfig',
        component: () => import('../views/admin/SegConfig.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'face-library',
        name: 'AdminFaceLibrary',
        component: () => import('../views/admin/FaceLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'bg-library',
        name: 'AdminBgLibrary',
        component: () => import('../views/admin/BgLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'pattern-style-library',
        name: 'AdminPatternStyleLibrary',
        component: () => import('../views/admin/PatternStyleLibrary.vue'),
        meta: { requiresAuth: true }
      },
      {
        path: 'feature-prompts',
        name: 'AdminFeaturePrompts',
        component: () => import('../views/admin/FeaturePromptConfig.vue'),
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
