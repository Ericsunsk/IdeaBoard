// Colocated admin messages for the users page (zh-only, no `t()` wrapping).
//
// Copy Policy (`shared/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Toast literals stay in
// `useUsersFeedback.ts` next to the feedback factory; row-view labels stay in
// `view.ts`; command copy stays in `commands.ts`/`context.ts`. Dialog copy for
// `AdminUserBanDialog.vue` / `OAuthProviderSettingsDialog.vue` lives here.

export const USERS_MESSAGES = {
  searchLabel: '搜索用户',
  searchPlaceholder: '搜索用户姓名、邮箱、IP...',
  oauthSettings: 'OAuth Provider 设置',
  filterUsers: '筛选用户',
  role: '角色',
  status: '状态',
  refreshUsers: '刷新用户',
  userColumn: '用户',
  joinedAtColumn: '加入时间',
  balanceColumn: '账户余额',
  openWalletAria: (displayName: string) => `打开 ${displayName} 的账户详情`,
  actionsMenuAria: (displayName: string) => `${displayName} 操作菜单`,
  // Ban dialog.
  banAria: '封禁用户',
  banDescription: '设置封禁时长与原因后确认封禁。',
  banAccessBadge: '访问控制',
  banForceBadge: '强制限制',
  banTitle: '高级访问限制',
  banDurationLabel: '封禁时长 (Duration)',
  banReasonLabel: '封禁原因 (内部记录)',
  banReasonPlaceholder: '例如：发表违规内容、恶意刷量...',
  banFooterNote: '封禁会立即断开该用户所有连接，封禁期间无法重新登录。',
  banConfirm: '确认封禁',
  // OAuth provider settings dialog.
  oauthDescription: '配置 OAuth 登录渠道后保存。',
  oauthLoading: '加载 OAuth Provider 设置',
  oauthGroupAria: 'OAuth 登录方式',
  oauthSelectLogoAria: (providerLabel: string) => `选择 ${providerLabel} Logo`,
  oauthProviderLoginAria: (providerLabel: string) => `${providerLabel} 登录`,
  oauthCredentialsTitle: (providerLabel: string) => `${providerLabel} 凭据配置`,
  oauthEnabledLabel: (enabled: boolean) => (enabled ? '已启用' : '未启用'),
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
