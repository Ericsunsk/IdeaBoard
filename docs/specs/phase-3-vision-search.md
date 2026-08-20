# 阶段三：Vision Search Core

状态：已确认，待实现

## 目标

在现有 Search 页面上增加一次性多图 Vision Search：用户可以上传本地图片，或从 Personal Asset Library 选择已保存素材；系统在一次请求中结合可选的用户文字和 1–4 张参考图，提取最终英文 `query`、palette 和 atmosphere tags，再复用现有 SearchCatalog 查询 Unsplash/Pexels。

阶段三只解决“参考图 + 文字 → 视觉解析 → 搜索 refinement”闭环，不改变 Library 的持久化语义，不接 Canvas、Moodboard 或 Billing。

## 已确认决策

- Vision Search 入口常驻在现有 Admin `Search Providers` 卡片内，不新增独立 Admin 栏目。
- Admin v1 只配置一个 active Vision provider；配置字段为 Endpoint URL、Model、API Key/Token、enabled 和 test state。
- 后端复用现有 Provider Endpoint/secret/test 基础设施，但使用独立 `VisionSearch` capability/protocol；Search 页面不感知 Cloudflare、Modal 或具体模型。
- IdeaBoard 与 Vision provider 的统一请求格式为 `multipart/form-data`：`images[]` 为二进制，`user_text` 为可选文本。
- Vision provider 不接受 URL-only 输入；Adapter 负责将统一 multipart 请求转换为具体 provider 协议。
- 参考图来源支持本地上传和 Library 选择；不支持用户手填任意 URL。
- Vision 输入全程不持久化：不写 `media_assets`、R2 或本地 `/uploads`。请求结束后释放内存缓冲区。
- Library 图片仅由服务端按 Unsplash/Pexels allowlist 临时读取；不自动复制成 Media asset。
- 本地图片和 Library 图片统一通过服务端校验；任一图片校验或读取失败时阻止整组提交，用户删除/替换后重试。
- 最多 4 张图片；复用现有图片 MIME、尺寸和格式校验；整组请求最多 10MB。
- 没有参考图时完全复用现有 Text Search，不调用 Vision provider；有参考图时点击 `Search` 才进入 Vision 流程。
- `user_text + images` 在一次 extraction 请求中融合；用户文字是明确意图，图片提供视觉上下文。
- Vision 输出统一为简洁英文 canonical search terms；用户输入可以是中文或英文。
- 最终 `query` 是普通可编辑主搜索词；Fusion Panel 不单独展示 AI query。
- Fusion Panel 只展示最多 5 个 palette dots 和最多 3 个 atmosphere tags；缺失结果不补假数据。
- palette/tag 是可 toggle 的结构化 refinement；点击后只重新执行 SearchCatalog，不重复 Vision extraction。
- 新增、删除或替换参考图会清空旧 extraction 并标记 stale；下一次 `Search` 才重新提取。
- 用户修改最终 query 只重新执行 SearchCatalog，不重复 Vision extraction。
- Vision extraction 成功后自动执行一次 SearchCatalog；Catalog 失败时保留 extraction 结果和旧画廊，Retry 只重试 Catalog。
- Extraction 失败不执行 Catalog Search；保留参考图、文字和旧结果，提供 Retry。
- Vision server timeout 为 30 秒；不自动重试，只有用户主动 Retry 才重新请求。
- 不做用户级 Vision rate limit，不扣 Ideas，不创建 Task；只记录一次 `vision_search_requests_total` metrics counter。
- 阶段三按 Desktop Web 交付；保留窄屏不溢出的基础可用性，不做 mobile-specific Vision UI。

## 输入与生命周期

### 本地上传

1. 用户选择 1–4 张图片。
2. 浏览器保留 `File` 和本地 object URL，仅用于缩略图。
3. 客户端做数量、MIME、文件大小和基础尺寸校验。
4. 用户点击 `Search` 后，才将图片发送到 API。
5. API 在受限内存中读取并校验图片，完成 Vision 请求后释放缓冲区。

### Library 选择

1. 用户在 Floating Island 的素材卡片点击 `Use as Search Reference`。
2. 素材加入顶部参考图栈；浮岛不自动关闭或展开。
3. 通过 `(provider, source_id)` 去重；重复点击不重复添加。
4. 提交时 API 仅允许读取已验证的 Unsplash/Pexels host。
5. 任一引用读取失败时整组提交失败；不静默丢弃单张图片。

### 参考图 UI

- Search 输入栏继续保留可编辑文字输入。
- 参考图以缩略图栈显示；单张可删除，支持继续添加直到 4 张。
- 清空全部参考图后回到 Text Search 状态，并清除 palette/tag refinement。
- Library 浮岛的打开/关闭完全由用户控制。
- Search 卡片主体点击打开 Lightbox；右上角参考图按钮只加入参考图栈，不自动提交或关闭浮岛。
- 已加入的参考图按钮显示 `check` 状态；不增加额外批量选择模式。

## Vision Contract

### Request

逻辑 endpoint：`POST /api/user/search/vision-extract`

`Content-Type: multipart/form-data`

- `images[]`：1–4 个图片 part。
- `library_asset_ids[]`：已归属当前用户的 Library asset ID；与 `images[]` 合计 1–4 个。
- `user_text`：可选，沿用 Search query 的 200 rune 上限。

服务端必须在请求体和图片 part 层面限制总大小，不接受任意外部 URL，不将图片写入持久化 Storage。

### Response

