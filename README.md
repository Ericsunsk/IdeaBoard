<p align="center">
  <img src="./docs/assets/ideaboard-wordmark.png" alt="IdeaBoard" width="461">
</p>

<p align="center">
  <strong>为服装设计与商品展示场景构建的 AI 画布工作流平台。</strong>
</p>

<p align="center">
  在统一画布中完成素材输入、节点编排、任务执行、结果沉淀与后台配置。
</p>

<p align="center">
  <img src="https://img.shields.io/badge/repository-private-4b5563?style=flat-square" alt="仓库：私有" />
  <img src="https://img.shields.io/badge/source-closed-374151?style=flat-square" alt="源码：闭源" />
  <img src="https://img.shields.io/badge/distribution-internal_only-1f2937?style=flat-square" alt="分发：仅限内部" />
</p>

<p align="center">
  <a href="#overview">项目概览</a> ·
  <a href="#highlights">核心亮点</a> ·
  <a href="#capabilities">能力范围</a> ·
  <a href="#quick-start">快速开始</a> ·
  <a href="#deployment">部署说明</a> ·
  <a href="#docs">相关文档</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/web-Vue_3-42b883?style=flat-square&logo=vuedotjs&logoColor=white" alt="Web: Vue 3" />
  <img src="https://img.shields.io/badge/language-TypeScript-3178c6?style=flat-square&logo=typescript&logoColor=white" alt="Language: TypeScript" />
  <img src="https://img.shields.io/badge/ui-Naive_UI-18a058?style=flat-square" alt="UI: Naive UI" />
  <img src="https://img.shields.io/badge/state-Pinia-ffd859?style=flat-square&logo=pinia&logoColor=1f2937" alt="State: Pinia" />
  <br/>
  <img src="https://img.shields.io/badge/api-Go-00add8?style=flat-square&logo=go&logoColor=white" alt="API: Go" />
  <img src="https://img.shields.io/badge/framework-Gin-008ecf?style=flat-square" alt="Framework: Gin" />
  <img src="https://img.shields.io/badge/orm-GORM-1f6feb?style=flat-square" alt="ORM: GORM" />
  <br/>
  <img src="https://img.shields.io/badge/database-MySQL_8-4479a1?style=flat-square&logo=mysql&logoColor=white" alt="Database: MySQL 8" />
  <img src="https://img.shields.io/badge/cache-Redis_8.6-dc382d?style=flat-square&logo=redis&logoColor=white" alt="Cache: Redis 8.6" />
</p>

<a id="product-preview"></a>
## 产品预览

<p align="center">
  <img src="./docs/assets/ideaboard-preview.png" alt="IdeaBoard 画布工作流预览" width="1400">
</p>

<p align="center">
  <sub>统一画布工作流：输入、编辑、模型控制与输出回写在同一界面完成。</sub>
</p>

<a id="overview"></a>
## 项目概览

IdeaBoard 是一套围绕 AI 设计工作流打造的全栈系统。前端提供节点画布、任务入口与管理后台，后端负责鉴权、任务落库、提示词模板装配、账号调度、异步执行、结果持久化，以及 `Local / Kodo / S3-compatible` 存储配置接入。

### 产品快照

| 维度 | 当前配置 |
|------|----------|
| 默认本地端口 | 前端 `5173`，Go API `6000`，Auth Service `6001` |
| 运行方式 | 本地开发、标准 Linux 部署（Nginx + systemd + 双服务） |
| 核心界面 | 用户画布工作台、画板管理、管理后台 |

### 系统架构

```mermaid
graph TD
  %% Client Layer
  subgraph Client [客户端]
    Browser([浏览器 / Web App])
  end

  %% Gateway Layer
  subgraph Gateway [流量入口]
    Nginx[Nginx 反向代理]
  end

  %% Logic Layer
  subgraph Services [核心服务]
    Auth[Better Auth Service<br/>Node.js :6001]
    GoAPI[Go API<br/>Gin :6000]
    Static[静态资源服务<br/>/uploads/*]
  end

  %% Persistence Layer
  subgraph Persistence [持久化与缓存]
    MySQL[(MySQL 8.0)]
    Redis[(Redis 8.6)]
  end

  %% Async Layer
  subgraph Execution [任务执行]
    Worker[Worker 消费者]
    Queue[Redis Queue]
  end

  %% Cloud/Storage Layer
  subgraph External [外部资源 / 存储]
    AI[上游 AI 渠道]
    Storage[对象存储<br/>Local / Kodo / S3]
  end

  %% Traffic Flow
  Browser --> Nginx
  Nginx -->|/api/auth/*| Auth
  Nginx -->|/api/*| GoAPI
  Nginx -->|/uploads/*| Static

  %% Data Flow
  Auth --> MySQL
  Auth --> Redis
  GoAPI --> MySQL
  GoAPI --> Redis

  %% Task Flow
  GoAPI --> Worker
  Worker --> Queue
  Worker --> AI
  Worker --> Storage
  Static --> Storage
```

