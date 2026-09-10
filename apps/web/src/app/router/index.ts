import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useUserStore } from '../../features/auth/session/userSession'
import { dismissAppToasts } from '../../shared/composables/useAppToast'
import { ADMIN_DEFAULT_PATH } from '../../features/admin/app/navigationMeta'
import { adminRoutes } from '../../features/admin/app/routes'
import {
  WORKSPACE_DEFAULT_PATH,
  workspaceRoutes,
} from '../../features/workspace/routes/workspaceRoutes'
import { ADMIN_ROOT_PATH, HOME_PATH, isPathUnderRoute } from '../../shared/constants/routePaths'
import { buildLoginRedirectPath } from '../../shared/auth/sessionPolicy'
import { applyRouteSeo } from '../../shared/seo/routeSeo'

const routes: RouteRecordRaw[] = [
  {
    path: HOME_PATH,
    name: 'Home',
    component: () => import('../../features/canvas/pages/CanvasLandingPage.vue'),
    meta: { public: true, seoKey: 'home' },
  },
  ...workspaceRoutes,
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
      return user?.role === 'admin' ? ADMIN_DEFAULT_PATH : WORKSPACE_DEFAULT_PATH
    } else {
      return WORKSPACE_DEFAULT_PATH
    }
  }

  if (isAdminRoute && user && user.role !== 'admin') {
    return WORKSPACE_DEFAULT_PATH
  }

  return true
})

router.afterEach((to, from) => {
  applyRouteSeo(to)
  if (to.fullPath !== from.fullPath) {
    dismissAppToasts()
  }
})

export default router
