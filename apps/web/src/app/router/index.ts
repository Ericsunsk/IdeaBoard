import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useUserStore } from '../../features/auth/session/userSession'
import { dismissAppToasts, useAppToast } from '../../shared/composables/useAppToast'
import { i18n } from '../../shared/i18n'
import { ADMIN_DEFAULT_PATH } from '../../features/admin/app/navigationMeta'
import { adminRoutes } from '../../features/admin/app/routes'
import {
  WORKSPACE_DEFAULT_PATH,
  workspaceRoutes,
} from '../../features/workspace/routes/workspaceRoutes'
import { ADMIN_ROOT_PATH, HOME_PATH, isPathUnderRoute } from '../../shared/constants/routePaths'
import { buildLoginRedirectPath } from '../../shared/auth/sessionPolicy'
import { applyRouteSeo } from '../../shared/seo/routeSeo'
import {
  readSessionStorageItem,
  removeSessionStorageItem,
  writeSessionStorageItem,
} from '../../shared/utils/storage'
import { CHUNK_RELOAD_GUARD_KEY, isChunkLoadFailure, resolveScrollPosition } from './routeRecovery'

const routes: RouteRecordRaw[] = [
  {
    path: HOME_PATH,
    name: 'Home',
    component: () => import('../../features/canvas/presentation/pages/CanvasLandingPage.vue'),
    meta: { public: true, seoKey: 'home' },
  },
  ...workspaceRoutes,
  ...adminRoutes,
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior: (to, from, savedPosition) => resolveScrollPosition(to, from, savedPosition),
})

// Deploy-time hazard: the client holds an old chunk manifest and a lazy
// route 404s. Recover with a single guarded reload; a second failure degrades
// to a toast instead of a reload loop.
router.onError((error) => {
  if (!isChunkLoadFailure(error)) return
  if (readSessionStorageItem(CHUNK_RELOAD_GUARD_KEY)) {
    useAppToast().error(i18n.global.t('common.errors.unexpected'))
    return
  }
  if (!writeSessionStorageItem(CHUNK_RELOAD_GUARD_KEY, '1')) {
    useAppToast().error(i18n.global.t('common.errors.unexpected'))
    return
  }
  window.location.reload()
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
  // A clean navigation proves the chunk manifest is fresh again.
  // Removal failure keeps the guard inert-safe (single reload already spent).
  removeSessionStorageItem(CHUNK_RELOAD_GUARD_KEY)
})

export default router
