# IdeaBoard Auth 运维手册

本文档定义 `Go/Vue 管理后台` 与 `Better Auth Admin plugin` 的职责边界，避免在账号与安全操作上出现重复实现和误操作。

## 目标

- 业务侧运营动作留在 IdeaBoard Admin。
- 身份与安全侧动作优先走 Better Auth 生态能力；高风险跨库动作由 Go Admin API 编排，避免前端直连删除身份数据。
- 出现异常时有一致的处理路径和审计口径。

## 职责分工

### IdeaBoard Admin（Go + Vue）

适用于业务治理：

- 用户封禁与解封（`status` 软控制）。
- 角色升降级：由 Go Admin API 校验后同步 Better Auth `user.role` 与业务侧 `users.role`。
- 受控彻底注销：仅允许删除非管理员、非当前登录管理员账号；Go Admin API 先在 PostgreSQL 冻结业务用户与 Credit Wallet 并记录 durable deletion phase，再依次清理 Auth、Storage 和业务侧 PII。
- 配额、业务权限分配。
- 业务配置维护（画板、渠道、系统提示词、存储配置等）。
- OAuth Provider 配置：从 `用户管理` 页的 settings icon 进入，经 Go Admin API bridge 保存到 Auth Service；当前只开放 Google。

> [!WARNING]
> 不在此处执行：后台改密、前端直连 Better Auth 物理删号。

### Better Auth Admin plugin（身份与安全）

适用于安全治理：

- 内嵌入口：后台 `用户管理` 页。
- 当前已开放：身份用户列表、角色升降级、身份侧封禁/解封、会话吊销/踢下线、伪装登录/停止伪装、受控彻底注销。
- 配套控制项：开放注册开关（落 `system_configs`）。登录/注册入口限流由 Auth Service 固定安全默认值控制。

> [!NOTE]
> 暂不在内嵌 UI 暴露：后台改密。未开放的高风险动作应走受控身份运维流程，并在明确审计要求后再扩展 UI。

## 操作准则

> [!TIP]
> 默认先封禁，再评估是否需要进一步身份侧处理。封禁可立即阻断业务访问且保留业务数据链路，便于审计与恢复。

1. **彻底注销只能走后台 `用户管理` 页的受控操作。** 前端不得直接调用 `authClient.admin.removeUser`；删除必须经由 Go Admin API 完成管理员自删保护、管理员账号保护、`auth_user_id` 校验与 Auth Service internal key 校验，避免本地业务库与 auth 库状态漂移。

2. **角色升降级只能走后台 `用户管理` 页的受控操作。** 前端不得直接调用 `authClient.admin.setRole`；角色变更必须同步 Better Auth 与 Go 业务库，且不允许修改当前登录管理员自己的角色。

3. **生产环境默认建议关闭公开注册。** 可在管理后台 `用户管理` 页的"开放注册"开关控制，仅在明确运营窗口内开启。

4. **公开注册使用 Better Auth `/api/auth/sign-up/email`。** Client 提交 display name、canonical Email 和 Password；Email 验证后以 `canonical_email` identity 进入 Go Identity。Password 登录只接受 Email，username 不再是 identity 或登录凭据。

5. **高风险动作保留审计记录。** 建议记录操作人、时间、原因、影响用户与回滚策略。

## 推荐流程

### 场景 A：普通违规账号处理

1. 在 IdeaBoard Admin 将用户 `status` 置为封禁。
2. 复核最近任务、画板与共享链接影响。
3. 若属于永久封禁，再在 `用户管理` 页执行身份侧封禁。

### 场景 B：账号疑似被盗

1. 在 IdeaBoard Admin 先封禁，立即阻断业务调用。
2. 在 `用户管理` 页使用伪装登录辅助排查前台体验；完成后立即停止伪装。
3. 在 `用户管理` 页执行会话吊销/踢下线，立即终止该账号活跃会话。
4. 如需强制凭据更新，走受控身份运维流程。
5. 完成核验后再解封。

### 场景 C：用户忘记密码

1. 优先走前台自助找回流程。
2. 当前后台不开放人工改密。
3. 必要时由安全运维按受控身份运维流程处理。

### 场景 D：合规彻底注销

1. 先确认目标账号不是管理员账号，也不是当前登录管理员自己。
2. 在 `用户管理` 页执行"彻底注销用户"。
3. Go Admin API 先在单个 PostgreSQL transaction 中把业务用户设为 inactive、冻结 Credit Wallet，并创建 `user_deletion_requests` 的 `pending_auth` 记录。
4. 请求随后按 `pending_auth` → `pending_storage` → `pending_tombstone` → `completed` 执行；Auth 404 与 Storage cleanup 都按幂等结果处理。
5. 任一步失败都保持 Wallet frozen；直接重试同一删除 API 会从持久 phase 继续，不需要手工改数据库。
6. 完成后保留匿名化 user tombstone，并永久保留 Task、Credit Ledger 与 Welcome Grant claim。只有持续重试仍失败时才依据日志排查对应外部依赖，不要跳过 phase 手工物理删除。

