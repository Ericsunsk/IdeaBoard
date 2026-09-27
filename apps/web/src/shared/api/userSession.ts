import { getUser } from './generated-query/public/user/user'
import { i18n } from '../i18n'
import type { User } from './sessionUserType'

// Session principal owned by the shared API boundary (issue #809).
//
// The legacy `generated-query/user` + `apiResponse` transport is gone; the
// Public Data Plane serves the principal at `GET /api/v1/user` (`UserEnvelope`).
// The `User` view type lives in `./sessionUserType` (import it from there, never
// from here: this module's runtime transport sits downstream of the shared
// API client and a type edge back would close an import cycle); unknown
// envelope fields fail closed to `undefined`.

function asTrimmedString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed ? trimmed : undefined
}

export function toSessionUser(raw: unknown): User | null {
  if (!raw || typeof raw !== 'object') return null
  const record = raw as Record<string, unknown>
  const publicId = asTrimmedString(record.public_id ?? record.id)
  if (!publicId) return null
  const user: User = { public_id: publicId }
  const displayName = asTrimmedString(record.display_name)
  if (displayName !== undefined) user.display_name = displayName
  const avatar = asTrimmedString(record.avatar)
  if (avatar !== undefined) user.avatar = avatar
  const role = asTrimmedString(record.role)
  if (role !== undefined) user.role = role
  const email = asTrimmedString(record.email)
  if (email !== undefined) user.email = email
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
