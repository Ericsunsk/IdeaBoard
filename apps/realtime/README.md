# Realtime Collaboration Server

`apps/realtime` 是 y-websocket 形态的协作 server：一 Doc 一 room
（`Y.Doc` + `getMap('nodes')` / `getMap('edges')`，keyed by id），跑 awareness
（identity/selection）与文档增量同步。详见
[ADR-0019](../../docs/adr/0019-realtime-presence.md)（接入与 presence）与
[ADR-0020](../../docs/adr/0020-board-ydoc-collaboration.md)（Y.Doc 协同）。

## 行为契约

- 客户端连 `/realtime?ticket=`。ticket =
  `base64url(payloadJSON).base64url(hexHmacSha256)`，
  payload `{bid, av, cap, exp, uid}`（`exp` 为 unix 秒），HMAC-SHA256 secret 取
  `RT_TICKET_SECRET`（与 Go mint 端同名同值）。
- 流程：本地验签 + `exp` → 调 Go 内网 `GET /internal/realtime/admit`
  （`X-Internal-Key` 取 `AUTH_INTERNAL_KEY`（与 API/Auth 共用同一 internal key），仿 `apps/auth` 的 internal 调用
  模式：单发 fetch、2s 超时、fail-closed）→ admit 才进 room。Go 以
  `?ticket=` 为主契约（`bid/av/uid` 随附 echo 校验），一律 `200 +
{code: 0, data: {admit}}` 应答，sidecar 只认 `admit === true`。
- room = `bid`。单副本内存 room，不扩展副本（见 compose 注释）。
- 文档面：只有 ticket `cap=write` 的连接能写 Y（其余连接的 SyncStep2 /
  Update 丢弃并计入 `ignoredDocUpdates`）。写者更新落 room Doc、广播给同 room
  peers（WS 增量收敛），并经 `POST /internal/realtime/y-updates`
  持久化到 Go（4xx 永久丢弃、`droppedTotal`；5xx/网络按退避重试，
  `retriedTotal`，耗尽记 `failedTotal`）。awareness 仍标准转发，不走持久化。
- 压缩：每 room 每 `RT_Y_COMPACT_EVERY` 个应用更新（默认 50）向 Go
  `POST /internal/realtime/compactions` 提交一次 stripTransient 后的快照；
  Go 以 `ValidateV1DocumentStrict` 同规则校验后写入 `board_documents` 并追加
  一行 `board_revisions`（同一事务），再清空该 board 的 update log。
  首屏仍走 HTTP 快照（`GET board` + previewscene，不动）；非协作写仍走
  ADR-0006 `If-Match` + 409。
- 拒绝码沿用 y-websocket 关码约定（4400–4499 = permanent，不重连）：
  ticket 问题 `4401`，admit 拒绝 `4403`。踢人本期只做到 ticket 窗口 +
  重连失效；存量连接不做 admit 轮询（phase-2 可加，见 `server.ts` 注释）。

`presence/room.ts` 的 `DurableRooms` module 统一拥有恢复、admission reservation、
write capability、queue budget、append → compaction 和回收。`server.ts` 只处理
HTTP upgrade、socket heartbeat 与 connection handle；`persist.ts` 是 Go HTTP
adapter。最后一个连接离开后，room 的 Doc 保留到已接受的写入落盘完成，再回收；
排队期间重连会等待 drain，shutdown 同样先 drain 再关闭 adapter。
compaction 在对应更新被接受时固定快照，只有其 append 及先前更新全部 ACK 后才提交，
不会把后续待写更新提前带入快照。terminal append failure 会关闭该 room 的连接、
停止其后续写入和压缩；新连接从 durable snapshot + tail 重新恢复。

## 运行方式

```bash
cd apps/realtime
cp .env.example .env
# 填 RT_TICKET_SECRET（与 Go 同值）、AUTH_INTERNAL_KEY（与 Go/Auth 同值）
npm install
npm run dev     # 本地开发，tsx watch
npm run build   # tsc typecheck + 产出 dist
npm test        # tsx --test 单测（验签/admit/protocol/durable rooms/persistence）
npm start       # 运行 dist（需先 build）
```

健康检查：`GET /health` 返回 `{code: 0, msg, data: {ok, rooms, connections,
ignoredDocUpdates, appliedDocUpdates, persistPending, persistedTotal,
persistDroppedTotal, persistFailedTotal, persistRetriedTotal, compactedTotal,
compactFailedTotal}}`。

## 环境变量

| 变量                   | 必填                                                         | 说明                               |
| ---------------------- | ------------------------------------------------------------ | ---------------------------------- |
| `RT_TICKET_SECRET`     | 是（生产 ≥32 字符、非 placeholder）                          | ticket HMAC secret，与 Go 同名同值 |
| `AUTH_INTERNAL_KEY`    | 是（生产 ≥32 字符、非 placeholder）                          | 调 Go 内网面的 `X-Internal-Key`    |
| `API_INTERNAL_URL`     | 否（默认 `http://127.0.0.1:6000`，容器内 `http://api:6000`） | Go 内网地址                        |
| `RT_HOST` / `RT_PORT`  | 否（默认 `127.0.0.1` / `6002`）                              | 监听面                             |
| `RT_Y_COMPACT_EVERY`   | 否（默认 `50`）                                              | 每 room 多少个应用更新压缩一次     |
| `RT_Y_PERSIST_RETRIES` | 否（默认 `5`）                                               | Y 落盘重试预算                     |

## 冒烟

```bash
# 1. 起一个假 Go admit（200 = 放行）
node -e "require('http').createServer((q,r)=>{r.writeHead(q.url.startsWith('/internal/realtime/admit')?200:404);r.end('{}')}).listen(6000)"
# 2. 起 sidecar（另开终端）
cd apps/realtime && RT_TICKET_SECRET=... AUTH_INTERNAL_KEY=... npm run dev
# 3. 健康检查
curl localhost:6002/health
# 4. 伪 ticket 应被 4401 关闭；合法 ticket + 假 admit 应收到 SyncStep1 回包
```

`RT_TICKET_SECRET` 测试值可用 `signTicket`（`src/admission/ticket.ts`）
现场签一个短 `exp` ticket。
