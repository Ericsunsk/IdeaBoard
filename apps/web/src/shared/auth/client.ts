import { createAuthClient } from 'better-auth/vue'
import { adminClient, usernameClient } from 'better-auth/client/plugins'
import { AUTH_BASE_URL, resolveAuthUrl } from '../config/runtimeUrls'

export const authClient = createAuthClient({
  baseURL: AUTH_BASE_URL,
  plugins: [usernameClient(), adminClient()],
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

export async function registerUsername(username: string, password: string): Promise<string> {
  const response = await fetch(resolveAuthUrl('/api/auth/register/username'), {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!response.ok) {
    let payload: unknown
    try {
      payload = await response.json()
    } catch {
      payload = null
    }
    throw new Error(extractBetterAuthErrorMessage(payload, '注册失败'))
  }
  const payload = (await response.json()) as { data?: { user?: { username?: unknown } } }
  const canonicalUsername = getTrimmedString(payload.data?.user?.username)
  if (!canonicalUsername) {
    throw new Error('注册成功，但服务端未返回 canonical username')
  }
  return canonicalUsername
}
