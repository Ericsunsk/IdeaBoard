---
status: accepted
---

# ADR-0040: 用户通知中心架构与领域数据模型 (User Notifications Center)

## Context

在 IdeaBoard 当前架构中，用户在协作、资产交付、组织流转及计费结算等业务场景下产生了一系列关键异步事件（例如：收到 Workspace 成员邀请 #957、收到 Studio Handover 移交邀请 #948、账户额度预警、充值到账确认、新成员加入等）。在此之前，系统缺乏统一的站内通知中心（User Notifications Center），用户无法集中查看与自己相关的待处理事项和状态变动，依赖于被动的页面重载或第三方外部渠道（如邮件），导致用户交互断层并增加了跨端同步复杂性。

Issue #958 明确要求建立统一的站内通知中心系统。为满足高性能、可靠触达、操作闭环以及长期存储健康度，本架构必须解决以下核心设计问题：
1. **统一领域数据模型**：在保证用户数据强隔离的前提下，提供高效的未读统计、分页拉取与状态流转。
2. **多形态通知与强类型 Action Payload**：各类业务事件（邀请、移交、计费）具有不同的动作属性与跳转目标，必须支持安全可扩展的载荷契约，避免字符串解析脆弱性。
3. **双通道触达架构（Dual Retrieval）**：既要具备实时感知能力，又要防止 WebSocket/SSE 成为状态事实源而引入丢消息或重放一致性风险。
4. **数据存储保留策略（Retention Policy）**：通知数据具有时效性，需要明确定义自动过期与清理机制，防止历史数据无限增长导致单表膨胀与索引退化。

## Decision

### 1. 领域数据模型 (Domain Model)

在数据库层引入统一通知事实表 `user_notifications`，采用 Dual-Key 策略（`internal_id` BIGINT 自增主键用于内部高效检索，`public_id` TEXT 携带 `notif_` 前缀对外暴露）：

- `internal_id` (BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY)
- `public_id` (TEXT UNIQUE NOT NULL, `CONSTRAINT chk_user_notifications_public_id CHECK (public_id LIKE 'notif\_%')`)
- `user_id` (BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE)
- `type` (TEXT NOT NULL)
- `title` (TEXT NOT NULL)
- `body` (TEXT NOT NULL)
- `action_payload` (JSONB NOT NULL DEFAULT '{}'::jsonb)
- `deduplication_key` (TEXT NOT NULL DEFAULT '')：内部事件幂等标识，不暴露到 API。非空时 `(user_id, deduplication_key)` 唯一；重放保留原通知、已读与归档状态。
- `status` (TEXT NOT NULL DEFAULT 'unread' CHECK (status IN ('unread', 'read', 'archived')))
- `read_at` (TIMESTAMPTZ NULL)
- `created_at` (TIMESTAMPTZ NOT NULL DEFAULT now())
- `updated_at` (TIMESTAMPTZ NOT NULL DEFAULT now())

#### 索引设计
为支持未读数聚合统计、按状态分页拉取与时间倒序扫描，创建以下复合索引：
- `idx_user_notifications_user_status`: `(user_id, status, created_at DESC)` —— 覆盖特定状态查询（如 `status = 'unread'` 或未读数统计）；
- `idx_user_notifications_user_created`: `(user_id, created_at DESC)` —— 覆盖用户通知流全量时间倒序分页；
- 数据保留策略索引：可通过 `created_at` 检索老旧数据执行批量归档或物理清理。

### 2. 通知分类 (Notification Types)

首期支持以下五种核心领域通知类型：

