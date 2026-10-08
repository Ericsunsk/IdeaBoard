import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useUserStore } from '../../features/auth'
import { dismissAppToasts, useAppToast } from '../../shared/composables/useAppToast'
import { i18n } from '../../shared/i18n'
import { adminRoutes } from '../../features/admin/app/routes'
import { workspaceRoutes } from '../../features/workspace/routes/workspaceRoutes'
import { ADMIN_ROOT_PATH, HOME_PATH, isPathUnderRoute } from '../../shared/constants/routePaths'
import { applyRouteSeo } from '../../shared/seo/routeSeo'
import {
  readSessionStorageItem,
  removeSessionStorageItem,
  writeSessionStorageItem,
} from '../../shared/utils/storage'
import { CHUNK_RELOAD_GUARD_KEY, isChunkLoadFailure, resolveScrollPosition } from './routeRecovery'
import { resolveAuthGateRedirect, resolveAuthSessionRefresh } from './authGate'

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
  const { shouldRefresh, needsFresh } = resolveAuthSessionRefresh({
    isAdminRoute,
    requiresAuth: to.meta.requiresAuth,
    guest: to.meta.guest,
    validatedAt: userStore.sessionValidatedAt,
  })
  const user = shouldRefresh ? await userStore.ensureSession({ fresh: needsFresh }) : userStore.user

  return resolveAuthGateRedirect({
    isAdminRoute,
    requiresAuth: to.meta.requiresAuth,
    guest: to.meta.guest,
    user,
    currentPath: to.path,
    currentFullPath: to.fullPath,
  })
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
