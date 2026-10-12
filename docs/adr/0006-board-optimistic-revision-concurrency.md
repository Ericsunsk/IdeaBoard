---
status: accepted
owner: Board
decision_date: 2026-09-11
supersedes: none
---

# Board optimistic revision concurrency

Board writes 使用 optimistic revision concurrency。Client 发送 observed document revision，Backend 原子比较 persisted revision；stale writer 收到 HTTP 409，不能以 last-write-wins 覆盖文档。

当前 wire contract 在 reads 返回 `ETag`，writes 使用 `If-Match`。Persisted document 自身保留 `revision` 字段；新 Board 的 initial revision 由 API 定义。

## Contract owners

- `apps/api/openapi.public.yaml` 定义 `If-Match` request header 和 `ETag` response header。
- `apps/api/internal/runtime/interfaces/http/contract_v1_boards.go` 校验 request header，并将 stale revision 映射为 `board_revision_conflict`。
- `apps/web/src/features/board/session/adapters/boardsResource.ts` 将 base revision 映射为 `If-Match`，并读取 response document revision。

Board writes 不再使用 atomic compare-and-swap，或 offline multi-writer reconciliation 需要更复杂 conflict model 时，应重新 review 此决策。
