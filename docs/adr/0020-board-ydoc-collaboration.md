---
owner: Boards
decision_date: 2026-10-19
supersedes: 0006-board-optimistic-revision-concurrency.md (collaborator path only)
---

# Board Y.Doc collaboration

## Context

Board 需要多人实时协同写文档（对标 TapNow 架构 §3 CRDT 同步 + §6 双通道
hydration：`Y.Doc` + `getMap('nodes')` / `getMap('edges')` keyed by id，HTTP
快照首屏 + WS 增量收敛）。[ADR-0006](./0006-board-optimistic-revision-concurrency.md)
的 optimistic revision + 409 只适用于单写者、last-write-wins 禁止的场景：
协作者的并发写入经 409 会互相覆盖失败，无法收敛。

[ADR-0019](./0019-realtime-presence.md) 的 presence-only sidecar
（`apps/realtime`）为此留了 room 模型（`room = bid`、ticket 接入、awareness
标准转发），但 Y 层更新一律丢弃（`ignoredDocUpdates`），文档仍走 409。
本 ADR 把该 sidecar 翻到持久化，完成 Y.Doc 后端半段。

## Decision

1. **Y.Doc 结构照抄标杆。** 一 board 一 `Y.Doc`，顶层只有
   `getMap('nodes')` + `getMap('edges')`，都 keyed by id（entry `id` 必须等于
   map key，否则该 entry 在压缩时丢弃）。不做第二套结构、不做 legacy 映射。
2. **双通道 hydration。** 首屏仍走 HTTP 快照（`GET board` + previewscene）；
   realtime room admission 先通过受限 internal read 加载最近压缩快照和按序
   update tail，再接受 websocket SyncStep2 / Update。前端协作接入须将
   `nodes`/`edges` 投影到画布；协作 board 的 Y.Doc 是唯一文档事实源，不能
   并行 whole-snapshot autosave/公开 PUT。Private board 和 local draft 继续使用
   原有 snapshot autosave。
3. **校验边界。** 压缩落快照时用现有 `ValidateV1DocumentStrict` 同规则
   （envelope 非法直接 400，整单拒绝）；`stripTransient` 白名单：
   `id/type/position/data/dimensions/measured/style` 进，
   `selected/dragging/resizing` + 其他未知字段丢弃（node 与 edge 各自多
   `source/target` 端点要求：edge 缺端点整条丢弃）。realtime 在压缩前 strip
   一次，Go 在接受事务前再 sanitize 一次（defense in depth）。
4. **落盘两件套。**
   - `board_y_updates`：一 update 一行，按 board 聚合（opaque CRDT bytes，Go
     永不解码；`cap=write` 才能写，无权限/archived fail closed）。
   - 快照压缩点：sidecar 每 N 个应用更新（默认 50）POST 一次 sanitize 快照；
     Go 在同一事务内写入 `board_documents` + 追加一行 `board_revisions`
     （不可变语义不动：每次压缩恰好一行，`parent` 链不断），同时保留 opaque
     `Y.encodeStateAsUpdate`（`payload.y_state`）以保存 clocks/tombstones。
     落库文档为 canonical Board v4 arrays，HTTP hydration 只读取其 Board 字段；
     WS 恢复先 apply 二进制 state 再 replay 有序 tail。首次 Board seed 使用固定
     保留 client identity，确保未压缩 tail 引用稳定。再清空该 board 的 update
     log。沿用现有事务模式（workspace 成员锁 → board 锁 → 写）。
5. **409 只留非协作路径。** 公开 `PUT document` 的 `If-Match` + 409 语义冻结
   不动；协作者写走 Y 收敛，压缩永不带 expected revision、永不报 409（并发
   压缩在 board 行锁上串行，last-writer-wins）。realtime 侧 4xx 永久丢弃不
   重试，5xx/网络按退避重试（失败计数进 `/health`）。
6. **不做向后兼容。** 无 legacy 双实现、无兼容 replay；未来冲突出新 ADR
   替代，不绕路。

## Consequences

- 正面：协作者并发写可收敛；update log 按 board 聚合可审计，room 重建时从
  最近压缩快照加有界、有序 update tail 恢复；409 路径零回归（公开写面一行
  未动）。
- 代价：单副本内存 room 仍是扩展上限（ADR-0019 延续）；压缩不发
  `board.document_replaced` 事件（SSE 订阅者经 WS 更新收敛，
  后续 ADR 可补事件）。
- 后续替代关系：本 ADR 部分替代 ADR-0006（协作者路径）；0006 的非协作
  409 路径继续有效。若以 Go 原生 Y 解码或多副本 fanout 替代 sidecar 持久化，
  须经新 ADR 明确 supersede 本文。

## Status

Accepted
