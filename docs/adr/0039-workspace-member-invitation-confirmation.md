---
status: accepted
---

# ADR-0039：Workspace 成员邀请确认机制与并发控制

## Context

Issue #957 要求建立 Workspace 成员邀请确认机制（Invitation Confirmation Mechanism）。
在此之前，Workspace 成员添加（`POST /v1/workspaces/:id/members`）采用直接写入 `workspace_memberships` 的模式，被添加用户无需确认即直接成为成员。为了增强协作安全性、保护用户隐私、防止垃圾邀请骚扰以及支持通过邮件和公开链接邀请新协作者，需要引入显式的邀请流与受邀者确认机制。

在现行架构与产品规则中：
1. **Personal Default Workspace 私密保护**（ADR-0038 / #952）：用户的默认个人工作区（`is_default = true` 且 `organization_id IS NULL`）仅供用户个人私有使用，绝对禁止邀请或添加任何协作者。
2. **工作区 5 人协作上限**（`MaxWorkspaceMembers = 5`）：任何 Workspace 的最大成员数硬约束为 5 人。
3. **资金与权限解耦**（ADR-0038）：Workspace 是内容与授权边界，加入 Workspace 不改变计费账户归属，Personal Workspace 由 Owner 的 Personal Billing Account 承担消费，新协作者默认不限额。

为支持双通道兑换（站内通知与外链兑换）、防止数据库凭证泄露风险，并保证高并发场景下严格不突破 5 人上限，本 ADR 制定工作区成员邀请的数据模型、状态机契约、并发安全与安全防范机制。

## Decision

### 1. 邀请生命周期与状态流转

每个邀请由 `workspace_member_invites` 数据表的一行承载，公共标识符采用 `wsi_` 前缀（`PrefixWorkspaceInvite`）。

邀请具有五种状态：`pending`、`accepted`、`declined`、`cancelled`、`expired`。

```
              ┌───────────────┐
              │    pending    │
              └───────┬───────┘
                      │
     ┌────────────────┼────────────────┬────────────────┐
     │ (受邀者接受)   │ (受邀者拒绝)   │ (发起者撤销)   │ (超时过期)
     ▼                ▼                ▼                ▼
┌──────────┐    ┌──────────┐    ┌───────────┐    ┌───────────┐
│ accepted │    │ declined │    │ cancelled │    │  expired  │
└──────────┘    └──────────┘    └───────────┘    └───────────┘
```

- **`pending`**（待处理）：初始状态。邀请已发出，等待受邀人响应且未超过有效期限。
- **`accepted`**（已接受）：受邀人在有效期内显式接受邀请。在此操作的同一事务中，创建 `workspace_memberships` 记录并原子将状态置为 `accepted`，记录 `responded_at` 与 `responded_by_user_id`。
- **`declined`**（已拒绝）：受邀人在有效期内显式拒绝邀请。状态流转为终态，记录响应时间与响应人。
- **`cancelled`**（已撤销）：邀请发起者或该工作区的管理员/拥有者在待处理期间主动撤销邀请。记录撤销时间与操作人。
- **`expired`**（已过期）：邀请创建超过 7 天有效期限（`now() >= expires_at`）。已过期的邀请不可被接受、拒绝或撤销。可在查询时动态判定失效，也可由系统清理任务或懒更新写入终态。

**终态不可逆**：`accepted`、`declined`、`cancelled`、`expired` 均为终态，不可重新回到 `pending` 或在终态之间流转。

### 2. 并发安全与 5 人成员上限不变量

系统必须确保任何时刻在工作区内：
$$\text{current\_active\_members} + \text{pending\_invites} \le 5$$

为了在多节点、高并发场景下防止超发或并发兑换冲破上限，采用悲观锁与事务保护机制：

1. **行级锁串行化**：在创建邀请或接受邀请前，在事务内调用 `LockWorkspaceMemberships`，对 `workspaces` 行施加 `SELECT ... FOR UPDATE` 互斥行锁。
2. **邀请时校验**：
   在获得锁后，事务内分别统计当前生效成员数与有效待处理邀请数：
   $$\text{active\_members} = \text{COUNT}(workspace\_memberships)$$
   $$\text{pending\_invites} = \text{COUNT}(workspace\_member\_invites \text{ WHERE status} = 'pending' \land expires\_at > now())$$
   若 $\text{active\_members} + \text{pending\_invites} \ge 5$，操作失败闭环，返回 `ErrWorkspaceMemberLimitReached`（HTTP 409 `workspace_member_limit_reached`）。
3. **接受时校验**：
   受邀人接受邀请时，同样获取 `workspaces` 的排他锁。由于该邀请本身已在 `pending_invites` 的配额占位中，在同一事务内执行 `workspace_memberships` 插入并将邀请状态更新为 `accepted`，总和保持不变，从根源杜绝并发竞争导致成员突破 5 人上限。

### 3. 安全架构与 Token 设计

为支持公开链接兑换同时防范凭据泄露，系统采用哈希存储与安全熵：

