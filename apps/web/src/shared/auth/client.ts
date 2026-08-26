import { createAuthClient } from 'better-auth/vue'
import { adminClient, emailOTPClient } from 'better-auth/client/plugins'
import { AUTH_BASE_URL } from '../config/runtimeUrls'
import { userFacingErrorCopy } from '../errors/errorCatalog'

export const authClient = createAuthClient({
  baseURL: AUTH_BASE_URL,
  plugins: [emailOTPClient(), adminClient()],
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
