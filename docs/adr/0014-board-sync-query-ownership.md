---
status: accepted
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
- Scope root：`['boards', workspaceId]`。

Gallery 只读取 catalog list，不为 thumbnails 获取完整 document。Canvas 通过 Board port 消费 document 与 draft overlay。Scope switch 取消旧请求并丢弃 late response；旧 response 不能降低 cached document revision。

### Freshness 与 Focus

Catalog list 使用 `accountScoped` preset（stale 15s）；document 使用 core default（stale 5s）。`gcTime` 10m 和 `retry 1` 继承 singleton Query client，feature 不创建第二个 QueryClient。

Global `refetchOnWindowFocus` 为 `false`。Gallery 的 visible touch 经 5s throttle 后显式 invalidate，hidden tab 不 hydrate；document 隐藏时 autosave flush pending save。

### SSE invalidation

SSE 只提供 invalidation signal，不提供 document snapshot：

- `board.document_replaced` 经 500ms coalesce 后 invalidate document，revision 小于或等于 observed revision 的 frames 丢弃。
- Dirty 或 saving draft 只 mark stale，不 refetch、不覆盖 draft。
- `board.member_changed` invalidate list。
- `board.share_revoked` invalidate list 并 remove document query。
- Gap 只 invalidate workspace scope root，保持其他 workspace 与 tasks cache。
- Event payload 即使带有 document 字段，也不能用于 `setQueryData` 写入 document content。

### Revision 与 404

Document save 将 observed revision 作为 `If-Match`。HTTP 409 保留 draft、base revision 与 conflict classification，直至显式 reload。Archived write 使用 banner path；rename 只 PATCH name，不写 document，并采用 server revision。

Document PUT 404 不触发 create 或 upsert，保留 local draft 和 base revision；never-synced draft 保持无 revision。Document GET 404 使 hydrate 失败并保留 draft 与 revision。只有成功取得的完整 catalog list 确认 absence 时才移除 previously synced board，never-synced offline creation 继续保留。

Wire contract 由 [ADR-0006](./0006-board-optimistic-revision-concurrency.md)、`apps/api/openapi.public.yaml`、`DESIGN.md` 和 `features/board/session/adapters/boardsResource.ts` 定义。

### Draft overlay

Local-newer draft 优先于旧 remote snapshot；显式 rollback 或 `preserveLocalNewer: false` 才收敛到 remote snapshot。Overlay 固定到请求捕获的 scope，workspace switch 后丢弃 late response。Server-content baseline 与 live dirty/saving guard 保护 draft。

## Verification

永久 contract specs 位于 `apps/web/src/features/board/session/sync/`：

- `boardSourceOfTruth.contract.spec.ts`：scope fail-closed、server/draft 分离、revision 单调和 workspace navigation race。
- `boardCacheTiming.contract.spec.ts`：freshness 与 gc boundary。
- `boardFocusControls.contract.spec.ts`：visible touch、hidden tab 和 pending save flush。
- `boardSseMapping.contract.spec.ts`：coalesce、revision guard、dirty draft、member/share/gap scope。
- `boardRevisionConflict.contract.spec.ts`：409、explicit reload 和 GET/PUT 404。
- `boardDraftOverlay.contract.spec.ts`：draft precedence、rollback、offline creation 和 late response。

```sh
cd apps/web
npm run test:run -- src/features/board/session/sync
```

## Consequences

Canvas、Search 和 Library 不直接使用 Query，通过 feature ports、stores 和 SSE coordination 消费状态。Key shape、freshness/gc、focus、SSE mapping、revision wire contract 或 draft precedence 的变更必须更新 contract specs 并经过 review；ownership 决策变更通过新 ADR 明确 supersede 本文。
