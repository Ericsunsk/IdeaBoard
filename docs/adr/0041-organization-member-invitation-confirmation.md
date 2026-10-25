---
status: accepted
---

# ADR-0041: Organization 成员邀请确认机制与多通道流转 (Organization Member Invitation Confirmation)

## Context

在 IdeaBoard 中，此前 Workspace 成员邀请已实现了基于受邀者确认的双通道机制（ADR-0039 #957）与统一通知中心（ADR-0040 #958）。然而，Organization（组织）层级的成员添加接口（`POST /v1/organizations/:id/members`）仍为直接写入 `organization_memberships`，导致受邀者未经显式同意即被加入组织。

根据 IdeaBoard 的核心多租户架构与业务约束：
1. **单一组织归属硬约束（Single-Org Invariant）**：每个用户最多只能属于一个 Organization（`chk_users_organization` / `ErrOrganizationLimitReached`）。如果直接将用户加入组织，会剥夺用户自主决定所属组织的权利，并可能导致用户无法加入其他合规组织。
2. **多租户权限边界**：加入 Organization 意味着自动获得该组织下所有 Workspace 的有效访问权限（Owner/Admin 自动具备有效权限，Member 具备组织内工作区授予资格）。
3. **协作安全性与通知闭环**：邀请必须显式进入受邀人的待处理清单，受邀人在知情、自愿的前提下确认接受（Accept）或拒绝（Decline），并在通知中心提供即时内联操作与状态追踪。

因此，需要建立完备的 Organization 成员邀请确认机制。

## Decision

### 1. 邀请领域模型与生命周期 (Domain Model & Lifecycle)

在数据库层引入 `organization_member_invites` 数据表，对外暴露统一携带 `osi_` 前缀的 Public ID（`PrefixOrganizationInvite`）。

每个邀请具有五种状态：`pending`、`accepted`、`declined`、`cancelled`、`expired`。

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

- **`pending`**：初始状态。组织拥有者（Owner）或管理员（Admin）发出邀请，等待受邀人响应。
- **`accepted`**：受邀人在 7 天有效期内显式接受。在同一事务中：
  - 锁定用户行并校验其当前尚未属于任何组织（若已属于其他组织，返回 `409 organization_limit_reached`）；
  - 创建 `organization_memberships` 记录；
  - 将邀请状态原子置为 `accepted`，记录 `responded_at` 与 `responded_by_user_id`。
- **`declined`**：受邀人显式拒绝邀请。记录拒绝时间与响应人。
- **`cancelled`**：组织发起人或组织管理者主动撤销邀请。
- **`expired`**：自创建起满 7 天自动失效。

终态不可逆：`accepted`、`declined`、`cancelled`、`expired` 均为终态。

### 2. 安全与并发控制 (Security & Concurrency Control)

1. **单待处理邀请唯一约束（Partial Unique Index）**：
   - `CREATE UNIQUE INDEX uq_organization_member_invites_pending_email ON organization_member_invites (organization_id, invitee_email) WHERE status = 'pending';`
   - 同一组织对同一受邀邮箱，在同一时间仅允许存在一条 `pending` 邀请。
2. **防爆破与 Token 哈希**：
   - 生成 32 字节密码学安全真随机数 Token；
   - 数据库仅存储 SHA-256 哈希值（`token_hash`，长度 64）；
   - 明文 Token 仅在创建响应中返回一次用于邮件/外链，数据库不持久化明文。
3. **角色与权限约束**：
   - 仅 Organization Owner 和 Admin 可发起邀请；
   - 仅 Owner 可以邀请 Admin 角色；
   - 严禁通过邀请直接赋予 Owner 角色（所有权移交必须走 Handover 协议）。

### 3. 通知中心集成 (Notification Center Integration)

与 ADR-0040 用户通知中心集成；以下通知写入与邀请变更共享同一 PostgreSQL transaction，失败时一起回滚，不使用 HTTP 提交后的 best-effort 分发：
1. **发出邀请时**：如果受邀邮箱已关联注册用户，通过 `CollaborationEventDispatcher` 自动向其分发 `organization_invite` 类型通知：
   - `type`: `organization_invite`
   - `title`: `Organization Invitation`
   - `body`: `You have been invited to join <OrgName> as <role>`
   - `action_payload`: `{ invite_id: "osi_...", organization_id: "org_...", organization_name: "...", role: "...", invite_status: "pending|accepted|declined|cancelled" }`（创建时 `pending`；决议后由 `MarkInviteNotificationResolved` 回写，契约见 ADR-0040 §4）
2. **受邀者处理**：
   - 受邀用户在前端通知中心（Dialog / Popover）中可直接点击「接受」或「拒绝」；
   - 接受后，后端回写同一 `invite_id` 可操作行的 `invite_status=accepted`，`notificationActionRegistry` 读该字段渲染为「已加入（Joined）」徽标，并原子触发数据缓存失效与工作区刷新（刷新后仍命中，不依赖本地乐观态）；
   - 拒绝后，后端回写 `invite_status=declined`，渲染为「已拒绝（Declined）」徽标；发起人 cancel 时回写 `cancelled`，前端同按已失效隐藏操作。
3. **决议通知**：
   - 接受时，自动向邀请人发送 `organization_member_joined` 通知；
   - 拒绝时，自动向邀请人发送 `organization_invite_declined` 通知。

### 4. 仅站内兑换（In-App Only，无公开 token 链路）

组织邀请与 Workspace 邀请（ADR-0039 双通道）不同，**不提供公开 token 链路**：

1. **无公开路由**：仅暴露站内 `osi_` 端点（`GET /user/organization-invites` 查询、`POST .../accept` / `.../decline` 兑换）；不存在对标 `GET /invites/:token` 预览与 `POST /invites/:token/accept|decline` 的公开兑换路由。
2. **无邮件外发**：`CollaborationEventDispatcher.OnOrganizationMemberInvited` 仅在受邀人已注册时写站内通知，不发送任何邀请邮件（email sender 仅 Workspace 侧存在）。因此不需要公开落地页；任何邮件模板都不得嵌入组织邀请 token 链接（现存 `teamInvite` 模板仅 board 协作场景使用，与组织邀请无关）。
3. 如未来需要邮件外发场景，必须先补齐对标 Workspace 的公开 token 路由（预览 + accept/decline）与落地页，再放行邮件模板改动。

### 5. 列表与过期语义（与 Workspace 对齐）

1. **Owner 列表仅返回待处理项**：`ListOrganizationMemberInvites` 与 `ListWorkspaceMemberInvites` 语义对齐，均只返回 `pending` 且未过期的邀请（可操作的 cancel 集合）；已决议（accepted/declined/cancelled）与过期行不出列表，审计历史保留在邀请行本身。
2. **GetByToken 过期为内存标记**：`GetOrganizationInviteByToken` 与 `GetWorkspaceInviteByToken` 对过期 `pending` 行仅做内存态 `expired` 标记，不落库；过期行的落库发生在 accept/decline/cancel 写路径的懒更新中。调用方不得把 token 预览行当作决议后的新鲜事实，决议成功后必须按 public ID 重读行再发事件。

## Consequences

- 彻底消除未经同意直接强加组织成员身份的安全与隐私风险。
- 严格保证每个用户最多属于一个组织的系统不变量，并在受邀人接受时进行悲观锁排他校验。
- 与现有 Workspace 邀请及 Notification Center 体验形成高度一致的交互标准。
