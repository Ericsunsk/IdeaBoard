# Shared

Shared 是前端唯一的横向依赖层：HTTP 出口、传输契约、纯展示组件、跨 feature 原语与主题。它不得依赖任何具体 feature 模块（运行时；contract spec 读源码做 token 校验的 3 处除外）。

## Language

**API Boundary**:
`Vue UI -> application use case -> resource/adapter -> generated API`。`orvalMutator` 是所有 generated 文件的唯一 fetcher；feature 必须经 `shared/api/*` wrapper 或自家 `*Resource.ts` 调用；generated 代码禁手改。
_Avoid_: page-direct generated call, hand-edited wrapper, second fetcher

**Error Catalog**:
有稳定 `code` 的错误必须走 `shared/errors/errorCatalog.ts`（`userFacingErrorCopy`），禁止 prose 匹配与散写文案；只有 `message_safe` 的后端 `msg` 才可透出。
_Avoid_: string-matched errors, leaked unsafe messages

**Query Client**:
唯一的 TanStack Query 单例（`stale 5s / gc 10m / retry 1 / no refocus`）。身份变更走 `clearAuthScopedQueryCache` 全清；feature 禁止私建 `QueryClient`。
_Avoid_: per-feature client, cached session identity

**MediaRef**:
Canonical media 事实（`assetId + tier + format`）。URL 只是运行时分发细节（`buildMediaBrowserURL`），永不持久化；Canvas facts 禁止存 URL。
_Avoid_: persisted provider URL, tier-less asset ref

**Control State**:
交互控件四态（`idle/selected/disabled/loading` + `aria-busy/aria-pressed`），由 `shared/interaction` 统一解析，样式只消费不定义。
_Avoid_: per-component disabled logic, missing busy semantics

**Theme Contract**:
PrimeVue PT 与 CSS token 的单源（`shared/theme`）。overlay/dialog/round-control 等样式必须消费 token；contract spec 反向校验 canvas/search/board-catalog/library/user-shell 的消费。
_Avoid_: hand-authored overlay CSS, hardcoded motion durations

**Locale Parity**:
`en` 常驻、`zh` 懒载；`en/zh-CN/es/ko` key 必须一致（`localeParity` 契约）；locale 切换原子更新 i18n + PrimeVue + `<html lang>` + 存储，失败不写存储。
_Avoid_: partial translation, half-switched locale

## Module Ownership

| Module               | Responsibility                                                                                                                                 | Must Never Depend On                                                    |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `shared/api`         | 唯一 HTTP 出口（ofetch 同源 `/api`、10s 超时、auth 失败收敛）、`resolveApiData/Ok`、task/upload/media 分发、SSE 用户事件流、`stripUndefined`。 | 具体 `features/*`。                                                     |
| `shared/auth`        | Better Auth 单例、重定向安全策略（`normalizeSafeInternalRedirectPath`，防 `//`/`\`/`%5c`/`..`）、密码与 OAuth 提供商策略。                     | feature 路由组件；`localStorage USER_KEY` 只经 `shared/utils/auth.ts`。 |
| `shared/components`  | 纯展示组件（账户菜单、余额按钮、确认框、语言选择、表格框、图表卡）。余额数字由调用方注入，组件不自取数。                                       | 任何 feature store/resource。                                           |
| `shared/composables` | `useAppToast`、`useAsyncAction`（pending 计数 + 生命周期钩子）、确认框、overlay 生命周期、品牌、登出重定向。                                   | 具体 feature composable。                                               |
| `shared/config`      | `API_BASE_URL`（生产强制 `/api`）、`AUTH_BASE_URL` 派生、asset/上传引用归一。                                                                  | 任何业务常量。                                                          |
| `shared/constants`   | 路由路径单源、法务链接、媒体常量。                                                                                                             | i18n 文案。                                                             |
| `shared/domain`      | 工作流词典（8 slugs）、generation primitives（outputCount 1–4 严格拒绝非 clamp）、outputResolution。                                           | 后端 SKU/价格（定价显示归 account/admin）。                             |
| `shared/errors`      | 错误码收敛与用户面文案。                                                                                                                       | 散写 `t('...')` 错误文案。                                              |
| `shared/i18n`        | i18n 单例、locale 原子切换、`"N Idea(s)"` 货币格式。                                                                                           | feature 私有 key 冲突。                                                 |
| `shared/interaction` | 控件四态解析。                                                                                                                                 | 具体组件样式。                                                          |
| `shared/lib`         | 无依赖纯函数（品牌、页面铬、图表基元）。                                                                                                       | DOM/Query。                                                             |
| `shared/media`       | MediaRef、上传管线（魔数嗅探、HEIC→JPEG、压缩阈值、失败回退原图）。                                                                            | Canvas facts 存 URL；压缩参数散改。                                     |
| `shared/motion`      | 动效时长/缓动 token 与 dialog/popover/chart 预设（`motionPolicyGuard.spec` 约束硬编码）。                                                      | 业务逻辑。                                                              |
| `shared/query`       | queryClient 单例、task 结果归一（只认 `role=image + asset_id`）、用户 credits 失效 pub/sub。                                                   | feature 私建 client；credits 失效必须走此通道。                         |
| `shared/theme`       | PT 单源 + CSS token。                                                                                                                          | feature 自造 overlay/dialog 样式。                                      |
| `shared/utils`       | 客户端身份单源（`get/set/clearStoredAuth` 含清 Query）、错误文案清洗、日期/表单/上传小工具。                                                   | feature 直读写 `localStorage['user']`。                                 |
| `shared/seo`         | 路由 SEO meta 与静态 HTML。                                                                                                                    | 业务数据。                                                              |
| `shared/assets`      | provider/storage/oauth 纯静态资源。                                                                                                            | 代码 import 以外的引用。                                                |

## Rules

- 新增 shared 模块前先证明三个以上调用方；禁止为凑复用而建通用层。
- 全局状态只允许：queryClient 单例、身份存储、media 分发缓存、SSE 单例、i18n bundle 缓存、composable 局部机。无 Pinia。
- transport DTO 到 feature model 的翻译发生在 adapter/resource 层；domain 代码不得 import Vue、router、Query 或 generated DTO。
- 认证/重试/关联/错误分类集中在 `shared/api` 与 `shared/auth`，feature 不得各搞一套。
