# 后端冗余审计报告

四片只读审计合成：`runtime/platform/db/events`、`tasking/boards/workflow/prompts/generation/media/library`、`billing/credits/rewards/payments/identity/tenancy/aigateway/searchcatalog/usage`、跨包字面量。评判以成熟 SaaS / Go 惯例为准。只记录、不动手。

## P0：语义分叉风险（优先收敛）

### 1. 结算双写：补偿路径 vs Tx 路径
- `tasking/workspacetasks/settle.go:313-371` 非 Tx 补偿分支（`allowNonTransactional`，手工 `Release/DeleteTask`）vs `settle_tx.go:243-359 createInTx` 同一事务；另 Cancel/Complete/Fail/Retry 四组双写 + `captureSplit` 双算。
- 收敛：删 `NewTestServiceForTests/allowNonTransactional` 全部分支，只留 `*InTx`；测试用 Tx fake 走同一路径。单独 PR，风险最高。

### 2. Board 姿态三重复 + 双 seam
- `workspacetasks/board_auth.go:112-130 resolveSubmitBoardAccess` vs `workspacemedia/submission_access.go:21-54` 板分支 vs `boards/taskaccess/authorizer.go:26-50 + policy.go:143-163`；消费侧 `AuthorizeTaskRead/Cancel` 又自写 owner 直通。
- 收敛：`boards` 只暴露 `ResolveBoardAccess` 一个 seam，统一 `BoardPolicy.Check(submit/read/cancel)`。

### 3. Outbox 双实现 + worker 模板 + StableID
- `events/outbox/task_outbox.go:98-185` vs `board_access_outbox.go:100-183` 逐字同（limit/pending/delivered/退避 SQL）；`Replay/Deliver` 循环仅 sink 不同。
- `runtime/interfaces/worker/outbox/` 两 worker 文件模板复制（周期/队列/预算同构）；`StableTaskEventID` vs `StableBoardAccessEventID` 同算法；`int64ToString` 手写等价 `strconv.FormatInt`。
- 收敛：抽 `genericOutboxStore{table,scanFn}` + `genericReplayer[Row]` + `events.StableEventID(parts...)`；worker 参数化 `NewOutboxWorker(kind,queue,replayer)`。

### 4. 媒体就绪门 + retry 三 taxonomy + 哨兵网
- `verifyInputAssets/verifyOutputAssets` vs `claim.go resolveMediaRefs` vs `TaskAssetAdapter.GetAsset` vs `library_publication` 四处同构门；`IsRetryable`（media codes）vs `IsRetryableProviderError`（tasking）vs `workspaceAttemptFailure` vs `routing.AttemptFailure`；哨兵在 `tasks.go/media/errors/storage/errors/imgproxy` 各一套。
- 收敛：抽 `media.GateAsset`；retry 只留 `routing.AttemptFailure`；哨兵只留任务 + 媒体两 registry。

### 5. 分页双标准 + v1 十一处复制
- `platform/pagination`（50/200）vs `platform/contract`（20/100）双标准；`contract.ParsePageSize` 在 v1 层复制 11 处；control/payments、search_handler、recharge_history 各有方言；`ListAssets(limit,offset)` 直传残留。
- 收敛：对外只暴露 `contract`（cursor+page_size）；v1 十一处提 gin helper；`limit` 方言迁移；`ListAssets` 迁 opaque cursor。

### 6. HTTP 错误映射表 ×5
- `billing/credits/rewards/payments` 各自 `*ErrorMapping+render` + `vision_input.writeVisionError` 第五实现；`msg*` 270 处散 22 文件；中文同冲突三种文案。
- 收敛：统一 `Mapping{Targets,Status,Code,Message} + RenderMapped` 泛型；`writeRewardRedeemError` 并入主表；wire code 全收 `errorcodes.go`，中文 msg 由 render 模板产出。

## P1：功能重复 helpers / validation

- Resolution 三重复（`workflow/billing/routing` + 默认 1k 两处）→ 只留 `routing.NormalizeResolution`。
- Submit validation 四重复（`tasks.go/canonical_intake/params.go/generation_contract` + 8 vs 4 上限打架）→ 只留 `workspacetasks.ValidateParams`，`paramkeys.IsPersistedMediaURL` 单谓词。
- Feature slug 四入口 → 只暴露 `ParseAIImageFeatureSlug + MustValidSlug`。
- Prompt 双读 + Fusion weights → `workflow` 只留 `Read*Contract/Normalize*Contract`。
- 结果物化下载器 + allowlist（renderer vs materializer 逐行同 + safehttp/remote 两道）→ 抽 `media/fetch.BytesFetcher + media/egress.CheckHostAllowlist`；`decodeDataURL` 三合一。
- Routing snapshot 哨兵 + 双读写 → worker 删本地 resolve，直调 `catalog.LoadRoutingSnapshot`。
- Wallet 哨兵三域（credits/rewards/payments）→ `credits/domain` 唯一源 + `MapCreditWalletError`。
- `credits.Service` 20 转发 + 双 InTx seam → 留其一；`payments.Service/Workflow` 三层嵌入 → 删中间层。
- Fixture/fake 三处逐字重复 + 别名墙 → 收 `testsupport` + 删跨包别名。
- 金额/分页/定价/vision/对账/tenancy 双 store/身份哨兵：见 P1 明细（B1–B7），逐项单 PR。
- `contract↔pagination/idempotency` 别名转发、双信封、requestID 双实现、一次性 wrapper、strict JSON 三处、`Effective*`、`Is*ID`、`cmd` 模板：见平台明细（R5–R15）。

## P2：字面量收敛（跨包）

- RBAC `owner/admin/member` 16 处 → 收 `tenancy`（board 侧用 `policy.go`，禁手写 case）。
- `board_/usr_/task_` 前缀：生产已收敛 `platform/contract`，测试 `HasPrefix` 改 `contract.Is*`，SQL CHECK 加注释对应 `Prefix*`（或 archtest 断言一致）。
- Header：`Idempotency-Key` 收 `contract.IdempotencyKeyFromHeader`（修 `user_redeem.go:66` 例外）；`Request-Id` 统一，`X-Request-ID` 仅 middleware 桥接；`Authorization` 学 `GatewayHeaderAuthorization` 模式；`X-Test-*` 60 处收 `testsupport` 注入器。
- `Content-Type` 嗅探三处 → 收 `media/imageformat.NormalizeImageMIME`。

## P3：延后

- 中文 `msg*` 表集中化、image 嗅探四合一、`V1Deduper` 与 log 去重二选一、`Effective` 泛型 clamp、NATS 三家族 codec 续抽。

## 不做

- `WorkspaceEventLog/UserEventHub` 泛型基、`V1Deduper` 现状、`limit,offset` 存量签名（待 cursor 迁移）、`tests/` 集成目录结构。
