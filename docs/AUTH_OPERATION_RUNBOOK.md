# IdeaBoard Auth 运维手册

本文档定义 `Go/Vue 管理后台` 与 `Better Auth Studio` 的职责边界，避免在账号与安全操作上出现重复实现和误操作。

## 目标

- 业务侧运营动作留在 IdeaBoard Admin。
- 身份与安全侧动作统一走 Better Auth 生态工具。
- 出现异常时有一致的处理路径和审计口径。

## 职责分工

### IdeaBoard Admin（Go + Vue）

适用于业务治理：

- 用户封禁与解封（`status` 软控制）。
- 角色、配额、业务权限分配。
- 业务资产维护（画板、渠道、Prompt、存储配置等）。

不在此处执行：

- 后台改密。
- 后台物理删号。
- 会话吊销与设备踢下线。

### Better Auth Studio（身份与安全）

适用于安全治理：

- 密码重置、账号资料维护。
- 会话审计与强制下线。
- 物理删号（需要彻底注销身份时）。

## 操作准则

1. 默认先封禁，再评估是否需要物理删号。  
封禁可立即阻断业务访问且保留业务数据链路，便于审计与恢复。

2. 物理删号必须走 Better Auth Studio。  
确保身份记录、会话、凭据等在同一体系下清理，避免本地业务库与 auth 库状态漂移。

3. 生产环境默认关闭公开注册。  
通过 `AUTH_REGISTRATION_ENABLED=false` 保持最小暴露面，仅在明确运营窗口内临时开启。

4. 高风险动作保留审计记录。  
建议记录操作人、时间、原因、影响用户与回滚策略。

## 推荐流程

### 场景 A：普通违规账号处理

1. 在 IdeaBoard Admin 将用户 `status` 置为封禁。  
2. 复核最近任务与共享资产影响。  
3. 若属于永久封禁，再在 Better Auth Studio 执行物理删号。

### 场景 B：账号疑似被盗

1. 在 IdeaBoard Admin 先封禁，立即阻断业务调用。  
2. 在 Better Auth Studio 吊销活跃 sessions。  
3. 进行密码重置或强制凭据更新。  
4. 完成核验后再解封。

### 场景 C：用户忘记密码

1. 优先走前台自助找回流程。  
2. 必要时由安全运维在 Better Auth Studio 人工处理。  
3. 不在 IdeaBoard Admin 执行改密。

## 上线与安全基线

- Studio 入口建议仅允许内网/VPN/IP allowlist 访问。
- 使用最小权限账号，避免共享管理员口令。
- 对删号与批量操作执行双人复核。
- 定期检查 `AUTH_RATE_LIMIT_*` 配置是否符合流量规模。

## 关联文档

- [README.md](../README.md)
- [DEVELOPMENT.md](../DEVELOPMENT.md)
- [DEPLOYMENT.md](../DEPLOYMENT.md)
- [architecture-constraints.md](./architecture-constraints.md)
