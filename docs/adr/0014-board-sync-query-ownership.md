---
owner: Board
decision_date: 2026-10-11
supersedes: none
---

# Board Sync Query Ownership

## Context

Board 同时管理跨视图共享的 server snapshots 和必须在刷新与冲突后保留的 local drafts。二者具有独立的 ownership boundary。

## Decision

### Source of truth 与 Query keys

TanStack Query 拥有 catalog list 和每个 board 的 server document snapshot；Board store 拥有 draft overlay。Workspace selection port 提供 scope，缺少 workspace 时在任何 network call 前 fail closed。

- List：`['boards', workspaceId, 'list']`。
- Document：`['boards', workspaceId, 'board', boardId, 'document']`。
- Access fact：`['boards', workspaceId, 'board', boardId, 'access']`（Share 面板，Creator only）。
- Scope root：`['boards', workspaceId]`。
- Workspace roster 由 Workspace module 统一 transport、parser 和 Query options，使用 generated workspace-members key（`['workspaces', workspaceId, 'members']`）。Workspace settings 负责邀请；Share 仅将同一 roster 投影为 Creator 与已选成员，不建立第二份缓存。

Gallery 只读取 catalog list，不为 thumbnails 获取完整 document。Canvas 通过 Board port 消费 document 与 draft overlay。Scope switch 取消旧请求并丢弃 late response；旧 response 不能降低 cached document revision。

### Freshness 与 Focus

Catalog list 使用 `accountScoped` preset（stale 15s）；document 使用 core default（stale 5s）。`gcTime` 10m 和 `retry 1` 继承 singleton Query client，feature 不创建第二个 QueryClient。

Workspace roster 使用 `accountScoped`（stale 15s）和 `noRetry`，Workspace settings 与 Share 共享相同的数据形状、严格 role 校验与 display-only email/avatar。

Global `refetchOnWindowFocus` 为 `false`。Gallery 的 visible touch 经 5s throttle 后显式 invalidate，hidden tab 不 hydrate；document 隐藏时 autosave flush pending save。

### SSE invalidation

SSE 只提供 invalidation signal，不提供 document snapshot：

- `board.document_replaced` 经 500ms coalesce 后 invalidate document，revision 小于或等于 observed revision 的 frames 丢弃。
- Dirty 或 saving draft 只 mark stale，不 refetch、不覆盖 draft。
- `board.access_changed` 使用接收者身份与已提交的 `access_version` 定向通知 Creator 以及原／新选中成员，触发访问事实刷新；确认失权后才清理受保护内容。
- `board.access_changed`（#940 定向信号，只发给被影响成员）触发访问重验证与 list invalidation，并使用 per-board access-version watermark：小于或等于已见 access version 的 frames 丢弃。该 watermark 只给 access 信号排序，不给 document revision 排序；Workspace membership 生命周期变化（leave/remove/rejoin）不经过该 version，经 scope resubscription + gap invalidation 收敛。
- 信号只触发重验证，从不直接收敛（#941）：打开中的 board 收到 `access_changed`、SSE gap、重连或 hidden-tab 恢复可见时，只对该 board 做一次 access revalidation（board meta 重读）。确认失权（`board_forbidden` / `board_not_found` / `workspace_not_found`，或裸 403/404）才走确认撤权清理；其余一律保留 draft 与重试。敏感操作（task submit）前若 access-stale flag 置位（gap/reconnect/可见恢复设置），先重验证再执行，每轮 stale 只验证一次。
- Gap 只 invalidate workspace scope root，保持其他 workspace 与 tasks cache。
- Event payload 即使带有 document 字段，也不能用于 `setQueryData` 写入 document content。

### Revision 与 404

Document save 将 observed revision 作为 `If-Match`。HTTP 409 保留 draft、base revision 与 conflict classification，直至显式 reload。Archived write 使用 banner path；rename 只 PATCH name，不写 document，并采用 server revision。

Document PUT 404 不触发 create 或 upsert，保留 local draft 和 base revision；never-synced draft 保持无 revision。Document GET 404 使 hydrate 失败并保留 draft 与 revision。只有成功取得的完整 catalog list 确认 absence 时才移除 previously synced board，never-synced offline creation 继续保留。

### 确认撤权清理（#941）

以上普通 404 preservation 规则保持不变，不做任何静默放宽。确认失权是独立的显式路径，只有 revalidation（board meta 或 access fact 重读）返回 `board_forbidden` / `board_not_found` / `workspace_not_found`（或裸 403/404）时才进入；document GET/PUT 的 404 本身只触发一次重验证，从不等同于确认。

确认后 Canvas：停读写、autosave 与生成（含 gate 住已调度的 retry 与 submit 入口，会话 registry 按 User + Workspace + Board 隔离并随 scope 切换清空）；关 Share；`removeQueries` 受保护 document/access 并 invalidate list；移除本地 catalog 条目、持久化 board state、server-content baseline 与 rollback markers；释放 workspace display-lease 缓存；`router.replace` 返回有权 Gallery；不自动复制未保存内容。late response 经 scope 检查与 verdict-drop-on-move 丢弃；用户/Workspace 切换清空 registry 与相关 markers，不污染新 session。

Wire contract 由 [ADR-0006](./0006-board-optimistic-revision-concurrency.md)、`apps/api/openapi.public.yaml`、`DESIGN.md` 和 `features/board/session/adapters/boardsResource.ts` 定义。

### Draft overlay

Local-newer draft 优先于旧 remote snapshot；显式 rollback 或 `preserveLocalNewer: false` 才收敛到 remote snapshot。Overlay 固定到请求捕获的 scope，workspace switch 后丢弃 late response。Server-content baseline 与 live dirty/saving guard 保护 draft。

## Verification

永久 contract specs 位于 `apps/web/src/features/board/session/sync/`：

- `boardSourceOfTruth.contract.spec.ts`：scope fail-closed、server/draft 分离、revision 单调和 workspace navigation race。
- `boardCacheTiming.contract.spec.ts`：freshness 与 gc boundary。
- `boardFocusControls.contract.spec.ts`：visible touch、hidden tab 和 pending save flush。
- `boardSseMapping.contract.spec.ts`：coalesce、revision guard、dirty draft、member/gap scope。
- `boardRevisionConflict.contract.spec.ts`：409、explicit reload 和 GET/PUT 404。
- `boardDraftOverlay.contract.spec.ts`：draft precedence、rollback、offline creation 和 late response。
- `../revocation/accessLoss.contract.spec.ts`（#941）：重验证专用的 loss 分类（确认 vs 保留矩阵）、registry/scope 隔离、mid-flight verdict 丢弃、open-board 观察（定向/roster 信号只重验证打开中的 board、gap 重验证、board 切换丢弃 late verdict）。
- `../revocation/convergeAccessLoss.contract.spec.ts`（#941）：确认清理（registry 标记、受保护 Query 移除、本地副本丢弃且不复制、surface 通知隔离、无 scope 不收敛、跨 workspace 隔离）。

```sh
cd apps/web
npm run test:run -- src/features/board/session/sync
```

## Consequences

Canvas、Search 和 Library 不直接使用 Query，通过 feature ports、stores 和 SSE coordination 消费状态。Key shape、freshness/gc、focus、SSE mapping、revision wire contract 或 draft precedence 的变更必须更新 contract specs 并经过 review；ownership 决策变更通过新 ADR 明确 supersede 本文。

## Status

Accepted
