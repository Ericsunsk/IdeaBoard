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
  <img src="https://img.shields.io/badge/api-Go-00add8?style=flat-square&logo=go&logoColor=white" alt="API: Go" />
  <img src="https://img.shields.io/badge/framework-Gin-008ecf?style=flat-square" alt="Framework: Gin" />
  <img src="https://img.shields.io/badge/orm-GORM-1f6feb?style=flat-square" alt="ORM: GORM" />
  <img src="https://img.shields.io/badge/database-MySQL_8-4479a1?style=flat-square&logo=mysql&logoColor=white" alt="Database: MySQL 8" />
  <img src="https://img.shields.io/badge/cache-Redis_7-dc382d?style=flat-square&logo=redis&logoColor=white" alt="Cache: Redis 7" />
</p>

<a id="product-preview"></a>
## 产品预览

<p align="center">
  <img src="./docs/assets/ideaboard-preview.png" alt="IdeaBoard 画布工作流预览" width="1400">
</p>

<p align="center">
  <sub>统一画布工作流：输入、编辑、模型控制与输出回写在同一界面完成。</sub>
</p>

---

<a id="overview"></a>
## 项目概览

IdeaBoard 是一套围绕 AI 设计工作流打造的全栈系统。前端提供节点画布、任务入口与管理后台，后端负责鉴权、任务落库、提示词模板装配、账号调度、异步执行、结果持久化，以及 `Local / Kodo / S3-compatible` 存储配置接入。

### 产品快照

| 维度 | 当前配置 |
|------|----------|
| 前端 | `Vue 3` + `Vite` + `TypeScript` + `Naive UI` + `Pinia` |
| 后端 | `Go` + `Gin` + `GORM` + `Better Auth (Node.js/TypeScript)` |
| 基础设施 | `MySQL 8.0+`、`Redis 7+` |
| 默认本地端口 | 前端 `5173`，Go API `6000`，Auth Service `6001` |
| 运行方式 | 本地开发、标准 Linux 部署、宝塔双服务托管（需单独配置 Auth Service） |
| 核心界面 | 用户画布工作台、画板管理、管理后台 |

### 运行链路

```text
画布界面
  -> API / Auth / Upload
  -> 任务提交
  -> Redis Queue
  -> Worker 调度
  -> 上游 AI 渠道
  -> 结果持久化
```

<a id="highlights"></a>
## 核心亮点

<table>
  <tr>
    <td valign="top" width="25%">
      <strong>画布工作流</strong><br/>
      以节点编排为中心，统一管理输入、连接、执行与结果回写。
    </td>
    <td valign="top" width="25%">
      <strong>任务运行时</strong><br/>
      任务异步执行，支持队列调度、账号池分发与状态轮询。
    </td>
    <td valign="top" width="25%">
      <strong>提示词系统</strong><br/>
      支持按 feature 维度配置系统提示词与模板变量。
    </td>
    <td valign="top" width="25%">
      <strong>管理控制台</strong><br/>
      管理用户、账号、渠道、素材模板、提示词与配置项。
    </td>
  </tr>
</table>

<a id="capabilities"></a>
## 能力范围

当前产品主链路覆盖以下能力：

| 能力类别 | 示例能力 |
|----------|----------|
| 设计生成 | 线稿成款、款式创新、图案生成 |
| 编辑调整 | 局部改款、换背景、换脸、双图融合 |
| 色彩与材质 | 多色配色、定向换色、上身贴合 |
| 展示与视角 | 单视角展示、商品展示 |
| 后处理 | 高清增强、画布延伸 |
| 存储接入 | 本地存储、Kodo、S3-compatible 对象存储 |

更细的 workflow feature contract、提示词模板变量与执行链路说明见 [DEVELOPMENT.md](./DEVELOPMENT.md)。如需按 feature 配置 admin 后台系统提示词，可参考 [SYSTEM_PROMPT.md](./SYSTEM_PROMPT.md)。

<a id="quick-start"></a>
## 快速开始

### 1. 启动本地依赖

```bash
cd docker
docker compose up -d
```

默认本地依赖：

| 项目 | 默认值 |
|------|--------|
| MySQL Root 密码 | `root123` |
| 业务数据库 | `ideaboard` |
| 业务用户 | `ideaboard` |
| 业务密码 | `ideaboard123` |
| Redis | `127.0.0.1:6379` |

说明：

- `docker compose` 默认只绑定到本机回环地址 `127.0.0.1`，避免把 MySQL / Redis 直接暴露到外网。
- 若你需要 Redis 密码，可在 `docker/.env` 中设置 `REDIS_PASSWORD`，并在 `apps/api/.env` 中同步设置 `REDIS_AUTH`。

