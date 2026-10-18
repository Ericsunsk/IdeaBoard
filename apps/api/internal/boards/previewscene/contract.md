# Board Preview Scene Contract v1

Board Preview Scene 是 Gallery 缩略图的唯一投影。本文定义 wire contract：

- Go producer：`scene.go`（`Extract`、`ApplyBudgets`）与 `derive.go`（`DeriveCatalog`）。
- Strict consumer：Go `ParseScene` 与 `apps/web/src/features/board/core/preview/scene.ts` 的 `parsePreviewScene`。
- 共享 fixtures：`testdata/*.json`。

## Scene 与版本

Scene 是 accepted Board Document 的有界纯数据投影，只包含 geometry 与 media identity。
Wire 使用 snake_case，前端逐字段映射为 camelCase。Scene 不携带 runtime URL、Blob、base64、raster payload、HTML/SVG markup 或完整 Prompt 文本。

`version` 固定为 `1`。Parsers 拒绝未知字段与其他版本；contract 变更必须同步提升版本和更新 Go、TypeScript consumer。
Extraction 仅接受 `SnapshotSchemaVersion`（`4`），不猜测或转换其他 schema。

## Snapshot 与节点

Extraction 输入为 `schemaVersion`、`imageNodes`、`promptNodes`、`fusionNodes`、`connections`、`groups`。
只解码 Scene 需要的字段，检查整数 ID、有限坐标和正数尺寸；默认宽度分别为 280、280、286。
其他 Snapshot 字段由 Board schema 负责校验。

节点按 image → prompt → fusion 排列，各类型内部保持 Snapshot 顺序。
节点 ID 为 `{connection-kind}-{numeric-id}`，与 group membership 一致。

| kind   | Geometry                                                                                      | Media                                               |
| ------ | --------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| image  | 高度为 `round(width × ratio)`，优先使用最新可用 output ratio，其次 source ratio，缺省高度 280 | source 与最新 output，最多 2 个；呈现时 output 优先 |
| prompt | 正方形；只保留 `{aspect_ratio, has_inputs}`                                                   | numeric input identity，最多 4 个                   |
| fusion | 正方形；`blend` 限制在 [0,1]，默认 (0.5,0.5)                                                  | input 最多 4 个及最新 output                        |

Media identity 至少包含 opaque `asset` 或正整数 `asset_numeric`。
Tier 使用 Snapshot 中有效的值，缺省为 `preview`；Gallery tier 独立于 Canvas zoom。
Role 为 `source`、`input`、`output`，`index` 保留位置。

## Geometry 与顺序

Bounds 只覆盖 node frames，group rect 不扩大布局。空 Board 为 `bounds: null` 与零 extent。
有节点时，尺寸至少为 1，使用统一比例 `s = 1/max(width,height)` 归一化。
Go producer 按 `(v - min) * s`、`w * s`、`w / h` 的运算顺序计算，使用 IEEE-754 double precision；共享 fixtures 固定结果。
负坐标移至原点，重叠与 Snapshot 顺序保持不变，extent 至少一边等于 1。
空 group 保留 `is_empty`，group rect 可以超出 unit range。

Connection endpoints 为 source 右侧中心至 target 左侧中心，使用同一 normalized space。
Extraction 丢弃缺失端点的 connection；strict parsers 拒绝 dangling connection。

## Content identity 与持久化

`content_version` 是 accepted Document canonical JSON 的 lowercase hex SHA-256，与 optimistic-lock revision 独立。
Canonicalization 保留 number literals 与 array 顺序，对 object keys 排序后重新编码；JSONB 的空白和 key 顺序变化不会改变 identity。

`DeriveCatalog` 是唯一持久化投影规则，供接受 Document、读取缺失投影和 backfill 共用：

- Document、content identity、Scene 在同一 PostgreSQL transaction、同一 `board_documents` row 更新；conflict 或失败不发布投影。
- Rename/archive 只更新 `boards`，不得改变 content identity 或重建投影。
- 不可提取的 schema 仍可保存 Document，对应 Scene 为 null。
- Gallery 以 content identity 判断内容变化；Scene 不包含 theme、color 或 token，切换 theme 只改变 renderer 呈现。

`board_documents` 保存 nullable `content_version`、nullable JSONB `preview_scene` 和非空 `schema_version`。
Catalog 在权限过滤后分页，只读取当前页的 metadata 与 Scene，不读取完整 Document。
Document endpoint 遇到缺失 identity 或 Scene 时从 stored bytes 纯计算，不写回。

## Budgets

`budgets` 记录 `max_nodes`、`max_connections`、`max_media_refs`、`max_scene_bytes`；null 表示未施加该项限制。
`omitted` 记录被预算省略的 nodes、connections、media refs、groups、group members，`truncated` 标记任意省略。

`CatalogLimits` 固定为 500 nodes、1000 connections、1500 media refs、65536 bytes
（唯一来源：`scene.go` 的 `CatalogMaxNodes`、`CatalogMaxConnections`、`CatalogMaxMediaRefs`、`CatalogMaxSceneBytes`；改值只改该处）。
所有持久化和返回的 Catalog Scene 都经 `DeriveCatalog` 执行这些限制。

Enforcement 保持 Snapshot 顺序：

- Node cap 保留前缀，同时丢弃悬空 connections；media cap 按节点顺序累计。
- Byte cap 先缩短 group 前缀（连同完整 members），再缩短 node 前缀，每步保留能容纳的最长前缀。
- Full-layout bounds 不变；被省略节点的 media 计入 `omitted.media_refs`。
- 包括固定字段在内的完整 JSON 必须满足 byte cap，否则不发布 Scene。

Extraction 不等待 media 下载，不授予 media 权限，也不改变 GC rules。
Media readiness、binary decode 与恢复由 Gallery media loader 负责。

## 缺失投影修复

`api backfill-board-previews` 从 canonical Document 为缺失 content identity 的行计算投影。
它按 internal ID 分批、幂等、可恢复，不修改 Snapshot、revision 或排序时间；不支持提取的 schema 保持 Scene 为 null。

- `--dry-run` 只报告首批，不写入。
- `--batch-size` 为 1..1000；`--max-boards` 为本次处理上限，0 表示无上限。
- 写入以 `content_version IS NULL` 为条件，并发运行不会覆盖已接受的新 Document。
