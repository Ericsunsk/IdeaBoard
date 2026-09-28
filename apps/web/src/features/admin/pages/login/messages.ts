// Colocated admin messages for the login page (zh-only, no `t()` wrapping).
//
// Copy Policy (`shared/CONTEXT.md`): admin/ops surfaces keep their literals next
// to the page that owns them instead of locale bundles. Feedback-toast literals
// stay in `useAdminLoginFeedback.ts` next to the feedback factory.

export const ADMIN_LOGIN_MESSAGES = {
  title: '管理员登录',
  passwordLabel: '密码',
  hidePassword: '隐藏密码',
  showPassword: '显示密码',
  loggingIn: '登录中',
  enterAdmin: '进入后台',
  userLogin: '用户登录',
  emailRequired: '请输入 Email',
  passwordRequired: '请输入密码',
} as const
