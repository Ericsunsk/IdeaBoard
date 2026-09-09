# Canvas Board Schema & Persistence

`canvas-board` 是 IdeaBoard 的持久化文档领域模型。它定义画布图结构快照（Snapshot）、持久化 Schema（当前为 v4）、媒体引用关系（MediaRefs）、反序列化兼容迁移、以及与服务端的版本化同步（Board Sync）。

## Language

**Canvas Board Document**:
由 `CanvasBoardState`（含 `schemaVersion`、`imageNodes`、`promptNodes`、`fusionNodes`、`connections`、`groups`、`mediaRefs`）构成的不可变/纯数据文档快照。
_Avoid_: 将 Vue Flow 运行时实例作为文档存储，向后端发送含 runtime UI 状态的 dirty 对象

**Schema Versioning & Migration**:
文档快照强 Schema 校验（由 Valibot `canvasBoardStateSchema` 拥有）。从历史版本加载时执行确定性迁移（如 v3 `adjustment` -> v4 `retouch`），严格向前兼容。
_Avoid_: 动态猜测未知字段，未校验 Schema 的 JSON.parse 直接喂给画布运行时

**Board State Envelope**:
传输层与持久化层的安全信封 `BoardStateEnvelope`，由 `boardStateEnvelopeContract.ts` 归一。
_Avoid_: 裸 JSON 跨进程传递，缺少 revision / version 信封标记

**Media Ref Extraction**:
画布节点中引用的媒体资产（`sourceMediaRef`、`mediaRefs`、`mediaAssetId`）持久化事实提取。保存 Board 时必须提取全量持久引用，供后端完成引用计数与垃圾回收锁。
_Avoid_: 保存只含临时 blob URL 的节点，未持久化 mediaAsset 引用

**Board Sync & Revision**:
乐观锁同步协议（`boardSync.ts`）。每次保存携带 `base_revision`，服务端返回单调递增 `new_revision`；遇到 409 冲突时触发 `resolveBoardSyncRevisionConflict`，确保多端编辑不发生静默覆盖。
_Avoid_: 无 revision 保存，冲突时直接盲覆盖（last write wins）

**Board Preview**:
看板缩略图生成器（`boardPreviewSerializer.ts`、`boardPreviewRefreshQueue.ts`）。根据节点布局与关键渲染图生成轻量 preview snapshot data URL，供画廊卡片消费。
_Avoid_: 在画廊列表全量拉取完整 board state 渲染缩略图

## Module Ownership

| Module | Responsibility | Public Interface | Implementation Detail Boundary |
| :--- | :--- | :--- | :--- |
| `domain/` | 纯节点定义（`nodes.ts`）、快照 Schema（`stateSchema.ts`）、信封契约、多图融合比例、媒体引用提取（`mediaRefs.ts`）。 | `domain/index.ts`（导出节点类型、Schema 解析器、`extractBoardStateMediaRefs`）。 | 严禁引入 Vue、Router 或运行时 DOM，纯函数驱动。 |
| `persistence/` | 序列化/反序列化（`boardStateSerialization.ts`）、版本迁移适配；`state.ts` 持有内存快照 map。 | `serializeBoardState`、`parseBoardState`（经 `persistence/index.ts`）。 | 兼容旧字段与空默认值修补，输出稳定合规模型。 |
| `preview/` | 预览图生成与刷新队列（`boardPreviewSerializer.ts`）。 | `renderBoardPreviewSvg`、`createBoardPreviewRefreshQueue`。 | 预览降级算法留在内部，对外输出稳定 Data URL。 |
| `catalog/` | 看板 CRUD 命令、云端同步（`boardSync.ts`）、协作加入用例。 | `catalog.ts`、`commands.ts`、`saveCanvasBoardState`（经 `catalog/index.ts`）。 | 封装 generated API，向 UI 暴露安全命令与冲突解决契约。 |

## Rules

- `domain` 必须保持纯净：无 Vue 响应式依赖，无浏览器 DOM 依赖。
- 画布保存必须是事务一致的快照；节点内的图片引用必须先持久化为 `mediaAssetId`，严禁将未上传的本地 Blob/Object URL 作为永久图层落库。
- 同步写操作必须携带 `revision`，冲突处理以领域策略为准。
- Schema 演进必须严格保留历史版本的加载迁移函数（如 `latestBoardState.ts`），禁止原地破坏旧看板数据。
