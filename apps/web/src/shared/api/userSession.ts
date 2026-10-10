import { getUser } from './generated-query/public/user/user'
import { i18n } from '../i18n'
import type { User } from './sessionUserType'

// /user projects principal fields, the persisted avatar URL, and profile
// facts (storage namespace, status, timestamps).
// Better Auth owns email; it stays outside this view.

function asTrimmedString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed ? trimmed : undefined
}

export function toSessionUser(raw: unknown): User | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  // Wire-boundary conversion: the canonical `User` schema keys the public
  // identifier as `id`. A `public_id` key never arrives on this surface
  // (no caller sends it), so only `id` is read — no legacy fallback.
  const publicId = asTrimmedString(record.id)
  if (!publicId) return null
  const user: User = { public_id: publicId }
  const displayName = asTrimmedString(record.display_name)
  if (displayName !== undefined) user.display_name = displayName
  const avatar = asTrimmedString(record.avatar)
  if (avatar !== undefined) user.avatar = avatar
  const role = asTrimmedString(record.role)
  if (role !== undefined) user.role = role
  const personalWorkspaceId = asTrimmedString(record.personal_workspace_id)
  if (personalWorkspaceId !== undefined) user.personal_workspace_id = personalWorkspaceId
  const storageNamespace = asTrimmedString(record.storage_namespace)
  if (storageNamespace !== undefined) user.storage_namespace = storageNamespace
  if (typeof record.status === 'number') user.status = record.status
  const createdAt = asTrimmedString(record.created_at)
  if (createdAt !== undefined) user.created_at = createdAt
  const updatedAt = asTrimmedString(record.updated_at)
  if (updatedAt !== undefined) user.updated_at = updatedAt
  return user
}

export async function fetchCurrentUserInfo(): Promise<{ user: User }> {
  const fallback = i18n.global.t('common.errors.sessionLoadFailed')
  const envelope = await getUser()
  const data = (envelope as { data?: unknown } | null | undefined)?.data
  const user = toSessionUser(data)
  if (!user) throw new Error(fallback)
  return { user }
}
