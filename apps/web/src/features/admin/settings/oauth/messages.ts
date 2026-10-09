// Colocated OAuth settings messages (zh-only, no `t()` wrapping).

export const OAUTH_MESSAGES = {
  oauthSettings: 'OAuth Provider 设置',
  oauthDescription: '配置 OAuth 登录渠道后保存。',
  oauthLoading: '加载 OAuth Provider 设置',
  oauthGroupAria: 'OAuth 登录方式',
  oauthSelectLogoAria: (providerLabel: string) => `选择 ${providerLabel} Logo`,
  oauthProviderLoginAria: (providerLabel: string) => `${providerLabel} 登录`,
  oauthCredentialsTitle: (providerLabel: string) => `${providerLabel} 凭据配置`,
  oauthEnabledLabel: (enabled: boolean) => (enabled ? 'Enabled' : 'Disabled'),
  oauthClientIdPlaceholder: '请输入 OAuth Client ID',
  oauthClientIdRequired: '请输入 Client ID',
  oauthClientSecretRequired: '请输入 Client Secret',
  oauthSecretPlaceholder: (configured: boolean) =>
    configured ? '已配置，留空则保持不变' : '输入 Client Secret',
  oauthCopyCallback: '复制',
  oauthCallbackHint: (portal: string) => `请在 ${portal} 将此 URL 添加为重定向白名单 URI`,
  oauthSave: '保存',
  oauthLoadFailed: '加载 OAuth Provider 设置失败，请重试',
  oauthIncompleteProvider: (providerLabel: string) =>
    `请先配置 ${providerLabel} 的 Client ID 和 Client Secret`,
  oauthSaved: (providerLabel: string) => `${providerLabel} OAuth 设置已保存`,
  oauthRefreshFailed: '设置已保存，但状态刷新失败，请重新加载',
  oauthSaveFailed: '保存 OAuth Provider 设置失败，请重试',
  oauthConfigMissing: '未找到 OAuth Provider 配置',
  oauthCallbackCopied: 'Callback URL 已复制到剪贴板',
  oauthCopyFailed: '复制失败，请手动选择复制',
} as const