生产环境由 Nginx 统一转发 `/api/auth/*`、`/api/*` 与 `/uploads/*`，Go API 与 Better Auth Node Service 双服务拓扑运行。

<a id="highlights"></a>
## 核心亮点

<table>
  <tr>
    <td valign="top" width="50%">
      <strong>画布工作流</strong><br/>
      以节点编排为中心，统一管理输入、连接、执行与结果回写。
    </td>
    <td valign="top" width="50%">
      <strong>任务运行时</strong><br/>
      任务异步执行，支持队列调度、账号池分发与状态轮询。
    </td>
  </tr>
  <tr>
    <td valign="top">
      <strong>提示词系统</strong><br/>
      支持按 feature 维度配置系统提示词与模板变量。
    </td>
    <td valign="top">
      <strong>管理控制台</strong><br/>
      管理用户、账号、渠道、统计仪表盘、线稿模式、提示词与配置项。
    </td>
  </tr>
</table>

<a id="capabilities"></a>
## 能力范围

当前产品主链路覆盖以下能力：

<table>
  <tr>
    <td valign="top" width="33%">
      <strong>设计生成</strong><br/>
      线稿成款、款式创新、图案生成
    </td>
    <td valign="top" width="33%">
      <strong>编辑调整</strong><br/>
      局部改款、换背景、换脸、双图融合
    </td>
    <td valign="top" width="33%">
      <strong>色彩与材质</strong><br/>
      多色配色、定向换色、上身贴合
    </td>
  </tr>
  <tr>
    <td valign="top">
      <strong>展示与视角</strong><br/>
      单视角展示、商品展示
    </td>
    <td valign="top">
      <strong>后处理</strong><br/>
      高清增强、画布延伸
    </td>
    <td valign="top">
      <strong>运营统计</strong><br/>
      调用聚合、模型占比、健康热力图
    </td>
  </tr>
  <tr>
    <td valign="top">
      <strong>存储接入</strong><br/>
      本地存储、Kodo、S3-compatible
    </td>
    <td valign="top">
      <strong>身份认证</strong><br/>
      登录、注册、会话、角色、封禁
    </td>
    <td valign="top">
      <strong>任务调度</strong><br/>
      队列、账号池、失败重试、模型解析
    </td>
  </tr>
</table>

更细的 workflow feature contract、提示词模板变量与执行链路说明见 [DEVELOPMENT.md](./DEVELOPMENT.md)。如需按 feature 配置管理后台系统提示词，可参考 [SYSTEM_PROMPT.md](./SYSTEM_PROMPT.md)。

<a id="quick-start"></a>
## 快速开始

### 环境要求

| 组件 | 版本 |
|------|------|
| Node.js | `20+` |
| Go | `1.25+` |
| MySQL | `8.0+` |
| Redis | `8.6+` |

### 1. 启动本地依赖

```bash
cd docker
docker compose up -d
```

<details>
<summary>默认本地依赖配置</summary>

| 项目 | 默认值 |
|------|--------|
| MySQL Root 密码 | `root123` |
| 业务数据库 | `ideaboard` |
| 业务用户 | `ideaboard` |
| 业务密码 | `ideaboard123` |
| Redis | `127.0.0.1:6379` |

</details>

> [!NOTE]
> `docker compose` 默认只绑定到本机回环地址 `127.0.0.1`，避免把 MySQL / Redis 直接暴露到外网。若你需要 Redis 密码，可在 `docker/.env` 中设置 `REDIS_PASSWORD`，并在 `apps/api/.env` 中同步设置 `REDIS_AUTH`。

### 2. 配置后端环境

```bash
cd apps/api
cp .env.example .env
```

> [!TIP]
> 至少确认数据库、Redis、端口以及 `AUTH_SERVICE_BASE_URL` / `AUTH_SERVICE_INTERNAL_KEY` 配置正确。`.env.example` 里的敏感字段是占位值，不应直接用于部署。

如果后端放在 Nginx / 宝塔等反向代理后面，建议额外设置：

```env
APP_HOST=127.0.0.1
```

