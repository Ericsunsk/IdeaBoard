# <div align="center">IdeaBoard 开发手册</div>

<p align="center">
  <strong>面向仓库维护者与开发者的开发手册。</strong>
</p>

<p align="center">
  聚焦当前系统结构、workflow contract、本地开发路径与关键目录边界。
</p>

<p align="center">
  <code>私有</code>
  <code>闭源</code>
  <code>内部工程文档</code>
</p>

<p align="center">
  <a href="#development-snapshot">系统概览</a> ·
  <a href="#workflow-contract">工作流契约</a> ·
  <a href="#task-lifecycle">任务链路</a> ·
  <a href="#local-dev-fast-path">本地开发</a> ·
  <a href="#repo-map">目录边界</a>
</p>

---

> [!IMPORTANT]
> 本文档仅供 IdeaBoard 授权成员内部使用。项目为闭源私有项目，未经授权不得复制、分发或对外传播。

<a id="development-snapshot"></a>
## 开发概览

IdeaBoard 当前以画布工作流为中心演进，不再以“每个页面 = 一个孤立功能模块”的方式组织产品语义。开发时应优先遵守 workflow feature contract、执行链 contract 和提示词模板 contract。

### 当前速览

| 维度 | 当前配置 |
|------|----------|
| 前端 | `Vue 3` + `Vite` + `TypeScript` + `Naive UI` + `Pinia` |
| 后端 | `Go` + `Gin` + `GORM` |
| 基础设施 | `MySQL 8.0+`、`Redis 7+` |
| 本地端口 | 前端 `5173`，后端 `6000` |
| 当前主链 | 画布节点编排 -> 任务提交 -> Worker 执行 -> 结果回写 |
| 技术标识 | `ideaboard-*` |

### 架构形态

```text
浏览器
  -> Vite / 前端 SPA
  -> Gin API
  -> MySQL (users / tasks / configs / accounts)
  -> Redis (token + task queue)
  -> Worker 消费者
  -> 上游 AI 渠道
  -> Kodo / 本地上传目录
```

## 职责边界

### 前端职责

前端主要负责：

- 画布工作台、节点编排与连接交互
- 任务请求装配
- 结果展示与画板管理
- 管理后台配置页
- 登录态维护与权限路由

主要页面入口：

| 路径 | 说明 |
|------|------|
| `/user/home` | 用户首页与工作台入口 |
| `/user/picture` | 画布工作台主页面 |
| `/user/boards` | 画板列表 |
| `/user/center` | 用户中心 |
| `/admin/*` | 管理后台 |

### 后端职责

后端主要负责：

- 用户鉴权与管理员权限控制
- 任务提交、任务记录与状态轮询
- 账号池、渠道商与任务调度
- 系统提示词模板读取与变量替换
- 分割、超分、向量化、上传与对象存储
- Worker 异步执行与结果持久化

<a id="workflow-contract"></a>
## 工作流契约

当前受支持的 workflow feature slug：

| 功能 slug | 典型语义 |
|-----------|----------|
| `sketch-to-design` | 线稿成款 |
| `style-innovate` | 款式创新 |
| `partial-edit` | 局部改款 |
| `bg-swap` | 换背景 |
| `remove-bg` | 抠图 |
| `face-swap` | 换脸 |
| `style-fusion` | 双图融合 |
| `series-color` | 多色配色 |
| `directed-recolor` | 定向换色 |
| `fabric-on-body` | 面料上身 |
| `multi-angle` | 单图单视角展示 |
| `virtual-3d` | 虚拟 3D 展示 |
| `pattern-gen` | 图案生成 |
| `hd-upscale` | 高清增强 |
| `canvas-extend` | 画布延伸 |

### 契约规则

- `feature` 应作为顶层任务字段传递，并在后端统一 canonicalize。
- `image_urls` 应作为顶层任务字段传递，不再在 `params` 里双轨存放。
- `params` 只承载 feature 专属参数，不再额外复制 `feature` 与 `image_urls`。
- `mask`、`use_mask`、`upscale_mode` 等兼容别名由后端统一读取 canonical key。
- 不保留已退休 feature 的产品文案与模板兼容。

