export const OAUTH_PROVIDER_CODES = ['google', 'discord'] as const

export type OAuthProviderCode = (typeof OAUTH_PROVIDER_CODES)[number]

export const OAUTH_PROVIDER_LABELS: Record<OAuthProviderCode, string> = {
  google: 'Google',
  discord: 'Discord',
}

export function getOAuthProviderLabel(provider: OAuthProviderCode): string {
  return OAUTH_PROVIDER_LABELS[provider]
}
