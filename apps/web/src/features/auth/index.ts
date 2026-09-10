/**
 * Auth feature public entrypoint (issue #685).
 *
 * External consumers import auth stores and modal components through this
 * barrel. Internal modules stay on relative imports.
 */
export { useAuthSession, useUserStore } from './session/userSession'
export type { UserSessionStore } from './session/userSession'
export { useAuthFeedback } from './useAuthFeedback'
export type { AuthFeedback } from './useAuthFeedback'
export { default as UserAuthModals } from './components/UserAuthModals.vue'
export { default as AuthModalHost } from './components/AuthModalHost.vue'
export { default as LoginAuthPanel } from './components/LoginAuthPanel.vue'
export { default as RegisterAuthPanel } from './components/RegisterAuthPanel.vue'