这样 `ideaboard-server` 只监听本机回环地址，避免被绕过代理直接访问。

### 3. 启动 Auth 服务

```bash
cd apps/auth
cp .env.example .env
npm install
npm run migrate
npm run dev
```

> [!IMPORTANT]
> 启用 Better Auth Admin plugin 后，请先执行 `npm --prefix apps/auth run migrate`（生产环境执行 `npm --prefix apps/auth run migrate:dist`），以补齐 `user.role`、`user.banned`、`user.banReason`、`user.banExpires` 与 `session.impersonatedBy` 等字段。

<details>
<summary>Auth 服务能力说明</summary>

- 身份侧高级管理入口：后台 `用户管理` 页。
- 当前支持：角色升降级、身份侧封禁/解封、会话吊销/踢下线、伪装登录/停止伪装、受控彻底注销、开放注册开关、防刷限流设置。
- 仍不开放：后台改密；彻底注销必须经由 Go Admin API 编排，不允许前端直连 Better Auth 删除身份数据。

</details>

### 4. 启动后端（Go API）

```bash
cd apps/api
go run ./cmd
```

### 5. 启动前端

```bash
cd apps/web
npm install
npm run dev
```

<details>
<summary>开发环境代理说明</summary>

- `apps/web/vite.config.ts` 已将 `/api/auth` 代理到 `http://localhost:6001`
- 其余 `/api/*` 与 `/uploads/*` 仍代理到 `http://localhost:6000`

</details>

### 6. 初始化管理员账号

```bash
cd apps/api
INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin
```

- 用户名：`admin`
- 密码：由 `INIT_ADMIN_PASSWORD` 指定

> [!IMPORTANT]
> `init-admin` 会调用 auth service 确保 Better Auth 存在 `admin` 账号，并绑定/创建本地 admin profile。执行该命令前请先启动 `apps/auth`。

### 本地访问地址

| 地址 | 用途 |
|------|------|
| `http://localhost:5173` | 前端开发环境 |
| `http://localhost:5173/admin/login` | 管理后台登录 |
| `http://localhost:6000/api/health` | 后端健康检查 |
| `http://localhost:6001/api/health` | Better Auth 健康检查 |

## 仓库结构

```text
IdeaBoard/
├── apps/
│   ├── web/                  # Vue 3 前端与管理后台
│   │   └── src/features/     # 业务模块（workspace、admin、profile 等）
│   ├── auth/                 # Better Auth 服务（登录/注册/session）
│   └── api/                  # Go API、任务、Worker、服务逻辑
│       └── internal/         # 领域包（accounts、channels、users、workflow 等）
├── docs/                     # 架构约束与补充文档
├── docker/                   # 本地 MySQL / Redis 依赖
├── scripts/                  # 打包与部署脚本
└── *.md                      # README、DEVELOPMENT、DEPLOYMENT、CHANGELOG
```

<a id="deployment"></a>
## 部署说明

### 标准 Linux 部署

适用于 `systemd + Nginx` 的常规服务器环境。

```bash
./scripts/package-deploy.sh
```

详见 [DEPLOYMENT.md](./DEPLOYMENT.md)。

运行拓扑见上方"系统架构"图。

<a id="docs"></a>
## 相关文档

| 文档 | 用途 |
|------|------|
| [DEVELOPMENT.md](./DEVELOPMENT.md) | 架构、contract、本地开发与目录边界 |
| [SYSTEM_PROMPT.md](./SYSTEM_PROMPT.md) | Feature System Prompt 配置手册与推荐模板 |
| [DEPLOYMENT.md](./DEPLOYMENT.md) | 标准 Linux 部署、升级与回滚 |
| [docs/AUTH_OPERATION_RUNBOOK.md](./docs/AUTH_OPERATION_RUNBOOK.md) | Auth 运维边界与应急流程 |
| [docs/architecture-constraints.md](./docs/architecture-constraints.md) | 目录边界与命名约束 |
| [docker/README.md](./docker/README.md) | 本地依赖说明 |
| [CHANGELOG.md](./CHANGELOG.md) | 发布记录 |

## 验证建议

上线前建议至少确认：

1. `curl http://127.0.0.1:6000/api/health` 返回成功。
2. `curl http://127.0.0.1:6001/api/health` 返回成功。
3. 用户端首页可正常访问。
4. 管理员可登录后台。
5. 图片上传与结果回写正常。
6. 任务提交后能看到完整状态流转。

<p align="center">
  <sub>本项目为闭源私有项目，未经授权不得复制、分发或使用。保留所有权利。</sub>
</p>