1. **高熵随机 Token**：邀请创建时，生成 32 字节的密码学安全真随机数（`crypto/rand`），Base64URL 或 Hex 编码提供 256 位安全熵。
2. **数据库单向哈希**：邀请表仅存储该 Token 的 SHA-256 摘要（`token_hash`，长度 64 的十六进制字符），并建立唯一索引。明文 Token 仅在创建成功的响应或邮件通知中一次性暴露，服务端不持久化明文，即便数据库备份泄露也无法逆向出兑换链接。
3. **兑换验证**：通过链接访问时，服务端接收客户端提交的明文 Token，计算 SHA-256 后与数据库 `token_hash` 匹配。
4. **时效约束**：邀请有效期严格限定为 7 天（`expires_at = created_at + INTERVAL '7 days'`）。任何过期的 Token 在兑换时直接拒绝。
5. **持久邮件投递**：邀请创建、站内通知与 River 邮件任务入队使用同一 PostgreSQL transaction。邀请表只保存摘要；River args 为投递暂存 AES-256-GCM 密文及主密钥版本，禁止明文 Token 入库或日志。收件人、Workspace 名称、邀请人名称和角色在入队时固定为邮件快照，重试不得从最新实体覆盖模板参数。worker 每次执行先检查邀请仍 pending 且未过期，再解密并调用 Auth 专用邮件接口；密钥版本不匹配失败关闭。Auth 从可信 public origin 构建链接，使用稳定的邀请投递幂等键调用邮件 Provider，避免重试重复投递。

### 4. 双通道兑换机制（Dual-Channel Redemption）

系统支持站内与站外两种兑换体验：

1. **站内列表兑换（In-App User List）**：
   - 针对已注册用户，登录后可查询发往其已验证邮箱（或绑定 `invitee_user_id`）的待处理邀请列表。
   - 用户在系统前端界面直接点击「接受」或「拒绝」，请求直接通过邀请的 `public_id`（`wsi_...`）操作，无需知晓或传递原始明文 Token。
   - 接口强制验证当前操作用户与邀请的目标受邀身份匹配。
2. **公开链接兑换（Public Token Link）**：
   - 邀请邮件或分享链接携带明文 Token（如 `/invites/workspace?token=...`）。
   - 匿名访问可预览邀请基本信息（Workspace 名称、邀请人昵称/头像，不泄露其他敏感信息）。
   - 兑换时若未登录则引导登录/注册；登录后验证用户邮箱与受邀邮箱一致（或允许受邀者账号绑定），核验无误后原子完成接受。

### 5. 幂等性与 Personal Default 空间保护

1. **Personal Default Workspace 严苛保护**：
   - 在任何邀请创建路径上，必须首先校验工作区属性：
     若 `workspace.is_default == true && workspace.organization_id == nil`，立即拒绝并返回 `ErrDefaultWorkspaceProtected`（HTTP 403 `workspace_default_protected`）。
   - 该保护在事务锁内再次复核，确保不可绕过。
2. **单待处理邀请唯一性（Partial Unique Index）**：
   - 数据库建立部分唯一索引：
     `CREATE UNIQUE INDEX uq_workspace_member_invites_pending_email ON workspace_member_invites (workspace_id, invitee_email) WHERE status = 'pending';`
   - 对同一个工作区和同一个受邀邮箱，同一时刻最多只能存在一个处于 `pending` 状态的邀请。
   - 重复邀请相同邮箱时：若已有未过期的 `pending` 邀请，返回冲突或幂等重放；若已有邀请已达终态（`declined`、`cancelled`、`expired`），则允许发起新的邀请。
3. **HTTP 幂等重试安全**：
   - 邀请创建与流转接口均支持 `Idempotency-Key` 请求头，结合 `control_idempotency` 确保弱网重试不会产生重复邀请邮件或状态混乱。

### 6. 列表与过期语义（与 Organization 对齐）

1. **Owner 列表仅返回待处理项**：`ListWorkspaceMemberInvites` 与 `ListOrganizationMemberInvites`（ADR-0041 §5）语义对齐，均只返回 `pending` 且未过期的邀请（可操作的 cancel 集合）；已决议与过期行不出列表，审计历史保留在邀请行本身。前端两处列表（`fetchWorkspaceMemberInvites`、`fetchOrganizationInvites`）均按 pending 集合消费。
2. **GetByToken 过期为内存标记**：`GetWorkspaceInviteByToken` 对过期 `pending` 行仅做内存态 `expired` 标记，不落库；过期行的落库发生在 accept/decline/cancel 写路径的懒更新中。调用方不得把 token 预览行当作决议后的新鲜事实，决议成功后必须按 public ID 重读行再发事件。

## Consequences

- 彻底消除未授权直接加入工作区的安全隐患，将协作者准入显式化、确认化。
- 严格保证 5 人配额不变量，将 pending 邀请计入配额消耗，防止超发配额产生超员故障。
- 采用 SHA-256 单向哈希保护公开链接 Token，遵循现代安全最佳实践。
- 完整保留邀请审计历史（创建人、受邀邮箱、最终决议时间与决议人），为团队管理提供清晰凭据。
