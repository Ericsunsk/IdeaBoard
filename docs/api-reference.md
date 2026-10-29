# IdeaBoard 系统完整接口规范文档 (API Reference)

> **版本**：v1.0 (Pre-Launch Baseline)
> **服务基础架构**：
> - **认证服务 (Auth Service)**：Fastify + Better Auth，路由基准路径：`/api/auth`
> - **业务服务 (Public V1 API)**：Go (Gin)，路由基准路径：`/api/v1`
> - **控制台服务 (Control Plane Admin API)**：Go (Gin)，路由基准路径：`/api/control/v1`
> - **实时同步 (Realtime WS)**：NATS JetStream + WebSocket，基于临时 Ticket 鉴权

---

## 目录
1. [全局设计规范与约定](#一全局设计规范与约定)
2. [认证服务接口 (Authentication API)](#二认证服务接口-auth)
3. [业务服务核心接口 (Public V1 API)](#三业务服务核心接口-public-v1)
   - [3.1 用户、通知与全局配置](#31-用户通知与全局配置)
   - [3.2 企业组织与全生命周期 (Organizations)](#32-企业组织与全生命周期-organizations)
   - [3.3 工作区与成员协作 (Workspaces & Members)](#33-工作区与成员协作-workspaces--members)
   - [3.4 画板、历史版本与协作通道 (Boards & Canvas)](#34-画板历史版本与协作通道-boards--canvas)
   - [3.5 媒体资产库与上传链路 (Library & Uploads)](#35-媒体资产库与上传链路-library--uploads)
   - [3.6 AI 生成任务与视觉搜索 (Tasks & Vision Search)](#36-ai-生成任务与视觉搜索-tasks--vision-search)
   - [3.7 积分账本、额度配额与充值结算 (Credits & Billing)](#37-积分账本额度配额与充值结算-credits--billing)
4. [管理后台接口 (Control Plane Admin API)](#四管理后台接口-control-plane-admin)
   - [4.1 用户治理与会话控制 (Users & Governance)](#41-用户治理与会话控制-users--governance)
   - [4.2 计费定价与服务模型池 (Catalog & Model Pool)](#42-计费定价与服务模型池-catalog--model-pool)
   - [4.3 积分调账、账本健康与对账 (Credits & Ledger Audit)](#43-积分调账账本健康与对账-credits--ledger-audit)
   - [4.4 支付充值订单管理 (Payment Orders)](#44-支付充值订单管理-payment-orders)
   - [4.5 平台系统配置 (AI Gateway, Storage, Outbound, Mail)](#45-平台系统配置-platform-settings)
   - [4.6 任务监控与兑换码 (Monitor & Rewards)](#46-任务监控与兑换码-monitor--rewards)

---

## 一、全局设计规范与约定

### 1.1 路由前缀与服务边界划分规范
系统按关注点分离（Separation of Concerns）与物理服务部署拓扑划分为三大路由空间：
* **`/api/auth/*`（认证横切基础设施）**：
  由独立的 Node.js (Fastify + Better Auth) 服务承载。作为全站用户身份的统一签发源，它跨越所有的业务 API 版本，终生保持稳定，确保 Google / GitHub / Discord 等第三方 OAuth Callback URL 永远无需随业务版本升级而变动。
* **`/api/v1/*`（业务领域 RESTful 资源）**：
  由 Go 核心服务承载，包含用户、工作区、画板、多端协同、AI 任务、积分钱包等核心业务资源。
* **`/api/control/v1/*`（管理控制台内部运营）**：
  由 Go 核心服务承载，受统一的管理员角色守卫（Admin Guard）保护，专供运营与运维团队进行平台治理。

### 1.2 鉴权与会话机制 (Authentication)
* **Session Cookie**：业务接口（`/api/v1` 与 `/api/control/v1`）通过 Better Auth 下发的高安全性 HTTP-only Cookie (`better-auth.session_token`) 携带身份凭据。
* **Control Plane 权限保护**：所有 `/api/control/v1/*` 接口强制校验请求主体身份为平台管理员（`role: admin`），否则直接返回 `403 Forbidden`。

### 1.3 幂等性要求 (Idempotency)
对于契约要求幂等的支付下单、AI 任务提交等写入接口，按各端点要求在 HTTP Header 中携带：
```http
Idempotency-Key: <unique-uuid-or-token>
```
* 重复的 `Idempotency-Key` 配合相同请求体将直接回放既有结果。
* 重复 Key 配合不同请求体将触发 `409 Conflict (billing_checkout_conflict / task_idempotency_conflict)`。

### 1.4 统一响应与错误格式 (Contract Envelope)

以下为 Go Public V1 与 Control Plane 的 JSON 契约；Auth Service 使用 Better Auth 自身的响应格式。具体字段和状态码以 [Public OpenAPI](../apps/api/openapi.public.yaml) 与 [Control OpenAPI](../apps/api/openapi.control.yaml) 为准，文件和事件流接口不使用 JSON envelope。

#### 成功响应 (Success)
```json
{
  "data": {
    "id": "resource-id"
  }
}
```

分页响应额外包含 `meta`，其中 `has_more` 为必填字段，`next_cursor` 按需返回。

#### 错误响应 (Error)
```json
{
  "error": {
    "code": "validation_failed",
    "message": "请求输入不合法",
    "details": {
      "fields": {
        "name": "不能为空"
      }
    }
  }
}
```

---

## 二、认证服务接口 (Auth)
基准路径：`/api/auth` (Better Auth 驱动)

| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `POST` | `/sign-up/email` | 邮箱密码注册 | `{ email, password, name }` |
| `POST` | `/sign-in/email` | 邮箱密码登录 | `{ email, password }` |
| `POST` | `/sign-in/social` | 第三方社交登录 (Google / Discord / GitHub) | `{ provider, callbackURL }` |
| `POST` | `/sign-out` | 退出登录并销毁会话 | 无 |
| `GET` | `/get-session` | 获取当前会话状态及登录用户信息 | 无 (依赖 Cookie) |
| `POST` | `/forget-password` | 发送重置密码邮件 | `{ email, redirectTo }` |
| `POST` | `/reset-password` | 执行重置密码 | `{ token, newPassword }` |

---

## 三、业务服务核心接口 (Public V1)
基准路径：`/api/v1`

### 3.1 用户、通知与全局配置
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/user` | 获取个人公开主页/基本资料 | 无 |
| `PATCH`| `/user` | 修改个人资料 (姓名等) | `{ name }` |
| `POST` | `/user/avatar` | 上传个人自定义头像 | `multipart/form-data (file)` |
| `GET` | `/public/users/{uid}/avatar` | 获取指定用户头像图 (支持尺寸缩放) | `Query: size=32\|64\|128` |
| `GET` | `/public/branding/logo` | 获取系统 Branding Logo 资源 | 无 |
| `GET` | `/support-channels` | 获取系统对外支持渠道 (客服邮箱/社群等) | 无 |
| `GET` | `/user/notifications` | 分页获取个人站内通知列表 | `Query: cursor, limit` |
| `GET` | `/user/notifications/unread-count` | 获取未读通知总数 | 无 |
| `POST` | `/user/notifications/{id}/read` | 标记单条通知为已读 | 无 |
| `POST` | `/user/notifications/read-all` | 标记所有通知为已读 | 无 |
| `POST` | `/user/notifications/{id}/archive` | 归档单条通知 | 无 |
| `GET` | `/user/invites` | 查询当前用户收到的工作区待处理邀请 | 无 |
| `POST` | `/user/invites/{invite_id}/accept` | 接受工作区邀请 | 无 |
| `POST` | `/user/invites/{invite_id}/decline`| 拒绝工作区邀请 | 无 |
| `GET` | `/user/organization-invites` | 查询当前用户收到的企业组织待处理邀请 | 无 |
| `POST` | `/user/organization-invites/{id}/accept`| 接受企业组织邀请 | 无 |
| `POST` | `/user/organization-invites/{id}/decline`| 拒绝企业组织邀请 | 无 |

---

### 3.2 企业组织与全生命周期 (Organizations)
> 基于 ADR-0038：个人拥有 1 个 Personal 资金池，最多加入/创建 1 个企业组织（共用共享资金池）。

| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/organizations` | 获取当前用户所属的组织列表/状态 | 无 |
| `POST` | `/organizations` | 创建新组织 (创建者自动成为 Owner) | `{ name }` |
| `GET` | `/organizations/{org_id}` | 获取指定组织的详情与成员状态 | 无 |
| `PATCH`| `/organizations/{org_id}` | 修改组织名称 | `{ name }` |
| `POST` | `/organizations/{org_id}/closure` | **永久关闭组织** (仅 Owner，不可恢复) | `{ confirmation: "CONFIRM" }` |
| `GET` | `/organizations/{org_id}/workspaces` | 获取该组织旗下绑定的所有业务工作区 | 无 |
| `POST` | `/organizations/{org_id}/workspaces` | 在该组织下创建新工作区 (使用共享池) | `{ name }` |
| `GET` | `/organizations/{org_id}/members` | 分页获取组织所有成员及角色列表 | `Query: cursor, limit` |
| `PATCH`| `/organizations/{org_id}/members/{id}`| 变更成员角色 (提升/降级 Admin) | `{ role: "admin" \| "member" }` |
| `DELETE`| `/organizations/{org_id}/members/{id}`| 从组织中移出成员 | 无 |
| `GET` | `/organizations/{org_id}/invites` | 获取组织当前发出的待处理邀请 | 无 |
| `POST` | `/organizations/{org_id}/invites` | 发送组织加入邀请 | `{ email, role }` |
| `DELETE`| `/organizations/{org_id}/invites/{id}`| 撤回组织邀请 | 无 |

---

### 3.3 工作区与成员协作 (Workspaces & Members)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/workspaces` | 分页获取当前用户可见的工作区列表 | `Query: cursor, limit` |
| `POST` | `/workspaces` | 创建工作区 (可指定挂载 Personal 或 Org) | `{ name, organization_id? }` |
| `GET` | `/workspaces/{workspace_id}` | 获取单个工作区详情与元信息 | 无 |
| `PATCH`| `/workspaces/{workspace_id}` | 重命名工作区 | `{ name }` |
| `DELETE`| `/workspaces/{workspace_id}` | 删除工作区 | 无 |
| `GET` | `/workspaces/{workspace_id}/members` | 获取工作区协作者成员列表 | 无 |
| `POST` | `/workspaces/{workspace_id}/members/lookup`| 通过 Email 检查成员是否已存在 | `{ email }` |
| `PATCH`| `/workspaces/{workspace_id}/members/{id}`| 变更工作区成员协作者角色 | `{ role: "admin" \| "member" }` |
| `DELETE`| `/workspaces/{workspace_id}/members/{id}`| 移出工作区协作者 | 无 |
| `GET` | `/workspaces/{workspace_id}/invites` | 查看工作区发出的邀请 | 无 |
| `POST` | `/workspaces/{workspace_id}/invites` | 邀请协作者加入当前工作区 | `{ email, role }` |
| `DELETE`| `/workspaces/{workspace_id}/invites/{id}`| 撤销工作区成员邀请 | 无 |

---

### 3.4 画板、历史版本与协作通道 (Boards & Canvas)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/workspaces/{workspace_id}/boards` | 分页获取工作区内的画板列表 | `Query: cursor, limit, keyword` |
| `POST` | `/workspaces/{workspace_id}/boards` | 创建新画板 (包含初始视口与默认配置) | `{ title, description? }` |
| `GET` | `/workspaces/{workspace_id}/boards/{id}`| 获取画板基础信息 | 无 |
| `PATCH`| `/workspaces/{workspace_id}/boards/{id}`| 修改画板标题/封面 | `{ title, thumbnail_url? }` |
| `DELETE`| `/workspaces/{workspace_id}/boards/{id}`| 彻底删除画板 | 无 |
| `POST` | `/workspaces/{workspace_id}/boards/{id}/archive` | 归档画板 | 无 |
| `POST` | `/workspaces/{workspace_id}/boards/{id}/restore` | 恢复归档的画板 | 无 |
| `GET` | `/workspaces/{workspace_id}/boards/{id}/document` | **获取画板完整 JSON 画布文档** | 无 |
| `PUT` | `/workspaces/{workspace_id}/boards/{id}/document` | **全量保存/更新画布文档** | `{ document: { ... } }` |
| `GET` | `/workspaces/{workspace_id}/boards/{id}/revisions` | 分页获取画板历史快照/版本链 | `Query: cursor, limit` |
| `GET` | `/workspaces/{workspace_id}/boards/{id}/revisions/{rev}`| 查看某一特定历史快照文档 | 无 |
| `POST` | `/workspaces/{workspace_id}/boards/{id}/access` | 记录最近访问画板时间 | 无 |
| `POST` | `/workspaces/{workspace_id}/boards/{id}/realtime-ticket`| **获取多端协同 WebSocket Ticket** | 无 |

---

### 3.5 媒体资产库与上传链路 (Library & Uploads)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `POST` | `/workspaces/{workspace_id}/uploads` | 初始化分片/分步上传会话 | `{ filename, mime_type, byte_size }` |
| `PUT` | `/workspaces/{workspace_id}/uploads/{id}/bytes` | 上传实际文件二进制数据 | `Raw Binary / Stream` |
| `POST` | `/workspaces/{workspace_id}/uploads/{id}/complete` | 上传完成确认 (触发元数据解析) | 无 |
| `GET` | `/workspaces/{workspace_id}/media-assets` | 分页查询工作区已处理媒体资产 | `Query: cursor, limit, type` |
| `GET` | `/workspaces/{workspace_id}/media-assets/{id}` | 获取单个媒体资产信息与衍生图 URL | 无 |
| `POST` | `/workspaces/{workspace_id}/media-assets/{id}/archive` | 归档/软删除媒体资产 | 无 |
| `GET` | `/workspaces/{workspace_id}/library/folders` | 获取资产库文件夹树 | 无 |
| `POST` | `/workspaces/{workspace_id}/library/folders` | 创建资产库文件夹 | `{ name, parent_id? }` |
| `PATCH`| `/workspaces/{workspace_id}/library/folders/{id}` | 重命名或移动文件夹 | `{ name?, parent_id? }` |
| `DELETE`| `/workspaces/{workspace_id}/library/folders/{id}` | 删除文件夹 | 无 |
| `GET` | `/workspaces/{workspace_id}/library/assets` | 查询资产库中引用的资源列表 | `Query: folder_id, cursor` |
| `POST` | `/workspaces/{workspace_id}/library/assets` | 将媒体资产保存入个人/团队资产库 | `{ asset_id, folder_id? }` |
| `DELETE`| `/workspaces/{workspace_id}/library/assets/{id}` | 从资产库中移除引用 | 无 |

---

### 3.6 AI 生成任务与视觉搜索 (Tasks & Vision Search)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `POST` | `/workspaces/{workspace_id}/task-quotes` | **查询 AI 任务预估报价 (询价)** | `{ service, model?, resolution?, params }` |
| `POST` | `/workspaces/{workspace_id}/tasks` | **事务性提交 AI 生成/处理任务** | `{ service, model?, prompt, params, board_id }` |
| `GET` | `/workspaces/{workspace_id}/tasks` | 分页查询任务列表与执行状态 | `Query: status, cursor, limit` |
| `GET` | `/workspaces/{workspace_id}/tasks/{task_id}` | 获取任务实时状态 (Progress/Logs) | 无 |
| `POST` | `/workspaces/{workspace_id}/tasks/{task_id}/cancel` | 取消在途排队或正在执行的 AI 任务 | 无 |
| `GET` | `/workspaces/{workspace_id}/tasks/{task_id}/outputs`| 获取任务产出的高清图片/素材结果 | 无 |
| `POST` | `/workspaces/{workspace_id}/search` | 综合素材与图片搜索 | `{ query, limit? }` |
| `POST` | `/workspaces/{workspace_id}/search/vision-queries` | **视觉搜索 (图搜图/相似图检索)** | `{ user_text?, library_asset_ids?, image_urls? }` |

---

### 3.7 积分账本、额度配额与充值结算 (Credits & Billing)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/workspaces/{workspace_id}/credits/balance` | **查询当前工作区归属的资金池可用余额** | 无 |
| `GET` | `/workspaces/{workspace_id}/credits/ledger` | 分页查询该资金账户的积分变动收支流水 | `Query: cursor, limit` |
| `GET` | `/workspaces/{workspace_id}/billing-overview`| 获取资金概览 (余额、扣减、成员配额汇总)| 无 |
| `GET` | `/workspaces/{workspace_id}/usage` | 查看用量统计看板 (按日/按周/按分类) | `Query: range=7d\|30d` |
| `GET` | `/workspaces/{workspace_id}/purchase-options`| 获取当前上架的积分充值规格选项 | 无 |
| `POST` | `/workspaces/{workspace_id}/checkout-sessions`| **发起 Stripe 线上充值收银台会话** | `{ idea_quantity, terms_accepted: true }` |
| `GET` | `/workspaces/{workspace_id}/checkout-sessions/{id}`| 检查充值支付会话状态 | 无 |
| `GET` | `/workspaces/{workspace_id}/pending-orders` | 查询未付款或处理中的待确认充值单 | 无 |
| `POST` | `/workspaces/{workspace_id}/pending-orders/{id}/continue` | 继续未完成的支付单 (重拉 Checkout URL) | 无 |
| `POST` | `/workspaces/{workspace_id}/pending-orders/{id}/abandon` | 主动放弃未付款的订单 | 无 |
| `GET` | `/workspaces/{workspace_id}/recharges` | 分页查询历史充值成功记录与发票单据 | `Query: cursor, limit` |
| `POST` | `/workspaces/{workspace_id}/rewards/redemptions`| **兑换码激活兑换** | `{ code }` |
| `GET` | `/workspaces/{workspace_id}/member-quotas` | 组织管理员查看所有成员的消费限额配置 | 无 |
| `GET` | `/workspaces/{workspace_id}/member-quotas/self` | 成员查看自己当前的消费限额与已用点数 | 无 |
| `PUT` | `/workspaces/{workspace_id}/member-quotas/{uid}`| 为组织成员配置消费限额 (限额周期/点数) | `{ period: "daily"\|"monthly"\|"unlimited", limit }` |

---

## 四、管理后台接口 (Control Plane Admin)
基准路径：`/api/control/v1` (需要系统管理员身份 `admin`)

### 4.1 用户治理与会话控制 (Users & Governance)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/users` | 分页筛选全平台用户 (按邮箱、角色、状态) | `Query: keyword, role, status, cursor` |
| `GET` | `/users/{user_id}` | 获取单个用户的完整系统档案与绑定账号 | 无 |
| `PATCH`| `/users/{user_id}/role` | 调整用户系统角色 (如设为 Admin) | `{ role: "admin" \| "user" }` |
| `POST` | `/users/{user_id}/ban` | **封禁用户** (支持封禁期限与原因) | `{ expires_in_days?, reason }` |
| `POST` | `/users/{user_id}/unban` | 解除用户封禁状态 | 无 |
| `GET` | `/users/{user_id}/sessions` | 查看该用户当前处于活跃状态的所有会话设备 | 无 |
| `DELETE`| `/users/{user_id}/sessions` | 强制注销该用户的所有登录设备 | 无 |
| `POST` | `/users/{user_id}/impersonation` | **管理员模拟登录 (Impersonate)** | 无 |

---

### 4.2 计费定价与服务模型池 (Catalog & Model Pool)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/billing/catalog` | 获取平台所有生图/抠图/超分 SKU 的定价表 | 无 |
| `PATCH`| `/billing/catalog/{sku}/prices` | 调整指定 SKU 各分辨率单价 (点数/次) | `{ prices: { "1k": 10, "2k": 20 } }` |
| `PATCH`| `/billing/catalog/{sku}/availability` | 临时熔断或恢复某个功能的可用性 | `{ enabled: true \| false }` |
| `GET` | `/billing/catalog/{sku}/pool` | **查询该业务功能背后的底层 AI 模型候选池** | 无 |
| `PUT` | `/billing/catalog/{sku}/pool` | **更新该业务功能挂载的 Provider/Model 列表** | `{ version, entries: [{ provider, model_id, enabled, resolutions }] }` |

---

### 4.3 积分调账、账本健康与对账 (Credits & Ledger Audit)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/credits/wallets/{user_id}` | 查看指定用户的钱包资产明细与冻结状态 | 无 |
| `GET` | `/credits/wallets/{user_id}/ledger` | 分页审计指定用户的全量账本原始流水 | `Query: cursor, limit` |
| `POST` | `/credits/grants` | **人工补发/增加积分 (调账)** | `{ billing_account_id, amount, reason }` |
| `POST` | `/credits/deductions` | **人工扣减积分** | `{ billing_account_id, amount, reason }` |
| `POST` | `/credits/corrections` | 账本差错人工冲正操作 | `{ entry_id, amount, reason }` |
| `GET` | `/credits/health` | **实时检查积分总账健康状态 (复式记账平衡性)** | 无 |
| `GET` | `/credits/reconciliation/runs` | 分页查看历史全量对账执行轮次 | `Query: cursor, limit` |
| `POST` | `/credits/reconciliation/runs` | 手动触发一轮全平台资产重算与差错探测 | 无 |
| `GET` | `/credits/reconciliation/findings`| 查看对账发现的账面差异清单 (Finding Queue) | 无 |

---

### 4.4 支付充值订单管理 (Payment Orders)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/payments/orders` | 分页筛选全平台真实资金支付充值单 | `Query: order_number, user, status, cursor` |
| `GET` | `/payments/orders/{order_number}`| 查看充值单详细数据 (Stripe ID、金额等) | 无 |
| `POST` | `/payments/orders/{order_number}/refunds` | **执行原路退款操作** | `{ amount_cents?, reason }` |

---

### 4.5 平台系统配置 (Platform Settings)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET/PUT` | `/platform/settings/registration` | 用户注册开放策略 (开放 / 仅限邀请) | `{ enabled, allow_domains? }` |
| `GET/PUT` | `/platform/settings/rate-limit` | 全局 API 限流频次配置 | `{ requests_per_minute }` |
| `GET/PUT` | `/platform/settings/login-lockout` | 登录密码防暴力破解锁定阈值 | `{ max_attempts, lockout_minutes }` |
| `GET/PUT` | `/platform/settings/branding` | 系统品牌配置 (网站标题、Logo URL) | `{ site_name, logo_url? }` |
| `GET/PUT` | `/platform/settings/support-channels` | 外部技术支持配置 (Discord、帮助文档链接) | `{ channels: [...] }` |
| `GET/PUT` | `/platform/settings/media-upload` | 媒体上传策略 (单文件大小上限、格式白名单) | `{ max_bytes, allowed_types }` |
| `GET/PUT` | `/platform/settings/outbound-policy` | 出站安全代理白名单 (支持整串 URL 粘贴解析) | `{ allow_hosts: [...] }` |
| `GET/PUT` | `/platform/settings/mail` | SMTP 邮件发送服务器配置 | `{ host, port, username, password, from }` |
| `POST` | `/platform/settings/mail/test` | 发送一封测试邮件验证 SMTP 连通性 | `{ to }` |
| `GET/PUT` | `/platform/ai-gateway` | **Cloudflare AI Gateway 连接四元组配置** | `{ base_url, account_id, gateway_id, token? }` |
| `POST` | `/platform/ai-gateway/test-connection` | **AI 网关连通性探活测试 (一键探活)** | `{ base_url, token? }` |
| `GET` | `/platform/ai-gateway/providers` | 获取 AI Gateway 已启用的供应商列表 | 无 |
| `GET/PUT` | `/platform/ai-gateway/catalog` | 全局 AI 模型注册花名册 | `{ items: [{ id, name, provider }] }` |
| `GET` | `/platform/storage/profiles` | 对象存储 Profile 列表 (S3 / R2 / OSS) | 无 |
| `POST` | `/platform/storage/profiles` | 新增对象存储配置 | `{ provider, bucket, endpoint, credentials }` |
| `POST` | `/platform/storage/profiles/{id}/apply`| 激活为当前全站生效的默认存储池 | 无 |
| `POST` | `/platform/storage/profiles/{id}/test` | 测试对象存储桶读写可用性 | 无 |
| `GET/PUT` | `/platform/feature-prompts` | 核心 AI 功能的系统 Prompt 模板配置 | `{ items: [{ slug, system_prompt }] }` |

---

### 4.6 任务监控与兑换码 (Monitor & Rewards)
| 方法 | 端点 | 功能说明 | 核心参数 / 请求体 |
| :--- | :--- | :--- | :--- |
| `GET` | `/platform/monitor` | 分页查看全平台当前执行中或排队中的任务 | `Query: status, service, cursor` |
| `GET` | `/platform/monitor/stream` | **SSE (Server-Sent Events) 实时任务监控大屏流** | 无 |
| `POST` | `/tasks/{task_id}/cancel` | 管理员强制终止异常卡住的后台任务 | `{ reason }` |
| `GET` | `/platform/statistics` | 获取平台运营统计概览 (注册用户、活跃度等) | 无 |
| `GET` | `/rewards/codes` | 分页筛选积分兑换码列表 | `Query: keyword, enabled, cursor` |
| `POST` | `/rewards/codes` | **批量生成/创建积分兑换码** | `{ code, credit_amount, max_redemptions, expires_at }` |
| `PATCH`| `/rewards/codes/{id}/enabled` | 紧急禁用或启用某个兑换码 | `{ enabled: true \| false }` |
| `GET` | `/rewards/codes/{id}/redemptions` | 查看某个兑换码的历史被兑换用户明细 | 无 |

---
*文档更新于：2026-10-29*