### 2. 配置后端环境

```bash
cd apps/api
cp .env.example .env
```

至少确认数据库、Redis、端口以及 `AUTH_SERVICE_BASE_URL / AUTH_SERVICE_INTERNAL_KEY` 配置正确；`.env.example` 里的敏感字段是占位值，不应直接用于部署。

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

> 启用 Better Auth Admin plugin 后，请先执行 `npm --prefix apps/auth run migrate`（生产环境执行 `npm --prefix apps/auth run migrate:dist`），以补齐 `user.role`、`user.banned`、`user.banReason`、`user.banExpires` 与 `session.impersonatedBy` 等字段。

Auth 服务建议额外确认：

- `AUTH_REGISTRATION_ENABLED`：开发环境可设为 `true`；生产建议显式设为 `false`（按运营策略再开启）
- `AUTH_RATE_LIMIT_WINDOW_SECONDS` / `AUTH_RATE_LIMIT_MAX_REQUESTS`：控制 `/api/auth/sign-in/*` 与 `/api/auth/sign-up/*` 的内存限流窗口与阈值
- 身份侧高级管理入口：后台 `用户管理 -> 高级选项`（当前支持身份用户列表、身份侧封禁/解封、伪装登录/停止伪装；不暴露后台改密或物理删号）

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

开发环境代理说明：

- `apps/web/vite.config.ts` 已将 `/api/auth` 代理到 `http://localhost:6001`
- 其余 `/api/*` 与 `/uploads/*` 仍代理到 `http://localhost:6000`

### 6. 初始化管理员账号

```bash
cd apps/api
INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin
```

管理员初始化：

- 用户名：`admin`
- 密码：由 `INIT_ADMIN_PASSWORD` 指定

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
│   ├── auth/                 # Better Auth 服务（登录/注册/session）
│   └── api/                  # Go API、任务、Worker、服务逻辑
├── docs/                     # 架构约束与补充文档
├── docker/                   # 本地 MySQL / Redis 依赖
├── scripts/                  # 打包与部署脚本
├── README.md                 # 项目首页
├── DEVELOPMENT.md            # 开发手册
├── DEPLOYMENT.md             # 标准 Linux 部署
└── DEPLOY_BAOTA_GO.md        # 宝塔旧单 Go 模式停用说明
```

<a id="deployment"></a>
## 部署说明

### 标准 Linux 部署

适用于 `systemd + Nginx` 的常规服务器环境。

```bash
./scripts/package-deploy.sh
```

详见 [DEPLOYMENT.md](./DEPLOYMENT.md)。

### 宝塔 Go 模式已停用

Better Auth 迁移后需要独立的 `apps/auth` Node Service，旧的单 Go 二进制部署方式已停用；`scripts/package-bt-go.sh` 会直接失败以避免生成不可登录的发布包。

详见 [DEPLOY_BAOTA_GO.md](./DEPLOY_BAOTA_GO.md)。

<a id="docs"></a>
## 相关文档

| 文档 | 用途 |
|------|------|
| [README.md](./README.md) | 项目首页与快速启动 |
| [DEVELOPMENT.md](./DEVELOPMENT.md) | 架构、contract、本地开发与目录边界 |
| [SYSTEM_PROMPT.md](./SYSTEM_PROMPT.md) | Feature System Prompt 配置手册与推荐模板 |
| [DEPLOYMENT.md](./DEPLOYMENT.md) | 标准 Linux 部署、升级与回滚 |
| [DEPLOY_BAOTA_GO.md](./DEPLOY_BAOTA_GO.md) | 宝塔旧单 Go 模式停用说明 |
| [docs/AUTH_OPERATION_RUNBOOK.md](./docs/AUTH_OPERATION_RUNBOOK.md) | Auth 运维边界与应急流程 |
| [docs/architecture-constraints.md](./docs/architecture-constraints.md) | 新目录边界与命名约束 |
| [docker/README.md](./docker/README.md) | 本地依赖说明 |
| [CHANGELOG.md](./CHANGELOG.md) | 发布记录与未发布变更 |

## 验证建议

上线前建议至少确认：

1. `curl http://127.0.0.1:6000/api/health` 返回成功。
2. `curl http://127.0.0.1:6001/api/health` 返回成功。
3. 用户端首页可正常访问。
4. 管理员可登录后台。
5. 图片上传与结果回写正常。
6. 任务提交后能看到完整状态流转。

## 权利声明

本项目为闭源私有项目，未经授权不得复制、分发或使用。

`保留所有权利。`
