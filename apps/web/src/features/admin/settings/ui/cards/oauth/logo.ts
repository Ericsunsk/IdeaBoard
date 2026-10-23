import googleLogoUrl from '@/core/assets/oauth/google.svg'
import discordLogoUrl from '@/core/assets/oauth/discord.svg'
import { adminBrandLogoMap, getAdminBrandLogoOverride } from '../../../../shared/index'
import type { OAuthProviderCode } from '@/core/auth/oauthProviders'

// 登录源 logo 必须精确解析：模糊匹配会把 'google' 命中到 AI 模型的
// google 品牌（models.dev 通用星星占位图），故只认 scope override，
// 其余一律用站内品牌资产。
export function resolveOAuthProviderLogo(provider: OAuthProviderCode): string {
  const override = getAdminBrandLogoOverride('oauth-provider', provider)
  const overrideSrc = override ? adminBrandLogoMap[override] : undefined
  if (overrideSrc) return overrideSrc
  return provider === 'google' ? googleLogoUrl : discordLogoUrl
}
