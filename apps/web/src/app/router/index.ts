import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useUserStore } from '../../features/auth/session/authSession'
import { dismissAppToasts } from '../../shared/composables/useAppToast'
import { ADMIN_DEFAULT_PATH } from '../../features/admin/app/adminNavigationMeta'
import { adminRoutes } from '../../features/admin/app/adminRoutes'
import { USER_DEFAULT_PATH, userRoutes } from '../../features/user/app/userRoutes'
import { ADMIN_ROOT_PATH, isPathUnderRoute } from '../../shared/constants/routePaths'
import { buildLoginRedirectPath } from '../../shared/auth/sessionPolicy'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: USER_DEFAULT_PATH,
  },
  ...userRoutes,
  ...adminRoutes,
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to) => {
  const userStore = useUserStore()
  const isAdminRoute = isPathUnderRoute(to.path, ADMIN_ROOT_PATH)
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
    return buildLoginRedirectPath(to.path, to.fullPath)
  }

  if (to.meta.guest && isLoggedIn) {
    if (isAdminRoute) {
      return user?.role === 'admin' ? ADMIN_DEFAULT_PATH : USER_DEFAULT_PATH
    } else {
      return USER_DEFAULT_PATH
    }
  }

  if (isAdminRoute && user && user.role !== 'admin') {
    return USER_DEFAULT_PATH
  }

  return true
})

router.afterEach((to, from) => {
  if (to.fullPath !== from.fullPath) {
    dismissAppToasts()
  }
})

export default router
