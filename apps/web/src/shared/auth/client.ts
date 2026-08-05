import { createAuthClient } from 'better-auth/vue'
import { adminClient, emailOTPClient, usernameClient } from 'better-auth/client/plugins'
import { AUTH_BASE_URL } from '../config/runtimeUrls'

export const authClient = createAuthClient({
  baseURL: AUTH_BASE_URL,
  plugins: [usernameClient(), emailOTPClient(), adminClient()],
  fetchOptions: {
    credentials: 'include',
  },
})

type ErrorRecord = Record<string, unknown>

function asErrorRecord(value: unknown): ErrorRecord | null {
  return value && typeof value === 'object' ? (value as ErrorRecord) : null
}

function getTrimmedString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function extractBetterAuthErrorMessage(error: unknown, fallback: string) {
  if (!error) return fallback

  const root = asErrorRecord(error)
  const message = getTrimmedString(root?.message)
  if (message) return message

  const statusText = getTrimmedString(root?.statusText)
  if (statusText) return statusText

  const nestedError = asErrorRecord(root?.error)
  const nestedErrorMessage = getTrimmedString(nestedError?.message)
  if (nestedErrorMessage) return nestedErrorMessage

  const data = asErrorRecord(root?.data)
  const dataMessage = getTrimmedString(data?.message)
  if (dataMessage) return dataMessage

  const dataMsg = getTrimmedString(data?.msg)
  if (dataMsg) return dataMsg

  return fallback
}

export function normalizeCanonicalEmail(email: string): string {
  return email.trim().toLowerCase()
}
