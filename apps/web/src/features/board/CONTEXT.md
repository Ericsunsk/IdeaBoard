# Board

`board` 是完整的 Board bounded context，也是唯一的 Board repository/synchronization fact source。它拥有 Board 文档模型与 Schema、媒体引用提取、看板 CRUD 与画廊用例、协作加入、revision/乐观锁同步、持久化与预览适配器。Canvas `board-session` 只做 session 编排，必须经由本 context 的 public seam 消费，不得另起 repository 或 sync fact source。

## Language

**Board Document**:
由 `BoardState`（含 `schemaVersion`、`imageNodes`、`promptNodes`、`fusionNodes`、`connections`、`groups`、`mediaRefs`）构成的不可变/纯数据文档快照，归 `domain/document` 拥有。
_Avoid_: 将 Vue Flow 运行时实例作为文档存储，向后端发送含 runtime UI 状态的 dirty 对象

**Schema Versioning & Migration**:
文档快照强 Schema 校验（由 Valibot `stateSchema` 拥有）。从历史版本加载时执行确定性迁移，严格向前兼容。
_Avoid_: 动态猜测未知字段，未校验 Schema 的 JSON.parse 直接喂给画布运行时

**Board State Envelope**:
传输层与持久化层的安全信封 `BoardStateEnvelope`，由 `domain/schema/boardStateEnvelopeContract.ts` 归一。
_Avoid_: 裸 JSON 跨进程传递，缺少 revision / version 信封标记

**Media Reference Extraction**:
画布节点中引用的媒体资产持久化事实提取。保存 Board 时必须提取全量持久引用，供后端完成引用计数与垃圾回收锁。
_Avoid_: 保存只含临时 blob URL 的节点，未持久化 mediaAsset 引用

**Board Repository**:
看板目录的唯一事实源（`application/catalog/repository.ts`）：本地目录状态 + CRUD 命令 + 服务端 hydrate 的组合外观。`useBoardRepository()` 是跨层消费的单一入口。
_Avoid_: second repository, session-owned catalog truth

**Board Sync & Revision**:
乐观锁同步协议（`application/sync/boardSync.ts`）。每次保存携带 `base_revision`，服务端返回单调递增 `new_revision`；409 冲突进入 rollback/reconcile 流程，多端编辑不发生静默覆盖。
_Avoid_: 无 revision 保存，冲突时直接盲覆盖（last write wins）

**Share Join**:
分享协作加入用例（`application/collaboration/joinSharedBoardWorkflow.ts`）：token 校验、加入、hydrate。
_Avoid_: 页面自建加入传输

**Board Preview**:
看板缩略图生成器（`infrastructure/preview`）。根据节点布局与关键渲染图生成轻量 preview snapshot data URL，供画廊卡片消费。
_Avoid_: 在画廊列表全量拉取完整 board state 渲染缩略图

**Catalog Gallery**:
画廊 UI（`presentation/catalog`）：`BoardsPage`/`BoardCard`、gallery controller、command handlers、feedback 与 skin。纯视图编排，数据经 repository。
_Avoid_: 页面直调 generated client，画廊自建目录事实

## Module Ownership

| Module | Responsibility | Public Interface | Implementation Detail Boundary |
| :--- | :--- | :--- | :--- |
| `domain/document` | 纯节点定义、快照模型、融合比例、尺寸与长宽比规则。 | `domain/index.ts`（经 `document/index.ts`）。 | 严禁引入 Vue、Router 或运行时 DOM，纯函数驱动。 |
| `domain/schema` | 快照 Schema、信封契约。 | `domain/index.ts`（经 `schema/index.ts`）。 | 同上。 |
| `domain/media` | 媒体引用提取（`references.ts`）。 | `domain/index.ts`（经 `media/index.ts`）。 | 同上。 |
| `application/catalog` | 目录 repository 外观、CRUD 命令、本地目录 store。 | `useBoardRepository()`、`createBoardRepository()`（经 `catalog/index.ts`）。 | 不直接 import generated-query；传输经 `infrastructure/api`。 |
| `application/sync` | 服务端 hydrate/同步/删除与冲突分类。 | `hydrateBoardsFromServer`、`syncBoardToServer`、`isBoardNameConflictError`。 | 传输经 `infrastructure/api/boardsResource`。 |
| `application/collaboration` | 分享加入工作流。 | `joinSharedBoardByToken`。 | 同上。 |
| `infrastructure/api` | Board 传输适配：boards CRUD + share join + DTO 重导出。 | `boardsResource.ts`（`fetchUserBoards`、`saveUserBoard`、`removeUserBoard`、`joinUserBoardByShareToken`）。 | 本层是唯一可 import generated-query boards 边界的地方。 |
| `infrastructure/persistence` | 序列化/反序列化、版本迁移适配、内存快照 map。 | `serializeBoardState`、`parseBoardState`（经 `persistence/index.ts`）。 | 兼容旧字段与空默认值修补，输出稳定合规模型。 |
| `infrastructure/preview` | 预览图生成与刷新队列。 | `renderBoardPreviewDataUrl`、`createBoardPreviewRefreshQueue`。 | 预览降级算法留在内部，对外输出稳定 Data URL。 |
| `presentation/catalog` | 画廊页面、卡片、gallery controller、feedback、skin、format。 | `createCatalog`、`useCatalog`、`BoardsPage`（经 root `board/index.ts`）。 | 纯视图组合，不拥有目录事实。 |
| `testing` | 测试工厂（snapshot/state 构造）。 | `board/testing`（仅测试引用）。 | 不得进入生产 barrel。 |

跨 feature 消费统一走 root `board/index.ts`；`board/testing` 仅测试引用。Canvas `board-session` 经本 seam 编排 session 生命周期。

## Rules

- `domain` 必须保持纯净：无 Vue 响应式依赖，无浏览器 DOM 依赖。
- 有且仅有一个 repository/sync fact source：本 context 的 `application/catalog` + `application/sync`。Canvas `board-session` 只做 orchestration。
- generated-query boards 边界只允许 `infrastructure/api/boardsResource.ts` import；application/presentation 经 adapter 消费。
- 画布保存必须是事务一致的快照；节点内的图片引用必须先持久化为 `mediaAssetId`，严禁将未上传的本地 Blob/Object URL 作为永久图层落库。
- 同步写操作必须携带 `revision`，冲突处理以领域策略为准。
- Schema 演进必须严格保留历史版本的加载迁移函数（如 `latestBoardState.ts`），禁止原地破坏旧看板数据。
- Protected contracts（persisted schema、API 语义、storage keys、route URLs、序列化语义）见 `docs/architecture/frontend-contract-inventory.md`，重组期间不得静默变更。