### 场景 E：Resend 配置与回滚

1. 保持公开注册关闭，先完成 Resend 发件 Domain verification。
2. 确认 API/Auth 使用同一份稳定的 `APP_SECRETS_ENCRYPTION_KEY`，再在 Admin `系统设置` 保存 Resend API Key 与发件人配置。
3. 发送 Test Email；只有 readiness 为 `ready` 且实际收到邮件后，才开放公开注册。
4. 上线 smoke 同时覆盖注册 OTP、Password reset、Email change 和 Email + Password 登录。邮件 workflow 依赖 readiness，已有 Password 登录不依赖 readiness。
5. 出现故障时先关闭公开注册，再清除或替换 Resend 配置；不要删除用户、Session 或业务数据。修复后重新完成 Test Email，再恢复公开注册。

### 场景 F：Google OAuth 分环境上线

#### 环境隔离

Google Cloud Console 中创建 `Web application` 类型的 OAuth Client。每个环境登记的 Authorized redirect URI 必须与 Admin 只读显示的 callback URL 完全一致：

| 环境       | Google Client                                      | Authorized redirect URI                                  |
| ---------- | -------------------------------------------------- | -------------------------------------------------------- |
| Production | 独立 Production Client，不与其他环境共享           | `${PRODUCTION_BETTER_AUTH_URL}/api/auth/callback/google` |
| Staging    | 独立 Staging Client，不使用 Production credentials | `${STAGING_BETTER_AUTH_URL}/api/auth/callback/google`    |
| Local      | 复用 Staging Client，仅供授权开发人员              | 默认 `http://localhost:5173/api/auth/callback/google`    |

Local 复用方法：把本地 `BETTER_AUTH_URL` 对应的 callback URL 加入 **Staging Client** 的 Authorized redirect URIs，通过团队 Secret Manager 获取 Staging Client ID/Secret，再在本地 Admin 保存。默认开发基准 URL 是 `http://localhost:5173`；若修改了 `BETTER_AUTH_URL`，以 Admin Dialog 的只读 callback URL 为准。不要把 localhost URI 或 Staging Secret 加入 Production Client、仓库、Issue、日志或截图。

Google Console 登记的是 Better Auth callback `/api/auth/callback/google`；登录成功后浏览器进入 Web completion page `/user/auth/callback`。两者不是同一个 URL，后者不需要登记为 Google Authorized redirect URI。

#### 首次配置

1. 先部署 migration 和应用，保持 Google disabled；确认 `BETTER_AUTH_URL` 是该环境的公开 HTTPS Auth 基准地址，`APP_SECRETS_ENCRYPTION_KEY` 是稳定的 base64 32-byte key。
2. 在 Google Console 配置对应环境的 callback URL。Google scopes 只允许 `openid email profile`，不申请 Google API scope、offline access 或 refresh token。
3. 进入 Admin `用户管理`，点击筛选 icon 左侧的 settings icon，选择 Google。先保持 disabled，填写 Client ID 和 Client Secret 后保存。Secret 为 write-only；后续留空表示保留原值。
4. 核对 Client Secret 输入框显示“已配置”的占位提示，callback URL 与 Google Console 完全一致。修改 Client ID 时先关闭 Provider，再保存新 Client ID/Secret。
5. 保持 disabled 完成原生 Auth smoke：Email + Password、注册 OTP、Password reset 和 Email change 均可用；User Login 不显示 Google button，Admin Login 始终不显示 Google button。
6. 在 Staging 打开注册并启用 Google。保存应立即使 `/api/auth/capabilities` 返回 `social.google=true`，无需重启 Auth Service。

#### Staging external smoke

使用隔离测试账号逐项记录时间、环境、测试 Email、结果和失败日志 request ID；不要记录 Authorization Code、Token 或 Client Secret。

