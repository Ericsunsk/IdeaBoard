---
owner: Boards
status: accepted
decision_date: 2026-10-20
supersedes: 0020-board-ydoc-collaboration.md (snapshot publication only)
---

# ADR-0022: Collaborative Board snapshot publication

## Context

ADR-0020 的默认每 50 次更新压缩策略允许短协作 session 仅留下 opaque Y update
tail。Boards catalog 的 preview scene 来自已接受的快照；反复短编辑并退出时，
画布内容可恢复，但缩略图长期为空或过期。原决策也不为压缩发布
`board.document_replaced`，已打开的 Boards 列表无法观察新投影。

## Decision

- 保留每 N 次更新的周期压缩，同时从首个未投影更新起 1 秒内排队一次压缩；
  后续更新不延长该期限。最后一个连接离开和 sidecar shutdown 时立即排队，
  不依赖所有协作者离开才生成投影。所有触发共用同一压缩队列，先按序持久化
  update，再接受对应快照；捕获快照内容和 actor，避免后续编辑污染先前排队的
  压缩。1 秒约束的是排队时机，完成仍受持久化延迟和重试影响。
- 恢复出的 durable update tail 同样标记为待投影。签名 ticket 具备 write
  capability 的 editor 打开 Board 后，可作为新 snapshot revision 的 actor，
  不必再次编辑；Go 仍事务性复核当前写权限与 media 访问。该 actor 不改变原
  update 的作者归属。只有 viewer 打开时保留 tail，不冒用 viewer 身份压缩；
  已有缺失投影须等待有写权限的 editor 打开后才可通过正常生命周期修复。
- Go 接受快照时继续在同一事务更新 document、revision、catalog projection
  并清理 update tail；事务成功后，通过公开 snapshot replacement 已有的
  workspace event pipeline 发布 `board.document_replaced`，携带实际接受的
  revision 和 author。拒绝或失败的压缩不发布该事件。
- Boards 消费该事件时刷新 document 和 catalog list cache；Canvas 保留已有的
  dirty/session guards，协作 Y.Doc 继续作为协作文档唯一事实源。禁止新增
  HTTP autosave、轮询或第二套缩略图持久化路径。

## Consequences

短 session 或其他协作者保持连接时，正常落盘均可生成新的 preview scene；现有
SSE 路径使在线 Boards 列表观察该投影。压缩失败保留 durable tail，后续编辑、
结束 session 或获准 editor 重新打开时可再次尝试。事件沿用现有 post-commit
best-effort 语义，增加一次 revision
写入成本；它不是 durable outbox，进程异常或事件传输失败时仍以重新读取的
catalog 投影为准，不承诺每次离开都立即刷新所有客户端。

本 ADR 仅替代 ADR-0020 的压缩时机与“不发布 document event”决策；其余
Y.Doc、权限、hydration、事务和非协作 If-Match 契约继续有效。

## Status

Accepted
