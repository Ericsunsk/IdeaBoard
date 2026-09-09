# Search

Search 是公共图文 Feed 与付费以图搜图的上下文，拥有分页 Feed、视觉引用、Library 桥接、匿名转化门与登录续跑。

## Language

**Search Feed**:
Feed 事实主（`application/createSearchFeed`）：`results/nextCursor/hasMore/loading/loadingMore/hasError/savedKeys/failedImages`，`AbortController + generation` 排序取消旧请求。
_Avoid_: useQuery-cached feed, uncancelled pagination

**Submitted Query**:
`route.query.q`，是搜索的唯一真源。`useSearchSession` 用 `immediate` watcher 驱动 `feed.load(true)`；提交与清空只经 `router.replace` 写 URL。
_Avoid_: duplicated query refs, draft as source of truth

**Vision Reference**:
以图搜图的输入引用（library 资产 ID 或外部引用 + 可选 `File/blob` 预览）。只有可序列化部分落盘，`File/blob` 永不持久化。
_Avoid_: persisted blob URL, oversized reference set

**Library Bridge / Coordinator**:
Feed 与 Library 之间的纯映射（`SearchResult/LibraryAssetView <-> VisionReference`）与发布订阅（`save/remove` 经 `libraryResource`，再 `publishMutation` 回写 feed 的 saved 态）。桥接不拥有 gallery。
_Avoid_: feed-owned library state, direct gallery mutation

**Auth Continuation**:
匿名动作（搜索/收藏/开 Library）在登录后 silicone 续跑的 pending 意图。`vision-search` 意图只放内存，其余可进 `sessionStorage`。
_Avoid_: lost post-login intent, anonymous state leak across scopes

**Scope**:
`userPublicID ?? anonymous` 的存储隔离键。登录后 scope 切换只迁移 vision 引用并 dispose 旧 scope。
_Avoid_: cross-user cache read, anonymous query gate bypass

## Module Ownership

| Module       | Responsibility                                                                                                          | Public Interface                                                                                                                           | Implementation Detail Boundary                                  |
| ------------ | ----------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------- |
| `index.ts`   | 统一公开出口：对外导出领域模型（`SearchResult`、`SearchPage` 等）与意图续行接口（`clearPendingSearchAuthAction` 等）。   | `features/search/index.ts`。                                                                                                               | 封装内部分层细节，禁止外部模块深层依赖 infrastructure。          |
| `pages`      | 唯一的路由页 `SearchPage`（`meta.public`），纯装配。                                                                    | 被 `userRoutes` 懒加载；`useSearchScreenRuntime` 组装一切。                                                                                | Page 不得直调 resource 或 feed。                                |
| `components` | 结果视口（虚拟化 masonry）、收藏弹窗、批量 dock。                                                                       | 回调式 props（`load-next-page/scroll-position`），不拥有数据。                                                                             | 滚动位置与 active anchor 由 feed 拥有，viewport 只上报。        |
| feed core    | `application/feed.ts` + `infrastructure/feedPersistence.ts`（30min TTL 快照 + scroll，快照写延后到 idle）。             | `application/createSearchFeed`；`reload` 用 `keepResultsOnNextLoad` 保卡。                                                                 | `savedKeys` 手动回填必须带 `savedMutationRevision` 防旧写覆盖。 |
| resource     | 唯一的 generated 调用处：灵感搜索、视觉抽取、失败上报、计费询价；`inFlightRequests` 去重，query 限 200 rune。           | `infrastructure/resource.ts` / `infrastructure/visionBillingResource.ts`（`sku=ai-search:vision` 纯询价展示）。             | `hold/capture` 不在 search 侧展示，只进 admin 账本。            |
| lifecycle    | 会话装配（`application/useSearchSession`/`presentation/useSearchScreenRuntime`）、vision 生命周期、lightbox、引用集合。 | `requireLogin`（设 pending + 开 overlay）、`resumePendingAction`。                                                                         | `effectScope` 按 user 会话隔离；旧 scope 必须 dispose。         |
| bridge       | Library 存/删与引用转换。                                                                                               | `infrastructure/libraryBridge`（纯映射）、`infrastructure/libraryCoordinator`（`provide/inject search:library-coordinator`）。             | 底层写经 `features/library` 公开出口，桥接只做投影与回写。      |
| persistence  | scope 隔离的 sessionStorage 原语、feed/island/vision/continuation 各自键。                                              | `infrastructure/browserPersistence` (`searchScopedStorageKey(:scope)`)；`anonymous-query-gate` 同 query 去重、上限 5。                      | vision 意图与 `File/blob` 只放内存，不得进 storage。            |

## Rules

- `?q` 是唯一真源：draft 只用于输入框，提交即写 URL，feed 只响应 URL。
- 不用 `useQuery` 存 feed/server 派生；`savedKeys` 手动回填且必须过 revision。
- 视觉引用上限 4、单文件 10MB、batch 上限 8；`blob:` URL revoke 责任在引用集合。
- 跨 feature 只碰 `auth/authOverlay`（`requireLogin`）与 `features/library` 公开出口（桥接），不拥有 gallery，不碰 Canvas。

## State Machines and Invariants

- **Feed**: `idle` → `load(reset)`（abort 旧请求，首屏骨架/SWR hydrate 旧快照）→ `idle`（`hasMore = cursor`）；`load(false)`（`hasMore && !loadingMore`）→ `loadingMore`（底部 spinner）。`AbortError` 忽略；`error` 进 `hasError`（有缓存则回滚 cursor/hasMore 保一页一致，空结果显示重试 panel）。
- **Vision**: `idle` → `submit`（有引用 + 已登录，fingerprint 去重）→ `extracting` → `ready`（抽取词写回 `submittedQuery` 并加载结果）；stillProcessing/空结果/抛错 → `idle + error` toast。删引用 → `markStale`（保结果）或清空 → `reset` 并回写 `?q`。
- **Auth continuation**：匿名 `submit/save/openLibrary` → 同 query 去重（超 5 个新 query 则拦截）→ `requireLogin`（设 pending + 开 overlay）→ 登录成功且 scope 切换 → `resumePendingAction`（文本→写 `?q` 并 consume；vision→resume 到 ready 才 consume；save→逐个全成功才 consume；open-library→`island.open = true`）。overlay 关闭 → `clearPending`。
