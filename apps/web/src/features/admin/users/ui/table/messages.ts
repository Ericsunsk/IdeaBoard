// Colocated admin messages for the users page (zh-only, no `t()` wrapping).
//
// Copy Policy (`core/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Toast literals stay in
// `useUsersFeedback.ts` next to the feedback factory; row-view labels stay in
// `view.ts`; command copy stays in `commands.ts`/`context.ts`. Dialog copy for
// `AdminUserBanDialog.vue` lives here.

export const USERS_MESSAGES = {
  searchLabel: '搜索用户',
  searchPlaceholder: '搜索用户姓名、邮箱、IP...',
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
} as const
