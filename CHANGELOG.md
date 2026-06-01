# 更新日志

本文档记录 IdeaBoard 的正式发布变更。

## Unreleased

### 画布与任务运行时

- 任务结果等待链路改为 SSE 状态推送，前端通过任务事件流接收终态结果。
- 画布工作台移除独立 Model 节点，模型与功能语义收敛到 workflow feature 和节点工具参数。
- 画布 chrome 控件移除悬停提示，仅保留可访问名称；节点内部工具按钮保留统一延迟提示。

### 安全与部署

- Better Auth 生产环境强制配置 `BETTER_AUTH_SECRET` 与 `BETTER_AUTH_URL`。
- 生产环境拒绝 Auth internal key 与 Better Auth secret 使用占位值。
- Auth 登录/注册限流改为依赖可信代理解析后的客户端 IP，避免直接信任请求头。
- Express 5 auth 路由迁移到命名 wildcard，避免 `/api/auth/*` 在新版 path parser 下启动失败。
- Go API CORS 改为 allowlist；生产配置 `APP_CORS_ALLOWED_ORIGINS` 时同样生效。
- 任务提交与画布保存增加大小、数量和嵌套层级限制。
- 任务参数字段调整为 `MEDIUMTEXT`，与 2MB 参数校验上限保持一致。
- 发布流程增加 Web/Auth 生产依赖审计与 Go API 测试。
- Nginx 上传上限调整为 `50m`，与后台上传接口保持一致。

### 依赖与构建

- 前端升级到 ECharts 6、vue-echarts 8、Vue Router 5 与 TypeScript 6，图表注册仅保留当前使用的 `line/bar/scatter`。
- Auth 服务升级到 Express 5、dotenv 17 与 TypeScript 6。
- Auth Docker 镜像固定升级到 npm 11.16.0，避免 `ideaboard update` 执行 migration 时由容器内旧 npm 触发 major update notice。
- 退出登录导航从 user store 移到共享 composable，消除 router 与 store 的循环依赖。

## v0.2.0 - 2026-05-05

管理后台统计系统、渠道商重构与后端性能优化。

### 统计仪表盘

- 新增运营统计页面（`/admin/statistics`），支持 7 天 / 30 天 / 1 年维度切换。
- 每日统计聚合：按 (日期, 模型, 用户) 维度汇总调用量、成功/失败数、延迟。
- 事件源去重：`TaskStatEvent` 按 `task_id` 幂等，`TaskDailyStat` 按唯一键 upsert。
- 历史回填：部署前任务通过 `TaskDailyStatBackfill` 标记按天扫描补录。
- 统计 API 支持回填状态返回 + 独立回填触发端点。
- 健康热力图按天分桶，模型消耗占比环形图，用户排行。

### 渠道商与账号重构

- 渠道商页面从表格重构为卡片式布局，账号管理内嵌到渠道商卡片。
- 提取 `useChannelAccounts` composable，账号 CRUD、表单、模型检测逻辑独立。
- 移除渠道级 `models` 字段，模型完全归属 API Key 账号。
- 模型检测改为后端代理（`POST /admin/channels/detect-models`），含 SSRF 防护。
- 账号模型必选校验恢复（`min: 1`）。

### 后端优化

- Provider-agnostic 模型解析：多模型账号自动选择匹配模型。
- `FailRunningByAccountID` 整体事务化，upsert 失败回滚。
- `UpsertDailyStatByTask` 懒 fallback：result 含 model 时跳过 DB 查询。
- `GetStatistics` 3 次 daily stats 查询合并为 1 次。
- 关键词长度截断（200 字符）防止查询退化。

### 前端优化

- Admin CSS 选择器从 `:deep()` 迁移到 `.admin-route` body 级 class。
- 统一 `--admin-danger` token，移除重复 `--admin-error`。
- 调用量趋势图表溢出裁剪（`area-chart-clipped`）。
- 年线 x-axis 仅显示月首标签（~12 个）。
- Tooltip 边缘自适应（`is-left-edge` / `is-right-edge`）。

### 移除

- `AccountsScreen.vue`、`MonitorScreen.vue`、`monitorMeta.ts` 及对应页面 wrapper。
- 渠道级 `models` 字段（Channel 模型、请求类型、前端表单、统计 fallback）。


## v0.1.0 - 2026-04-26

首个发布版基线，提供完整的 AI 画布工作流、后台运营配置、身份认证与标准 Linux 部署能力。

### 核心能力

- 提供用户画布工作台、画板管理、用户中心与管理后台。
- 支持草图成图、风格延展、局部改款、局部重绘、双图融合、单视角展示、商品展示、图案生成、HD 与图片延伸。
- 支持任务落库、Redis 队列、Worker 异步执行、账号池调度、状态轮询与结果持久化。
- 支持 `Local / S3-compatible` 存储配置。

### 认证与后台

- 登录、注册、登出与 session 校验由 Better Auth Service 接管。
- Go API 负责业务授权、用户档案、角色/status 判定与高风险身份操作编排。
- 管理后台支持用户封禁/解封、会话吊销、伪装登录/停止伪装、角色升降级、受控彻底注销、开放注册开关与防刷限流设置。
- 后台系统提示词按 workflow feature 配置，并使用当前 canonical contract。

### 部署与文档

- 标准部署形态为 `Nginx + systemd + Go API + Better Auth Node Service + MySQL + Redis`。
- 发布包由 `scripts/package-deploy.sh` 生成，包含前端静态产物、Go API 二进制、Auth Service 构建产物、systemd 与 Nginx 模板。
- 文档覆盖项目入口、开发 contract、系统提示词配置、Auth 运维边界、本地依赖与 Linux 部署流程。