```json
{
  "query": "warm minimalist cafe interior with natural wood and soft studio light",
  "palette": ["#d97706", "#78350f", "#fef3c7"],
  "atmosphere_tags": ["Walnut Wood", "Studio Lighting"]
}
```

Adapter 必须负责：

- trim 和规范化 `query`，复用 200 rune 上限；
- 验证 `query` 非空且为可搜索文本；
- 验证 palette 为合法颜色值、去重并截取最多 5 个；
- trim、去重并截取最多 3 个短英文 atmosphere tags；
- 丢弃 provider 原始文本、confidence、token usage、debug 字段；
- 非法或空 query 按 schema error 处理，不向前端透传半合法结果。

### Provider test

Admin `测试` 使用内置极小 fixture 图片和固定短文本，完整执行一次 extraction 和 response schema validation；不使用管理员/用户图片，不写 Storage，不记录原始 payload。错误分类至少包括 endpoint、credentials、timeout 和 response schema。

## Search Integration

SearchCatalog 接收结构化 refinement，而不是由页面拼接 provider-specific query：

```ts
type SearchRefinements = {
  palette: string[]
  atmosphereTags: string[]
}
```

- Query 与 refinements 分开传递。
- Unsplash/Pexels Adapter 各自映射支持的 refinement。
- Provider 不支持的 refinement 只被该 Adapter 忽略，不使整次搜索失败。
- Search 页面不分支 provider 名称。
- 最终 query、选中的 palette 和 tags 写入当前 Search URL/query state；参考图不写入 URL。
- 刷新或前进/后退只恢复 query/refinement，并回到 Text Search；不伪装恢复参考图。

## State Model

### Search mode

- `text-idle` / `text-loading` / `text-ready`：现有文本搜索状态。
- `vision-input`：存在参考图但尚未提交 extraction。
- `vision-extracting`：显示固定高度 Fusion skeleton，保留旧画廊。
- `vision-ready`：显示 query、palette、tags，并执行/完成 Catalog Search。
- `vision-extract-error`：保留输入和旧结果，提供 Retry。
- `catalog-loading` / `catalog-ready` / `catalog-error`：沿用现有 SearchCatalog 状态。

### Fusion Panel

- 没有参考图或尚未提交：隐藏。
- extraction 中：显示固定高度 skeleton，避免布局跳动。
- 成功：显示最多 5 个颜色点和最多 3 个 tags。
- extraction 失败：显示简短错误和 Retry。
- 参考图发生变化：清空旧结果并回到未解析状态。

## API Errors

至少包括：

- `vision_search_not_configured`
- `vision_search_unavailable`
- `vision_search_input_invalid`
- `vision_search_image_invalid`
- `vision_search_image_unavailable`
- `vision_search_timeout`
- `vision_search_provider_error`
- `vision_search_schema_error`

前端只显示简短可行动文案，不展示 endpoint URL、provider payload、内部堆栈或图片 URL。

## Security and Operations

- 仅认证用户可调用 Vision endpoint。
- Provider secret 只在服务端读取；浏览器不接触 API Key/Token。
- Library 临时读取只允许已知 Unsplash/Pexels host。
- 日志不记录图片 bytes、base64、原始 URL、用户原文或 provider payload。
- 只记录安全的 provider、endpoint id、图片数量、请求大小、耗时、outcome 和 correlation id；阶段三只要求 `vision_search_requests_total` counter，详细 observability 延后。
- 不新增 rate limit；未来 Billing 需要独立、可审计的 usage fact，不复用 metrics counter。

## Testing

### Backend

- 1–4 张图片、整组 10MB、MIME/尺寸边界。
- multipart 解析和内存生命周期，不写 Media/Storage。
- Library allowlist 临时读取与任一图片失败时 all-or-nothing。
- Vision provider request mapping、timeout、cancel、fixture test 和 response schema validation。
- query/palette/tag normalization、去重、截断和非法响应。
- endpoint 未配置、认证失败、超时、provider error、schema error。
- SearchCatalog structured refinement 到 Unsplash/Pexels Adapter 的 capability mapping。
- metrics counter 仅在有效 Vision 提交时递增。

### Frontend

- Text/Vision 路由分支和输入状态。
- 参考图添加、删除、去重、4 张上限和 Library 不自动关闭。
- extraction loading/success/error/retry，保留旧结果。
- query 编辑不重复 extraction；图片变化标记 stale。
- palette/tag toggle、debounced Catalog Search 和 URL/query state 恢复。
- Lightbox、Keyboard、无障碍名称和窄屏不溢出。

不新增纯文案测试、截图测试或浏览器 E2E；浏览器体验由人工验证。

## Explicitly Deferred

- Vision 输入持久化、Media asset copy、R2/local `/uploads` 临时文件。
- 用户级 rate limit、Ideas 扣费、Task/River 执行和 Billing usage ledger。
- 多 Vision provider pool、权重、failover 和 provider-specific UI。
- Canvas import、Moodboard、Design Brief、AI Style Generator。
- Media Transformation、自动压缩、裁剪、转码和 CDN proxy。
- Mobile-specific picker/bottom-sheet 交互。
- 原始 provider payload、confidence、token usage 和详细 observability dashboard。

## Delivery Sequence

1. Vision provider capability、multipart contract、配置/测试与 backend adapter。
2. SearchCatalog structured refinement 与 provider capability mapping。
3. Search reference input、Library card reference action、Fusion Panel 与 state machine。
4. API/generated contracts、targeted tests、frontend tests、build/lint 与人工 browser smoke。
