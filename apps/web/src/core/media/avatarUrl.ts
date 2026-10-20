const PUBLIC_AVATAR_ROUTE_PREFIX = '/api/v1/public/users/'

const AVATAR_SIZE_VARIANTS = [32, 64, 128, 256] as const

type AvatarSizeVariant = (typeof AVATAR_SIZE_VARIANTS)[number]

function resolveAvatarSizeVariant(
  cssWidthPx: unknown,
  devicePixelRatio?: unknown,
): AvatarSizeVariant {
  const width = Number(cssWidthPx)
  if (!Number.isFinite(width) || width <= 0) return 32
  const ratio = Number(devicePixelRatio ?? 1)
  const scale = Number.isFinite(ratio) && ratio > 0 ? ratio : 1
  const effective = width * scale
  for (const size of AVATAR_SIZE_VARIANTS) {
    if (effective <= size) return size
  }
  return 256
}

/** Size public avatars for CSS width × DPR; pass external URLs through unchanged. */
export function resolveSizedAvatarUrl(
  base: unknown,
  cssWidthPx: unknown,
  devicePixelRatio: unknown = typeof window === 'undefined' ? 1 : window.devicePixelRatio,
): string | undefined {
  if (typeof base !== 'string') return undefined
  const trimmed = base.trim()
  if (!trimmed) return undefined
  if (!trimmed.startsWith(PUBLIC_AVATAR_ROUTE_PREFIX)) return trimmed
  const url = new URL(trimmed, 'https://avatar.invalid')
  url.searchParams.set('size', String(resolveAvatarSizeVariant(cssWidthPx, devicePixelRatio)))
  return `${url.pathname}${url.search}${url.hash}`
}

/** Minimal roster identity for member avatar resolution. */
export type MemberAvatarEntry = {
  userId: string
  avatar_url?: string | null | undefined
}

/** Minimal session viewer for the self-avatar fallback. */
export type MemberAvatarViewer =
  { public_id?: string | null | undefined; avatar?: string | null | undefined } | null | undefined

/**
 * Resolves one member avatar for display at 64px CSS width: the roster
 * entry URL first, then the viewer's own session avatar when the entry
 * is the viewer, else undefined. External URLs pass through unsized.
 */
export function resolveMemberAvatar(
  entry: MemberAvatarEntry,
  currentUser: MemberAvatarViewer,
): string | undefined {
  const fromEntry = resolveSizedAvatarUrl(entry.avatar_url, 64)
  if (fromEntry) return fromEntry
  if (entry.userId && currentUser?.public_id && entry.userId === currentUser.public_id) {
    return resolveSizedAvatarUrl(currentUser.avatar, 64)
  }
  return undefined
}

/** Minimal roster identity for member initial resolution. */
export type MemberInitialEntry = {
  userId: string
  email?: string | null | undefined
}

/**
 * Resolves one member fallback initial: the email first letter first,
 * then the userId with the `usr_` prefix stripped, else `U`.
 */
export function resolveMemberInitial(entry: MemberInitialEntry): string {
  if (entry.email) {
    return entry.email.charAt(0).toUpperCase()
  }
  return entry.userId.replace(/^usr_/, '').charAt(0).toUpperCase() || 'U'
}