1. 新 Google 用户：完成 Consent 和 callback，确认创建唯一业务用户，`display_name`/Avatar 只在首次 provisioning 初始化。
2. 资料保持：主动修改 display name 或 Avatar 后再次 Google 登录，确认 Google profile 不覆盖修改。
3. 同 Email linking：先记录已验证 Email + Password 用户的 `public_id`，再以相同 verified Google Email 登录；确认仍是同一 `public_id`，没有重复业务用户。
4. 不同 Email 隔离：使用不同 Google Email 登录，确认不会合并到现有账号。
5. 注册关闭：关闭公开注册后，已 linked Google 用户仍可登录；从未登录过的新 Google 用户必须被拒绝。
6. 安全 redirect：分别验证合法站内 `redirect` 和外部 URL；前者回到目标页面，后者回到安全默认页，不能跳转到外部域名。
7. 失败清理：取消 Consent 或制造 callback 失败，确认页面只显示通用错误，Session/cache 被清理，URL 和日志不暴露 Provider 原始错误、Token 或 Secret。
8. Provider disabled：关闭 Google 后确认 capability 变为 false、User Login button 消失，Email + Password、OTP 与 Password reset 仍可用。

scope allowlist、Secret 加密/留空保留、Provider 异常 fail closed 和 runtime capability 由自动化 contract test 覆盖。`email_verified=false` 当前自动化测试覆盖 Provider profile 到不可信 Email 的 mapping gate，但完整 callback/provisioning 拒绝仍属于 integration gate；若受控 Google 测试身份能返回 unverified Email，再补 external smoke。普通 Google 账号通常无法稳定构造该状态，不能用“Google 一般返回 verified”替代 application-level gate。

#### Production rollout gate

Production Client 必须在 Staging external smoke 全部通过后单独配置。先用 Production 测试账号在公开注册关闭时验证已有 linked 用户登录，再在受控注册窗口验证一个新用户；复核 callback host、`public_id`、资料初始化和安全 redirect 后才保留 enabled。Production 不复制 Staging 数据库中的 encrypted Secret，必须通过 Production Admin 单独保存 Production credentials。

> [!WARNING]
> 自动化 tests/build 只能验证应用 contract，不能证明 Google Console、DNS、反向代理、Consent Screen 与真实账号链路正确。本次 clean rollout 尚未使用真实 Google credentials 完成 external flow；在 Staging 留存上述 smoke 证据前，不得把 Production Google 标记为已验证或长期 enabled。

#### 回滚

1. 在 Admin `用户管理` 的 OAuth settings 中关闭 Google 并保存；runtime capability 应立即变为 false，无需回滚 image、删除账号或恢复数据库。
2. 确认 User Login 不再显示 Google button，并复测 Email + Password、OTP 和 Password reset。关闭 Provider 不撤销已有 Session，也不删除 linking/account 记录。
3. 已退出的 Google-only 用户在 Provider disabled 期间无法重新登录；故障公告和客服处理需要明确这一临时影响。修复 credentials 或 Google Console 配置后，先在 Staging 重跑 external smoke，再重新启用 Production。
4. 怀疑 Secret 泄露时先在 Google Console rotate/revoke Secret，再保持 Provider disabled，保存新 Secret 后重新验证。系统不提供 Clear action；Client Secret 留空只表示保留旧值。

## 上线与安全基线

> [!IMPORTANT]
> `用户管理` 页的身份侧操作建议仅允许内网/VPN/IP allowlist 访问。

- 生产环境必须设置 `BETTER_AUTH_SECRET`（至少 32 字符高熵随机值）与 `BETTER_AUTH_URL`（正式访问地址），不得使用 `replace-with-*` 或本地开发 fallback。
- `AUTH_INTERNAL_KEY` 必须在 API 与 Auth Service 中保持一致，并仅在两侧共享；生产环境不得使用占位值。
- `APP_SECRETS_ENCRYPTION_KEY` 必须同时提供给 API 与 Auth Service，用于静态加密 Resend/API/Storage Secret；生产环境不得临时轮换或使用占位值。
- Auth Service 与 Go API 建议只监听 `127.0.0.1`，由 Nginx 统一对外暴露，避免绕过可信代理与限流边界。
- 登录/注册限流依赖可信代理解析后的客户端 IP；不要让公网直接访问 `:6001`。
- 使用最小权限账号，避免共享管理员口令。
- 对删号、改密、会话吊销与批量操作执行双人复核。
- 定期检查后台"注册与 API 限流"是否符合 Go API 流量规模；Auth Service 登录/注册入口限流使用代码内固定默认值，不读取后台 runtime settings。
- 启用 Admin plugin 后先执行迁移：开发环境 `npm --prefix apps/auth run migrate`，生产环境 `npm --prefix apps/auth run migrate:dist`。

## 关联文档

| 文档                                       | 用途                           |
| ------------------------------------------ | ------------------------------ |
| [README.md](../../README.md)               | 项目首页                       |
| [DESIGN.md](../../DESIGN.md)               | 系统设计与 Auth 边界           |
| [DEVELOPMENT.md](../../DEVELOPMENT.md)     | 开发手册                       |
| [deployment.md](./deployment.md)           | 部署与运维                     |
| [data-protection.md](./data-protection.md) | Backup、restore 与密钥材料处理 |
