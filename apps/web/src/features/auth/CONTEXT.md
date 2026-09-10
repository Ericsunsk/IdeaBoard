# Auth

Auth 是 Better Auth 会话事实主，拥有登录/注册/OTP/OAuth/登出/impersonate 退出、登录注册弹窗 chrome 与登录后重定向意图。

## Language

**User Session**:
客户端会话事实：`user/isAdmin/sessionValidatedAt`，唯一单例 `useUserStore()`。服务端会话以 Better Auth 为准，客户端只做 30s 缓存快照。
_Avoid_: useQuery-cached session, duplicated user refs

**Auth Transport**:
唯一触达 Better Auth 的适配层（`session/transport.ts`）：signIn/signUp/signOut/getSession 与 `/api/auth/*` fetch。面板与页面不得绕过它。
_Avoid_: direct authClient call, scattered auth fetch

**Session Cache**:
内存会话快照，TTL 30s（`browserSessionTransition.ts`）。`ensureSession` 缓存有效直接返回，否则 `hasSession ? fetch : clear`。
_Avoid_: perpetual cache, fetch on every render

**Post-Auth Path**:
登录成功后的重定向意图，只存 `sessionStorage`（`ideaboard:auth:post-auth-path`），消费即焚，且必须过内部路径校验。
_Avoid_: localStorage redirect, open redirect

**Auth Overlay**:
全局登录/注册弹窗（`mode: login|register|null`），由 `createAuthOverlay/provideAuthOverlay` 安装在 `WorkspaceLayout`，各 surface 按需 `open`。
_Avoid_: per-page login modal, route-only auth gate

**OTP**:
忘记密码与注册验证的两段式表单态（`otpPending`）。成功才进入 authenticated，失败只 toast 不切态。
_Avoid_: single-form auth, silent OTP failure

**Impersonation**:
管理员冒充横幅态：`Better Auth useSession()` 的 `impersonatedBy` 只在 `WorkspaceLayout` 读取展示；退出走 `stopImpersonating`。
_Avoid_: impersonation flags in session store, hidden impersonation

## Module Ownership

| Module            | Responsibility                                                                                                            | Public Interface                                                     | Implementation Detail Boundary                                                              |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `session`         | 会话事实与传输：store 单例、transport、transition 状态机、persistence 薄封装。                                            | `useUserStore()`、`authTransport`（来自 `session/transport.ts`）。无 barrel，直连相对导入。 | Better Auth client 实例不出 transport；`localStorage['user']` 只经 `shared/utils/auth.ts`。 |
| `components`      | 弹窗 chrome：`UserAuthModals` 路由器、`Login/Register` 面板、`AuthModalHost` 壳、共享 `useAuthPanelControls` 与表单工具。 | 无 barrel；`UserAuthModals` 按 route path 或 overlay mode 挂载面板。 | 面板提交态（pending 禁关）与字段校验留在面板内，不得上浮到 modal 宿主。                     |
| `pages`           | 唯一的路由页 `UserAuthCallbackPage`（OAuth 回调，`loading/failed` 两态）。                                                | 被 `workspaceRoutes` 懒加载。                                             | 回调失败必须清会话并回到无 query 的 callback 页，不得残留 provider error 参数。             |
| `useAuthFeedback` | 登录/注册/退出冒充三组 toast，基于 `useAppToast + errorCatalog + i18n`，无状态。                                          | `login/register/stopImpersonating` 三组 success/error。              | 文案 key 收敛到 errorCatalog，禁止散写。                                                    |

## Rules

- 页面与面板不得直调 generated client 或 Better Auth：链路固定为 `Panel --runAsyncAction--> useUserStore --> authTransport --> authClient/fetch`。
- 身份变更（登录/切换身份/登出/401·403）必须：更新 refs + 写/清 `localStorage` 快照 + 全清 auth-scoped Query cache（`clearAuthScopedQueryCache`）。
- `?redirect=` 与 post-auth path 必须过 `isSafeInternalRedirectPath` / `normalizeSafeInternalRedirectPath`，非法回落 `USER_BOARDS_PATH`。
- `localStorage` 只存会话快照与 scope 恢复键；重定向意图只放 `sessionStorage`；`File/blob` 预览不得落盘（见 search 延续约束）。

## State Machines and Invariants

- **Session**: `unauthenticated` → `restoring`（内存恢复）→ `authenticated`（login/verifyOTP + `fetchUserInfo(force, failClosed)`）→ `session-expired`（401/403/failClosed → `clearSession` 清 refs + 存储 + Query）。`ensureSession` 缓存 < 30s 直接返回。
- **OAuth callback**: `loading` → `ensureSession(fresh) + replace(redirect)`；providerError 或会话失败 → `logout/clear + replace` 到无参 callback → `failed`（显示回登录按钮）。
- **Overlay**: `closed` → `open(mode, postAuthPath?)` → `login|register` → `close` / `overlay-success`（消费 post-auth path 并 `router.push`）。overlay 关闭同时 `clearPendingSearchAuthAction`。
- **Panel submit**：`idle` → 提交（`runAsyncAction.pending`，submit disabled，mask/esc 不可关）→ `success`（emit）/ `error`（toast）。注册特有 `form` → `otpPending` → `success` 两段。