对应实现：

- [executionContract.ts](./apps/web/src/features/workspace/config/executionContract.ts)
- [features.go](./apps/api/internal/workflow/features.go)

### 画布直连工具

除 workflow feature 外，画布中还存在少量直接工具动作：

- `to-vector`

它们不进入 `feature_prompt:<slug>` 模板链，也不属于 workflow feature contract 的 active slug 集合。审计 prompt 系统或 admin 配置页时，不要把这类直连工具误当成 workflow feature 漏斗的一部分。

<a id="task-lifecycle"></a>
## 任务生命周期

### 请求结构

前端通过统一执行 contract 拼装请求，典型字段如下：

```json
{
  "prompt": "用户提示词",
  "feature": "style-fusion",
  "image_urls": ["https://..."],
  "params": {
    "blend_ratio": 0.5
  }
}
```

### 前端入口

- [useNodeExecutors.ts](./apps/web/src/features/workspace/composables/useNodeExecutors.ts)
- [useCanvasDirectActions.ts](./apps/web/src/features/workspace/composables/useCanvasDirectActions.ts)
- [WorkspaceScreen.vue](./apps/web/src/features/workspace/screens/WorkspaceScreen.vue)

### 后端流程

1. 标准化 `feature` 与关键参数。
2. 基于风格卡与系统提示词模板生成最终 prompt。
3. 写入任务表并推入 Redis 队列。
4. Worker 从队列消费任务并调度可用账号。
5. 调用上游渠道商接口。
6. 将结果持久化到对象存储或本地上传目录。
7. 更新任务状态并返回给前端轮询。

关键代码：

- [service.go](./apps/api/internal/accounts/service.go)
- [engine.go](./apps/api/internal/prompts/engine.go)
- [consumer.go](./apps/api/internal/runtime/worker/consumer.go)

## 提示词系统

系统提示词由后台配置项 `feature_prompt:<slug>` 驱动，支持 `{{变量}}` 模板语法。

### 核心规则

- `feature slug` 是唯一索引，不再维护旧兼容名称。
- `{{user_prompt}}` 是用户输入信号，不与旧线稿模板主链混写。
- `mode -> template` 映射仅保留必要项。
- 执行语义相关 hidden constraint 由后端在特定 feature 下追加。

### 常用变量

| 变量 | 说明 |
|------|------|
| `{{user_prompt}}` | 用户输入文本 |
| `{{colors}}` | 颜色列表 |
| `{{target_color}}` | 定向换色目标色 |
| `{{blend_ratio}}` | 融合比例 |
| `{{extend_top}}` `{{extend_right}}` `{{extend_bottom}}` `{{extend_left}}` | 画布延伸参数 |
| `{{tile_mode}}` | Wear 贴图模式 |
| `{{viewpoint}}` | 展示视角描述 |
| `{{lighting}}` | 打光描述 |
| `{{use_mask}}` | 是否启用 mask |
| `{{mask_image_url}}` | Mask 图 URL |

相关入口：

- 管理页：[FeaturePromptConfigScreen.vue](./apps/web/src/features/admin/prompt-config/FeaturePromptConfigScreen.vue)
- 后端装配：[engine.go](./apps/api/internal/prompts/engine.go)

## 鉴权与会话

系统采用 `JWT + Redis token` 双重校验：

- JWT 提供用户身份与角色信息。
- Redis 保存当前有效 token，实现主动失效与登出控制。
- 若 `JWT_SECRET` 未配置，开发环境会生成一次性随机密钥；服务重启后旧 token 会失效。

关键代码：

- [middleware.go](./apps/api/internal/auth/middleware.go)
- [auth.ts](./apps/web/src/shared/utils/auth.ts)
- [user.ts](./apps/web/src/app/stores/user.ts)

<a id="local-dev-fast-path"></a>
## 本地开发快速路径

### 环境要求

