# 业界成熟 SaaS API 规范与架构对标调研报告

> **调研对象**：Stripe, Figma, Replicate, OpenAI, Auth0/Better Auth, Slack, GitHub
> **对比主体**：IdeaBoard API 规范 (`docs/api-reference.md`)
> **归档位置**：`docs/architecture/api-standards-research.md`
> **状态**：已完成 (Completed)

---

## 摘要 (Executive Summary)

本调研基于硅谷一线顶尖 SaaS（Stripe、Figma、Replicate、OpenAI、Auth0 等）的官方开发者文档和 API 规范，对 IdeaBoard 目前的接口设计进行全维度对标。

**核心结论**：
1. **认证路径独立于 `/v1` 是业界通行的标准（Best Practice）**：包括 Auth0、Okta、Stripe、NextAuth、Supabase 在内的主流产品均将 Auth 视为全局横切基础设施，不参与 `/v1` 资源版本化。
2. **多租户层级路由与 Figma/Slack 100% 对齐**：租户作用域（`/workspaces/{id}/...`）结合资源唯一标识符是防范越权（IDOR）和实现清晰租户隔离的标准做法。
3. **幂等性与异步任务流是生产级 AI SaaS 的行业范式**：直接对标 Stripe 的 `Idempotency-Key` 语义和 Replicate 的异步预测生命周期（`create -> poll/sse -> outputs`）。
4. **当前架构定位精准**：整体设计处于成熟现代 SaaS 的主流水准，既没有玩具项目的业余漏洞（如无幂等、同步阻塞 AI 生成、裸改余额），也没有超前过度引入开放平台专用的复杂 Webhook/开发者密钥体系。

---

## 一、调研维度 1：Auth 接口与版本号划分 (`/api/auth` vs `/api/v1/auth`)

### 1.1 业界主流实践调研 (Primary Sources)

| 平台 / 规范 | 认证路由基准 | 业务 API 基准 | 设计原因与官方立场 |
| :--- | :--- | :--- | :--- |
| **Auth0 / Okta** | `/oauth/token`<br>`/authorize` | `/api/v2/*` | 认证协议（OAuth2/OIDC）规范极度稳定，属于平台安全网关，不随业务资源（`/api/v2`）版本更新而重命名。 |
| **Stripe** | `https://connect.stripe.com/oauth/token` | `https://api.stripe.com/v1/*` | 认证授权入口完全独立于业务结算 API 域，保证合作伙伴配置的 OAuth Redirect URI 永久有效。 |
| **Better Auth / Supabase / NextAuth** | `/api/auth/*` | `/rest/v1/*` | 框架官方 SDK 默认约定以 `/api/auth` 为根节点。解耦业务资源迭代与身份会话管理。 |
| **Google Identity** | `/o/oauth2/v2/auth` | `/calendar/v3`, `/drive/v3` | 仅在 OAuth 协议本身升级（如 v1 到 v2）时变更协议版本，不参与具体产品（Calendar, Drive）的 API 版本。 |

### 1.2 调研结论
* **业界共识**：**`api/auth` (不带业务版本号) 是广泛认可的最佳实践**。
* **反模式警告**：将认证挂在 `/api/v1/auth` 通常属于单体框架默认路由懒惰配置（Framework Monolithic Default）。一旦未来升级 `/api/v2`，会导致客户端被迫重写登录入口或出现“到底用哪个版本认证”的概念混乱。

---

## 二、调研维度 2：多租户与资源层级模型 (REST Hierarchy)

### 2.1 Figma 与 Slack 官方架构对比

* **Figma REST API 层次结构**：
  * 用户上下文：`GET /v1/me`
  * 团队与项目：`GET /v1/teams/:team_id/projects` ➔ `GET /v2/folders/:folder_id/files`
  * 文档与叶子节点：`GET /v1/files/:file_key`（全量 JSON 节点树）➔ `GET /v1/files/:file_key/versions`
* **IdeaBoard 当前实现**：
  * 全局上下文：`GET /api/v1/user`
  * 工作区与成员：`GET /api/v1/workspaces/{workspace_id}`
  * 画板与画布：`GET /api/v1/workspaces/{workspace_id}/boards/{id}/document` ➔ `GET /.../revisions`

### 2.2 调研结论
* **强租户隔离（Tenant-First Hierarchy）**：IdeaBoard 在路由显式携带 `{workspace_id}`，让权限校验守卫在最外层（Gin 中间件）即可完成租户准入判定，完全符合 Figma 与 Slack 的多租户防护模型。

