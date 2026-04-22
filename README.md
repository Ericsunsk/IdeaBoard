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
  <img src="https://img.shields.io/badge/repository-private-4b5563?style=flat-square" alt="Repository: Private" />
  <img src="https://img.shields.io/badge/source-closed-374151?style=flat-square" alt="Source: Closed" />
  <img src="https://img.shields.io/badge/distribution-internal_only-1f2937?style=flat-square" alt="Distribution: Internal Only" />
</p>

<p align="center">
  <a href="#overview">Overview</a> ·
  <a href="#highlights">Highlights</a> ·
  <a href="#capabilities">Capabilities</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#deployment">Deployment</a> ·
  <a href="#docs">Docs</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/frontend-Vue_3-42b883?style=flat-square&logo=vuedotjs&logoColor=white" alt="Frontend: Vue 3" />
  <img src="https://img.shields.io/badge/language-TypeScript-3178c6?style=flat-square&logo=typescript&logoColor=white" alt="Language: TypeScript" />
  <img src="https://img.shields.io/badge/ui-Naive_UI-18a058?style=flat-square" alt="UI: Naive UI" />
  <img src="https://img.shields.io/badge/state-Pinia-ffd859?style=flat-square&logo=pinia&logoColor=1f2937" alt="State: Pinia" />
  <img src="https://img.shields.io/badge/backend-Go-00add8?style=flat-square&logo=go&logoColor=white" alt="Backend: Go" />
  <img src="https://img.shields.io/badge/framework-Gin-008ecf?style=flat-square" alt="Framework: Gin" />
  <img src="https://img.shields.io/badge/orm-GORM-1f6feb?style=flat-square" alt="ORM: GORM" />
  <img src="https://img.shields.io/badge/database-MySQL_8-4479a1?style=flat-square&logo=mysql&logoColor=white" alt="Database: MySQL 8" />
  <img src="https://img.shields.io/badge/cache-Redis_7-dc382d?style=flat-square&logo=redis&logoColor=white" alt="Cache: Redis 7" />
</p>

## Product Preview

<p align="center">
  <img src="./docs/assets/ideaboard-preview.png" alt="IdeaBoard canvas workflow preview" width="1400">
</p>

<p align="center">
  <sub>统一画布工作流：输入、编辑、模型控制与输出回写在同一界面完成。</sub>
</p>

---

## Overview

IdeaBoard 是一套围绕 AI 设计工作流打造的全栈系统。前端提供节点画布、任务入口与管理后台，后端负责鉴权、任务落库、提示词模板装配、账号调度、异步执行、结果持久化，以及 `Local / Kodo / S3-compatible` 存储配置接入。

### Product Snapshot

| 维度 | 当前配置 |
|------|----------|
| 前端 | `Vue 3` + `Vite` + `TypeScript` + `Naive UI` + `Pinia` |
| 后端 | `Go` + `Gin` + `GORM` |
| 基础设施 | `MySQL 8.0+`、`Redis 7+` |
| 默认本地端口 | 前端 `5173`，后端 `6000` |
| 运行方式 | 本地开发、标准 Linux 部署、宝塔 Go 单体部署 |
| 核心界面 | 用户画布工作台、画板管理、管理后台 |

### Runtime Shape

```text
Canvas UI
  -> API / Auth / Upload
  -> Task Submission
  -> Redis Queue
  -> Worker Dispatch
  -> Upstream AI Channels
  -> Result Persistence
```

## Highlights

<table>
  <tr>
    <td valign="top" width="25%">
      <strong>Canvas Workflow</strong><br/>
      以节点编排为中心，统一管理输入、连接、执行与结果回写。
    </td>
    <td valign="top" width="25%">
      <strong>Task Runtime</strong><br/>
      任务异步执行，支持队列调度、账号池分发与状态轮询。
    </td>
    <td valign="top" width="25%">
      <strong>Prompt System</strong><br/>
      支持按 feature 维度配置系统提示词与模板变量。
    </td>
    <td valign="top" width="25%">
      <strong>Admin Console</strong><br/>
      管理用户、账号、渠道、素材模板、提示词与配置项。
    </td>
  </tr>
</table>

## Capabilities

当前产品主链路覆盖以下能力：

| 能力类别 | 示例能力 |
|----------|----------|
| 设计生成 | 线稿成款、款式创新、图案生成 |
| 编辑调整 | 局部改款、换背景、换脸、双图融合 |
| 色彩与材质 | 多色配色、定向换色、上身贴合 |
| 展示与视角 | 单视角展示、商品展示 |
| 后处理 | 高清增强、画布延伸 |
| 存储接入 | 本地存储、Kodo、S3-compatible 对象存储 |

更细的 workflow feature contract、提示词模板变量与执行链路说明见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

## Quick Start

### 1. Start Local Dependencies

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

### 2. Configure Backend Environment

```bash
cd backend
cp .env.example .env
```

至少确认数据库、Redis、端口和 `JWT_SECRET` 配置正确。

### 3. Run the Backend

```bash
cd backend
go run cmd/main.go
```

### 4. Run the Frontend

```bash
cd frontend
npm install
npm run dev
```

### 5. Initialize the Admin Account

```bash
cd backend
INIT_ADMIN_PASSWORD='your-strong-password' go run cmd/main.go init-admin
```

默认管理员初始化：

- 用户名：`admin`
- 密码：由 `INIT_ADMIN_PASSWORD` 指定

### Local Access

| 地址 | 用途 |
|------|------|
| `http://localhost:5173` | 前端开发环境 |
| `http://localhost:5173/admin/login` | 管理后台登录 |
| `http://localhost:6000/api/health` | 后端健康检查 |

## Repository Layout

```text
IdeaBoard/
├── frontend/                 # 前端与管理后台
├── backend/                  # API、任务、Worker、服务逻辑
├── docker/                   # 本地 MySQL / Redis 依赖
├── scripts/                  # 打包与部署脚本
├── README.md                 # 项目首页
├── DEVELOPMENT.md            # 开发手册
├── DEPLOYMENT.md             # 标准 Linux 部署
└── DEPLOY_BAOTA_GO.md        # 宝塔 Go 部署
```

## Deployment

### Standard Linux Deployment

适用于 `systemd + Nginx` 的常规服务器环境。

```bash
./scripts/package-deploy.sh
```

详见 [DEPLOYMENT.md](./DEPLOYMENT.md)。

### Baota Go Deployment

适用于以前后端单可执行方式部署的场景。

```bash
./scripts/package-bt-go.sh
```

详见 [DEPLOY_BAOTA_GO.md](./DEPLOY_BAOTA_GO.md)。

## Docs

| 文档 | 用途 |
|------|------|
| [README.md](./README.md) | 项目首页与快速启动 |
| [DEVELOPMENT.md](./DEVELOPMENT.md) | 架构、contract、本地开发与目录边界 |
| [DEPLOYMENT.md](./DEPLOYMENT.md) | 标准 Linux 部署、升级与回滚 |
| [DEPLOY_BAOTA_GO.md](./DEPLOY_BAOTA_GO.md) | 宝塔 Go 部署 |
| [docker/README.md](./docker/README.md) | 本地依赖说明 |
| [CHANGELOG.md](./CHANGELOG.md) | 发布记录与未发布变更 |

## Verification

上线前建议至少确认：

1. `curl http://127.0.0.1:6000/api/health` 返回成功。
2. 用户端首页可正常访问。
3. 管理员可登录后台。
4. 图片上传与结果回写正常。
5. 任务提交后能看到完整状态流转。

## Rights

本项目为闭源私有项目，未经授权不得复制、分发或使用。

`All Rights Reserved.`
