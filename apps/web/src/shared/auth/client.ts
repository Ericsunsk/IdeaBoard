import { createAuthClient } from 'better-auth/vue'
import { adminClient, usernameClient } from 'better-auth/client/plugins'

const authBaseURL =
  (import.meta.env.VITE_AUTH_BASE_URL || '').trim() ||
  (typeof window !== 'undefined' ? window.location.origin : 'http://localhost:5173')

export const authClient = createAuthClient({
  baseURL: authBaseURL,
  plugins: [usernameClient(), adminClient()],
  fetchOptions: {
    credentials: 'include',
  },
})

export function extractBetterAuthErrorMessage(error: any, fallback: string) {
  if (!error) return fallback
  if (typeof error.message === 'string' && error.message.trim()) return error.message
  if (typeof error.statusText === 'string' && error.statusText.trim()) return error.statusText
  if (typeof error.error?.message === 'string' && error.error.message.trim()) return error.error.message
  if (typeof error.data?.message === 'string' && error.data.message.trim()) return error.data.message
  if (typeof error.data?.msg === 'string' && error.data.msg.trim()) return error.data.msg
  return fallback
}
