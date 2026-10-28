const EMAIL_AVATAR_CDN_BASE = 'https://api.dicebear.com/9.x/big-ears-neutral/svg'

// Auth stores a URL seeded by the internal user id; Go Media renders the bytes.
export function buildEmailAvatarUrl(seed: string): string {
  const normalized = String(seed ?? '').trim()
  if (!normalized || normalized.length > 191 || /\s/.test(normalized)) return ''
  return `${EMAIL_AVATAR_CDN_BASE}?seed=${encodeURIComponent(normalized)}`
}
