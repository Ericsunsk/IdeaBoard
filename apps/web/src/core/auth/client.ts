import { createAuthClient } from 'better-auth/vue'
import { emailOTPClient } from 'better-auth/client/plugins'
import { AUTH_BASE_URL } from '../config/runtimeUrls'
import { userFacingErrorCopy } from '../errors/errorCatalog'

// `adminClient()` is deliberately NOT registered. The browser's only need for
// the Better Auth admin plugin is ending an impersonation, which
// `authTransport.stopImpersonating` now calls directly. Leaving `adminClient()`
// in the bundle would hand every future contributor a typed, silently working
// `authClient.admin.removeUser()` / `.setRole()` / `.banUser()` affordance that
// mutates Auth state without the Go Control Plane orchestration DESIGN.md
// requires for cross-database high-risk identity actions. The Auth Service
// additionally denies those permissions at runtime, and the reverse proxy
// rejects the whole `/api/auth/admin/` surface except stop-impersonating, so
// this is defence in depth rather than the primary guard.
export const authClient = createAuthClient({
  baseURL: AUTH_BASE_URL,
  plugins: [emailOTPClient()],
  fetchOptions: {
    credentials: 'include',
  },
})

export function resolveBetterAuthErrorMessage(error: unknown, fallback: string): string {
  return userFacingErrorCopy(error, fallback)
}

export function normalizeCanonicalEmail(email: string): string {
  return email.trim().toLowerCase()
}
