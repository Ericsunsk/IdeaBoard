export type OAuthProviderCode = string

export function getOAuthProviderLabel(provider: OAuthProviderCode): string {
  return provider
    .split(/[-_]/g)
    .filter(Boolean)
    .map((part) => part[0].toUpperCase() + part.slice(1))
    .join(' ')
}