| Type | 触发场景 | 业务实体关联 | 典型动作 |
| :--- | :--- | :--- | :--- |
| `workspace_invite` | 用户被邀请加入特定工作区 (#957) | `wsi_...` (`PrefixWorkspaceInvite`) | 弹窗查看邀请详情、接受或拒绝邀请 |
| `studio_handover` | 收到 Studio / 计费账户移交邀请 (#948) | `bti_...` (`PrefixBillingTransferInvite`) | 前往移交确认页面、接受或拒绝移交 |
| `billing_credit_low` | 计费账户积分余额低于预警阈值 | `ba_...` (`PrefixBillingAccount`) | 快捷跳转充值/订阅页面 |
| `billing_recharge_success` | 充值订单支付成功到账 | `ba_...` / 订单公共标识 | 查看账户余额及交易流水明细 |
| `workspace_member_joined` | 新协作者接受邀请加入工作区 | `ws_...`, `usr_...` | 刷新工作区成员列表、查看协作者资料 |

系统保持开放性，后续可通过新增 `type` 枚举常量扩展更多通知类型。

### 3. 生命周期与状态机契约 (States & Lifecycle)

通知状态机包含三种状态：`unread`（未读）、`read`（已读）、`archived`（已归档）。

```
        ┌───────────────┐
        │    unread     │
        └───┬───────┬───┘
            │       │
    (已读)  │       │ (归档)
            ▼       ▼
    ┌──────────┐  ┌──────────┐
    │   read   ├──┤ archived │
    └──────────┘  └──────────┘
```

- **`unread`**：通知生成后的初始状态。`read_at` 保持为 `NULL`。未读通知计入用户的全局未读红点计数（Unread Badge Count）。
- **`read`**：用户已浏览或点击通知。流转至 `read` 时，原子更新 `read_at = now()` 与 `updated_at = now()`。支持单条标为已读与全部标为已读（`MarkAllNotificationsAsRead`）。
- **`archived`**：用户主动归档或移除通知。不再在默认通知列表中显示，仅在归档视图或过滤归档状态时可见。已归档状态可直接由 `unread` 或 `read` 流转。

### 4. 动作载荷契约 (Action Payload Contract)

`action_payload` 字段采用 JSONB 存储，提供强类型的操作上下文，严禁在 `title`/`body` 中嵌入易碎的实体 ID 字符串。

载荷规范：
1. **实体公共标识引用**：必须引用标准前缀的 Public ID（如 `wsi_...`、`osi_...`、`bti_...`、`ba_...`、`ws_...`），禁止泄露内部数据库自增 ID。
2. **按通知类型定形的载荷（本节为事实源；`dispatcher.go` 实现与本节一致）**：
   ```jsonc
   // workspace_invite（受邀人可操作行）
   { "invite_id": "wsi_...", "workspace_id": "ws_...", "workspace_name": "...", "role": "member|admin", "invite_status": "pending|accepted|declined|cancelled" }
   // organization_invite（受邀人可操作行）
   { "invite_id": "osi_...", "organization_id": "org_...", "organization_name": "...", "role": "member|admin", "invite_status": "pending|accepted|declined|cancelled" }
   // workspace_member_joined / organization_member_joined（发给邀请人的决议通知，不可操作）
   { "invite_id": "wsi_...|osi_...", "member_user_id": "usr_...", "role": "..." }
   // workspace_invite_declined / organization_invite_declined（发给邀请人的决议通知，不可操作）
   { "invite_id": "wsi_...|osi_...", "invitee_email": "..." }
   // studio_handover / billing_*（跳转类，不做内联决议）
   { "transfer_invite_id": "bti_...", "workspace_id": "ws_...", "workspace_name": "...", "from_user_id": "usr_..." }
   ```
3. **邀请决议的 `invite_status` 写路径**：创建邀请通知时写 `"invite_status": "pending"`；受邀人 accept/decline 决议后，后端经 `MarkInviteNotificationResolved` 把同一 `invite_id` 的可操作行（`type IN ('workspace_invite','organization_invite')`）合并更新为 `accepted`/`declined`；发起人 cancel 时合并为 `cancelled`。前端 `notificationActionRegistry` 只读该字段做已决议渲染（`accepted|joined` → 已加入，`declined|cancelled|expired` → 已拒绝/已失效），不再把通知状态机或邀请列表当作决议事实源。
4. 允许客户端根据 `type` 与 `action_payload` 原生渲染快捷交互按钮（如“接受邀请”、“立即充值”），实现无需跳转的内联操作闭环。

### 5. 双通道触达架构 (Dual Retrieval Architecture)

系统严格采用 **“HTTP REST API 查询与状态更新 + 实时通道轻量信号唤醒（Signal-Driven Refresh）”** 的双通道模型：

```
                ┌──────────────────────────────────────────────┐
                │             Domain Services / River          │
                └──────┬────────────────────────────────┬──────┘
                       │ 1. 事务插入通知行               │ 2. 发布轻量信号
                       ▼                                ▼
            ┌─────────────────────┐          ┌───────────────────────┐
            │ `user_notifications`│          │ Realtime Signal (SSE) │
            │   PostgreSQL Fact   │          │ "notification:new"   │
            └──────────┬──────────┘          └───────────┬───────────┘
                       │ 4. REST 拉取与变更              │ 3. 触发客户端拉取
                       ▼                                 ▼
            ┌────────────────────────────────────────────────────────┐
            │                     Frontend Client                    │
            └────────────────────────────────────────────────────────┘
```

1. **事实源唯一性**：PostgreSQL 的 `user_notifications` 是状态与通知列表的唯一事实源（Source of Truth）。
   邀请 create/accept/decline/cancel 的业务变更、可操作通知状态回写及决议通知由 `tenancy/invitations.Service` 在同一 ambient transaction 内完成；任何通知写入失败都回滚整个变更。Studio Handover 创建也使用同一事务，通知幂等键源于持久请求标识，因此 HTTP 缓存过期、进程重启与 in-place reissue 均不依赖进程内 first-execution flag。邮件仅在该事务内入 River，不在持锁事务内外发。
2. **轻量信号唤醒**：Realtime 通道（SSE / NATS JetStream / Realtime 服务）仅下发轻量 Ping 信号（如 `{"event": "notification.created", "user_id": "usr_..."}`），不依赖实时连接承载长消息体或执行状态同步。这确保了在断线重连、弱网或未开启实时连接的情况下，客户端通过首屏 REST API 依然能获得完整准确的通知列表与未读数。V1 envelope 的 `workspace_id` 必须为 `ws_`（见 `ValidateV1Envelope`），无 `ws_` 归属的组织/账单通知在本轮仍走 30s 轮询，不新增 envelope 形态。
   所有通知写入统一经 `notifications.Service`，以 persistence 返回的实际通知行为准派生收件人、Workspace 和引用；批量已读与邀请决议更新也返回受影响行。HTTP 不重新拼接 hint。无事务的单条写入成功后直接发布；有 ambient transaction 时必须在写入前注册 post-commit hook，由 transaction owner 成功提交后运行，borrower 不提交也不发布，缺少有效 registry 时拒绝写入。Hints 只做 best-effort fanout，使用有时限的 detached context；回滚或提交失败不发布，不新增 outbox 或可靠投递承诺。事件 ID 包含通知行身份，更新事件再包含持久化状态，以区分不同收件人以及已读、归档、邀请决议。Actor 来自请求 principal，系统调用使用 system actor；不把 recipient 当作 actor。
3. **接口契约**：
   - `GET /v1/user/notifications`：分页获取当前登录用户的通知列表，支持 `status` 和 `category=all|invites|billing` 筛选，均在分页前生效。默认排除已归档通知，归档页显式指定 `status=archived`；响应包含 `items/total/limit/offset`。
   - `GET /v1/user/notifications/unread-count`：获取当前未读通知总数，用于顶部导航栏 Red Dot 徽标。
   - `POST /v1/user/notifications/:id/read`：单条标为已读。
   - `POST /v1/user/notifications/read-all`：批量将所有未读通知置为已读。
   - `POST /v1/user/notifications/:id/archive`：归档指定通知。

### 6. 数据保留策略 (Retention Policy)

通知属于时效性沟通数据，长期堆积无价值且会拖慢单表索引查询。确立以下数据生命周期策略：

1. **90 天保留窗口（90-Day Retention Policy）**：创建时间超过 90 天（`created_at < now() - INTERVAL '90 days'`）的已读与已归档通知进入清理窗口。
2. **定期清理任务（Retention Sweep Job）——已明确 defer，未接线**：`DeleteOldNotifications` 查询已存在但无任何 River 定时任务调用（`task_wiring.go` periodicJobs 未注册通知清理 kind），当前不执行自动物理删除。defer 期间单表只增不减：`user_notifications` 按用户时间倒序索引支撑列表/未读数，长期堆积会放大单表体积与索引扫描成本，高活跃用户首屏分页延迟随行数增长。后续接线前必须先补按时间分段批量删除 + 未读保护 + 可观测（删除行数/耗时），再解除 defer。
3. **未读保护**：对于超过 90 天仍处于 `unread` 状态的通知，系统可选择自动转为 `archived` 或在日志中记录告警，随后在安全期限后完成回收，确保活跃工作区与通知表规模受控。

### 7. 安全与多租户隔离 (Security & Multitenancy)

1. **用户级别强隔离**：通知数据完全属于接收人个人（`user_id`）。所有 SQL 查询与更新必须硬编码 `WHERE user_id = $authenticated_user_id`，防止越权（IDOR）查看或修改他人通知。
2. **级联安全**：当用户主体被物理注销或合规删除（`DELETE FROM users`）时，通过外键 `ON DELETE CASCADE` 级联清理该用户的所有通知，不留孤儿数据。

## Consequences

### Positive
- **体验统一**：各类关键协作与计费事件收拢于同一交互入口，用户对团队邀请与额度变动可即时感知。
- **高韧性**：REST API 兜底 + 实时信号唤醒，即使 WebSocket 中断亦不影响业务通知的持久呈现与操作。
- **架构解耦**：通知生成方仅需将通知记录写入 DB 并发送通知信号，与具体的通知展示与动作执行解耦。
- **存储健康**：明确的 90 天 Retention 策略防止表体无限膨胀，保障索引检索效率稳定。

### Compliance & Risks
- **Prefix 规范**：新增 `notif_` 必须严格注册至 `platform/contract/identifiers.go` 与 `tenancy/identifiers.go`，并在数据库层设置 `chk_user_notifications_public_id` 校验约束。
- **通知降噪**：上游业务在触发通知时需具备防重与合并机制（例如高频成员变动或短期重复充值），避免向用户轰炸过多琐碎通知。

## Status

Accepted