---

## 三、调研维度 3：长耗时 AI 生成的异步任务管线 (Async Inference Pipeline)

### 3.1 Replicate 与 OpenAI 异步预测模型对比

在生图、超分、视频生成等耗时 5s~60s 的场景下，同步阻塞 HTTP 是业余反模式。

* **Replicate Predictions API**：
  1. `POST /v1/predictions` ➔ 立即返回 `201 Created`（包含 `id` 和初始状态 `starting`）。
  2. 轮询状态：`GET /v1/predictions/{id}`（状态流转：`starting ➔ processing ➔ succeeded / failed`）。
  3. 获取产物：`output` 字段提供产出文件的永久/临时 CDN 存储链接。
* **IdeaBoard 当前实现**：
  1. 报价前置：`POST /api/v1/workspaces/{ws}/task-quotes`（防余额不足或价格变动争议）。
  2. 事务提交：`POST /api/v1/workspaces/{ws}/tasks`（秒级返回 201 + 数据库预占积分 + River 任务入队）。
  3. 执行与监控：客户端轮询或通过 `/platform/monitor/stream` (SSE) 监听进度。
  4. 产出消费：`GET /.../tasks/{task_id}/outputs` 获取高清衍生图与成果物。

### 3.2 调研结论
* **设计成熟度极高**：IdeaBoard 不仅实现了 Replicate 的标准异步流水线，还增加了前置询价（Quote）与事务性积分预占（Hold），完全吻合商业化 AI 平台的生产标准。

---

## 四、调研维度 4：写操作幂等性与资金安全 (Financial Idempotency)

### 4.1 Stripe 幂等性金标准对标

* **Stripe 规范**：
  * 所有涉及付款、充值、转账的请求强制传递 `Idempotency-Key`。
  * 相同 Key + 相同参数：直接返回上一次的执行结果缓存（安全重试）。
  * 相同 Key + 不同参数：触发 `409 Conflict`。
* **IdeaBoard 当前实现**：
  * 在 Stripe 充值、积分扣减、任务创建中全面推行 `Idempotency-Key`。
  * 幂等性以 PostgreSQL 中的持久记录、唯一约束和事务为事实源；请求参数冲突时返回 `409 Conflict (billing_checkout_conflict / task_idempotency_conflict)`。Task insert、Credit Hold 与 River enqueue 在同一事务中提交，不依赖 Redis 或内存锁维持跨实例正确性。

### 4.2 调研结论
* 达到 Fintech 级别的账务防重标准，能够从容抵御断网重试、前端连击和网络重放攻击。

---

## 五、差距分析与阶段演进建议 (Gap Analysis)

对比 Stripe / GitHub 的**全球开发者开放平台（Public Developer Platform）**，IdeaBoard 当前在定位上有所聚焦：

| 领域 | 顶级开放平台 (Phase 2) | IdeaBoard 当前 (SaaS 预发阶段) | 评估与建议 |
| :--- | :--- | :--- | :--- |
| **凭据类型** | 支持 Personal Access Token / API Key (`ib_live_...`) | 纯基于浏览器会话 Cookie (`better-auth`) | **符合现状**：现阶段服务于自家 Web 前端（BFF 模式），尚未开放第三方开发者生态，暂无引入 API Key 的必要。 |
| **限流响应头** | 遵循 RFC 6585 标准（下发 `RateLimit-*` 头部） | 服务端拦截并返回 `429 Too Many Requests` | **低成本可优化**：建议在未来的中间件中补齐 `X-RateLimit-Limit` 和 `X-RateLimit-Remaining` 响应头。 |
| **外部回调通知** | 提供客户自定义 Webhook 接收面板 | 站内 WebSocket / SSE 实时通道 | **符合现状**：目前用户全部在前端画布实时协同，无需向第三方企业服务器推送 Webhook。 |

---

## 六、综合评估结论

IdeaBoard 的接口规范**不仅是成熟 SaaS 的标准形态，而且遵循了当前现代云原生产品的最佳架构模式**：
* 认证基础设施全局解耦（`/api/auth`）；
* 业务核心严守多租户租界（`/api/v1`）；
* 敏感运维收归独立控制面（`/api/control/v1`）；
* 重耗时 AI 流程异步化、全链路幂等化、账务不可变化。

此架构足以支撑从 Pre-Launch 到规模化商业运营的平滑演进。
