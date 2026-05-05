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
- 受控彻底注销：仅允许删除非管理员、非当前登录管理员账号；由 Go Admin API 先调用 Auth Service internal endpoint 清理身份、会话与凭据，再清理业务侧用户数据。
- 配额、业务权限分配。
- 业务配置维护（画板、渠道、系统提示词、存储配置等）。

> [!WARNING]
> 不在此处执行：后台改密、前端直连 Better Auth 物理删号。

### Better Auth Admin plugin（身份与安全）

适用于安全治理：

- 内嵌入口：后台 `用户管理` 页。
- 当前已开放：身份用户列表、角色升降级、身份侧封禁/解封、会话吊销/踢下线、伪装登录/停止伪装、受控彻底注销。
- 配套控制项：开放注册开关、防刷限流设置（落 `system_configs`）。

> [!NOTE]
> 暂不在内嵌 UI 暴露：后台改密。未开放的高风险动作应走受控身份运维流程，并在明确审计要求后再扩展 UI。


## 操作准则

> [!TIP]
> 默认先封禁，再评估是否需要进一步身份侧处理。封禁可立即阻断业务访问且保留业务数据链路，便于审计与恢复。

1. **彻底注销只能走后台 `用户管理` 页的受控操作。** 前端不得直接调用 `authClient.admin.removeUser`；删除必须经由 Go Admin API 完成管理员自删保护、管理员账号保护、`auth_user_id` 校验与 Auth Service internal key 校验，避免本地业务库与 auth 库状态漂移。

2. **角色升降级只能走后台 `用户管理` 页的受控操作。** 前端不得直接调用 `authClient.admin.setRole`；角色变更必须同步 Better Auth 与 Go 业务库，且不允许修改当前登录管理员自己的角色。

3. **生产环境默认建议关闭公开注册。** 可在管理后台 `用户管理` 页的"开放注册"开关控制，仅在明确运营窗口内开启。

4. **高风险动作保留审计记录。** 建议记录操作人、时间、原因、影响用户与回滚策略。


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
3. Go Admin API 会先调用 Auth Service `/internal/users/:id` 删除身份记录、会话与凭据；Auth 删除失败时不会继续删除业务侧数据。
4. 若业务侧清理失败，按接口错误与服务日志进行补偿处理，避免重复手工删除造成二次漂移。


## 上线与安全基线

> [!IMPORTANT]
> `用户管理` 页的身份侧操作建议仅允许内网/VPN/IP allowlist 访问。

- 使用最小权限账号，避免共享管理员口令。
- 对删号、改密、会话吊销与批量操作执行双人复核。
- 定期检查后台"防刷限流设置"与 `system_configs`（`auth_rate_limit_window`、`auth_rate_limit_max_requests`）是否符合流量规模。
- 启用 Admin plugin 后先执行迁移：开发环境 `npm --prefix apps/auth run migrate`，生产环境 `npm --prefix apps/auth run migrate:dist`。


## 关联文档

| 文档 | 用途 |
|------|------|
| [README.md](../README.md) | 项目首页 |
| [DEVELOPMENT.md](../DEVELOPMENT.md) | 架构与开发手册 |
| [DEPLOYMENT.md](../DEPLOYMENT.md) | 部署与运维 |
| [architecture-constraints.md](./architecture-constraints.md) | 目录边界与命名约束 |
