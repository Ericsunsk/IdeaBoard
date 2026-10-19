---
owner: Realtime
decision_date: 2026-10-19
supersedes: none
---

# Realtime Presence Sidecar

## Context

Board 需要多人 cursor/selection presence，但文档写必须保留
[ADR-0006](./0006-board-optimistic-revision-concurrency.md) 的 optimistic
revision + 409 语义：任何经 Y 层写入文档的第二通道都会绕开 revision
比较，造成 silent last-write-wins。需求是 presence-only 的 sidecar
（`apps/realtime`，y-websocket 形态），与文档同步面彻底解耦。

## Decision

1. **Presence-only。** Sidecar 只跑 awareness 协议（cursor/selection），使用
   标准件 `yjs` + `y-protocols`（sync/awareness）+ `lib0` framing，不自创
   任何 message type。`room = bid`，单副本内存 room，不做持久化、不扩展副本。
2. **Y 层不写文档。** 服务端 Doc 永久为空，只做 awareness 挂载点：
   SyncStep1 只读应答（让未改动的 y-websocket 客户端完成握手），SyncStep2 /
   Update 一律丢弃并计入 `ignoredDocUpdates`（`/health` 可见）。文档写仍归
   ADR-0006：走 Go HTTP API 的 `If-Match` + 409，Y 层永远不收敛内容。
3. **接入鉴权。** 客户端连 `/realtime?ticket=`；
   ticket = `base64url(payload).base64url(hex hmac-sha256)`，
   payload `{bid, av, cap, exp, uid}`，
   HMAC-SHA256 secret 取 `RT_TICKET_SECRET`（与 Go mint 端同名）。
   先本地验签 + `exp`，再调 Go 内网 `GET /internal/realtime/admit`
   （`X-Internal-Key` 取 `AUTH_INTERNAL_KEY`，与 API/Auth 共用同一 internal key，仿 `apps/auth` internal 调用：单发、2s 超时、
   fail-closed），admit 才进 room。请求以 `?ticket=` 为主契约（`bid/av/uid`
   随附 echo 校验）；Go 一律 `200 + {code: 0, data: {admit}}` 应答，sidecar
   只认 `admit === true`。拒绝沿用 y-websocket 关码约定
   （4400–4499 = permanent）：ticket 问题 `4401`，admit 拒绝 `4403`。
4. **踢人本期只到 ticket 窗口 + 重连失效。** 存量连接不做 admit 轮询；
   短 `exp` ticket + 重连必验即是撤销机制。phase-2 可加存量连接的 admit
   定时重查并以 `4403` 关闭（`server.ts` 已留注释）。

## Consequences

- 正面：presence 与文档写物理隔离，ADR-0006 的 409 语义不受第二通道威胁；
  awareness 行为与上游 y-websocket server 一致，客户端无需定制。
- 代价：单副本内存 room 是扩展上限；多实例 fanout、admit 轮询、Go
  `/internal/realtime/admit` 落地前，非法/过期身份只能靠 ticket 窗口收敛。
- 后续替代关系：本 ADR 不 supersede 任何 ADR（0006/0014 继续有效）；若未来
  以可扩展后端（如 `@y/hub` 系）或 Go 原生 presence 替代本 sidecar，须经新
  ADR 明确 supersede 本文。

## Status

Accepted, partially superseded by [ADR-0020](./0020-board-ydoc-collaboration.md):
Y 层更新不再一律丢弃——`cap=write` 连接的 SyncStep2 / Update 落 room Doc、
广播给 peers 并持久化到 Go（`ignoredDocUpdates` 只计无写权限/畸形/超限丢弃）。
接入鉴权、awareness 标准转发、ticket 窗口踢人模型维持不变；上文原文冻结。
