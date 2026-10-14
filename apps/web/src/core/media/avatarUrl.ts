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