| 组件 | 建议版本 |
|------|----------|
| Node.js | `18+` |
| Go | `1.25+` |
| MySQL | `8.0+` |
| Redis | `7+` |

### 启动步骤

1. 启动本地依赖：

```bash
cd docker
docker compose up -d
```

2. 配置后端环境变量：

```bash
cd apps/api
cp .env.example .env
```

3. 启动后端：

```bash
cd apps/api
go run ./cmd
```

4. 启动前端：

```bash
cd apps/web
npm install
npm run dev
```

5. 初始化管理员：

```bash
cd apps/api
export INIT_ADMIN_PASSWORD='your-strong-password'
go run ./cmd init-admin
```

> [!IMPORTANT]
> `apps/api/.env` 中的 `DB_USER / DB_PASSWORD` 是数据库连接账号，不是网站后台管理员账号。管理员登录账号来自业务库里的 `users` 表；本地首次启动时，建议执行一次 `INIT_ADMIN_PASSWORD=... go run ./cmd init-admin`。
>
> 当前仓库不再通过 SQL 种子管理员；若跳过 `init-admin`，空库不会自动生成可登录管理员账号。`init-admin` 既是创建入口，也可在 `admin` 用户已存在且角色为 `admin` 时执行密码重置。

## 验证矩阵

### 前端

```bash
cd apps/web
npm run build
```

### 后端

```bash
cd apps/api
go test ./...
go build ./...
```

### 跨层改动

以下改动默认同时验证前后端：

- 路由
- 鉴权
- 任务 contract
- 上传
- 提示词模板
- 管理后台配置

## 环境变量

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DB_HOST` | `localhost` | MySQL 主机 |
| `DB_PORT` | `3306` | MySQL 端口 |
| `DB_USER` | `ideaboard` | MySQL 用户 |
| `DB_PASSWORD` | 无默认值 | MySQL 密码；使用仓库自带 Docker Compose 时默认业务密码为 `ideaboard123` |
| `DB_NAME` | `ideaboard` | 数据库名 |
| `REDIS_HOST` | `localhost` | Redis 主机 |
| `REDIS_PORT` | `6379` | Redis 端口 |
| `REDIS_AUTH` | 空 | Redis 密码 |
| `APP_ENV` | `development` | 运行环境标识；生产环境必须显式设置为 `production` |
| `APP_PORT` | `6000` | 后端监听端口 |
| `JWT_SECRET` | 空 | JWT 密钥；生产环境必须配置 |
| `ENV_FILE` | 空 | 可选，自定义 `.env` 路径 |
| `DB_LOG_MODE` | `warn` | GORM 日志等级，可选 `silent/error/warn/info` |

<a id="repo-map"></a>
## 仓库目录图

```text
apps/web/src/
├── app/                       # 入口、router、store 装配
├── pages/                     # 路由级容器
├── features/                  # 领域模块
├── shared/                    # 共享 API、UI、types、utils、assets
└── config/                    # 仅保留跨 feature 运行配置

apps/api/
├── cmd/                       # 服务入口
├── internal/platform/         # config、database、redis、response、safehttp
├── internal/runtime/          # bootstrap、http 路由、worker
├── internal/auth/             # 鉴权与 token
├── internal/users/            # 用户管理
├── internal/accounts/         # 账号池与任务调度
├── internal/channels/         # 渠道管理
├── internal/tasks/            # 任务模型与状态
├── internal/workflow/         # feature contract
├── internal/prompts/          # 提示词模板与装配
├── internal/storage/          # 对象存储配置与实现
├── internal/assets/           # 风格卡、人脸库、背景库等资产
└── internal/imageproc/        # 分割、线稿、向量化等图片处理
```

## 维护规则

在继续迭代时，优先遵守以下规则：

1. 以 working tree 为准，不回滚用户未提交改动。
2. 改任务 contract 时，优先同步前端执行链、后端 canonical helper 和 admin 配置页。
3. 已退休 feature 不应继续出现在产品文案、模板兼容或展示分组里。
4. 跨层变更默认需要同时验证前端与后端。
