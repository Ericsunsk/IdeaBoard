/**
 * Auth feature public entrypoint (issue #685).
 *
 * External consumers import auth stores and modal components through this
 * barrel. Internal modules stay on relative imports.
 */
export { useAuthSession, useUserStore } from './session/userSession'
export type { UserSessionStore } from './session/userSession'
export { authTransport } from './session/transport'
export { createAuthOverlay, provideAuthOverlay, useAuthOverlay } from './overlay'
export type { AuthOverlay, AuthOverlayMode } from './overlay'
export { useAuthFeedback } from './useAuthFeedback'
export type { AuthFeedback } from './useAuthFeedback'
export { default as UserAuthModals } from './components/UserAuthModals.vue'
export { default as AuthModalHost } from './components/AuthModalHost.vue'
export { default as LoginAuthPanel } from './components/LoginAuthPanel.vue'
export { default as RegisterAuthPanel } from './components/RegisterAuthPanel.vue'

/**
 * Lazily loads the OAuth callback page through the auth entrypoint so route
 * wiring keeps its code-split chunk instead of importing `pages/` directly.
 * `session/transport` stays dependency-free of this barrel, so re-exporting
 * `authTransport` above cannot close a module cycle.
 */
export const loadUserAuthCallbackPage = () => import('./pages/UserAuthCallbackPage.vue')
