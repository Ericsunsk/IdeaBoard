# IdeaBoard

<p align="center">
  <img src="./docs/assets/ideaboard-wordmark.png" alt="IdeaBoard Wordmark" width="480">
</p>

<p align="center">
  <strong>面向视觉设计师与电商场景的高性能 AI 画布工作流平台</strong><br>
  <em>Next-Generation AI Canvas for Visual Designers to Explore, Compare & Refine Image Directions in One Workspace.</em>
</p>

<p align="center">
  <a href="https://ideaboard.art"><img src="https://img.shields.io/badge/Website-ideaboard.art-000000?style=for-the-badge&logo=cloudflare&logoColor=white" alt="Live Site"></a>
  <a href="#-技术栈矩阵-tech-stack-matrix"><img src="https://img.shields.io/badge/Version-v0.1.8-blue?style=for-the-badge" alt="Version"></a>
  <a href="#-核心处理主链路-end-to-end-sequence"><img src="https://img.shields.io/badge/CI%2FCD-Passing-10B981?style=for-the-badge&logo=githubactions&logoColor=white" alt="CI/CD Status"></a>
  <a href="#-四层架构组件分层-system-architecture-flow"><img src="https://img.shields.io/badge/AI_Engine-Multi--Provider_Router-8B5CF6?style=for-the-badge&logo=openai&logoColor=white" alt="Multi-Provider AI Engine"></a>
</p>

> [!NOTE]
> **IdeaBoard** 打破了传统单图生成的线性对话束缚。它引入无边界 **Visual Canvas Workspace**，允许设计师在同一个画布中并排比对图案方向、执行局部重绘、多图融合以及 4K 高清渲染。

---

## 📸 核心界面预览 (Workspace Preview)

<p align="center">
  <img src="./docs/assets/ideaboard-preview.png" alt="IdeaBoard 无限画布工作流" width="100%" style="border-radius: 12px; box-shadow: 0 12px 32px rgba(0,0,0,0.15);">
</p>

---

## 🍱 核心能力 Bento Grid 网格矩阵 (Key Highlights)

| 核心特性 | 架构能力 | 商业与工程价值 |
| :--- | :--- | :--- |
| 🎨 **无限 AI 画布** *(Infinite Canvas)* | Vue Flow 可视化节点编排、连接线自动引流、多任务并行渲染 | 告别线性对话束缚，提供网格对比与无边界视觉探索 |
| 🔀 **多模型智能路由** *(Multi-Provider Router)* | 统一 `GenerationContract` 契约，支持 Google Vertex AI、OpenAI、Stability AI 等 Provider | 无缝支持预设与 `Auto` 原生尺寸，高清晰度与风格多变 |
| 🚀 **极速无锁任务队列** *(High-Perf Queue)* | PostgreSQL 17 事务入队 + River Worker 调度，带心跳与续租 | 数据库事务写入，零脏数据，长任务与多变体生成绝不误杀 |
| 💰 **按比例 Partial 结算** *(Partial Settlement)* | 按实际成功输出的图片张数精准扣费，未成功变体自动秒级退款 | 杜绝“成功一部分全额扣费”的坏体验，精准保障用户权益 |
| 💳 **灵活 Credit 计量与配额** *(Pricing & Quota)* | 灵活的 Credit 积分扣减与配额管理，支持多节点动态计费系数 | 健全的商业化计费体系，支持额度管控、充值与配额审计 |
| 🛡️ **企业级安全与存储** *(Sec & Infra)* | Better Auth、OTP 验证、Google OAuth 2.0、Cloudflare R2 / WAF 防线 | 零信任架构、安全密钥转义、资产持久化与全球 CDN 加速 |

---

## 📐 核心架构与四层拓扑 (4-Tier Architecture Topology)

### 1️⃣ 四层架构组件分层 (System Architecture Flow)

```mermaid
graph TD
    classDef client fill:#1E293B,stroke:#64748B,color:#F8FAFC
    classDef api fill:#0F172A,stroke:#3B82F6,color:#F8FAFC
    classDef queue fill:#18181B,stroke:#10B981,color:#F8FAFC
    classDef ai fill:#2e1065,stroke:#8B5CF6,color:#F8FAFC

    subgraph Layer1 ["💻 应用表现层 (Presentation Layer)"]
        Canvas["Vue 3 Canvas Workspace"]
        Admin["Admin Management UI"]
        Chrome["Image Mask & Adjust Tools"]
    end

    subgraph Layer2 ["🚀 核心 API 层 (API Gateway & Core Logic)"]
        GinAPI["Go API (Gin Router)"]
        Contract["GenerationContract Compiler"]
        Intake["Task Intake & Readiness"]
    end

    subgraph Layer3 ["🗄️ 任务引擎与存储 (Engine & Persistence)"]
        PG17[("PostgreSQL 17 Database")]
        River["River Worker Task Queue"]
        NATS["NATS JetStream Event Fanout"]
        R2["Cloudflare R2 Storage"]
    end

    subgraph Layer4 ["⚡ 智能模型路由 (Multi-Provider AI Router)"]
        Vertex["Google Vertex AI Provider"]
        OpenAI["OpenAI Provider"]
        Stability["Stability AI / Custom Providers"]
    end

    Canvas -->|REST / SSE| GinAPI
    Admin -->|REST| GinAPI
    Chrome -->|State Mutation| Canvas
    GinAPI -->|Validation & Freeze| Contract
    Contract -->|Prepare Media| Intake
    Intake -->|Transactional Enqueue| PG17
    Intake -->|Enqueue Job| River
    River -->|Lease Heartbeat & Execute| Vertex
    River -->|Lease Heartbeat & Execute| OpenAI
    River -->|Lease Heartbeat & Execute| Stability
    River -->|Persist Media Asset| R2
    River -->|Event Fanout| NATS
    NATS -->|SSE Realtime Stream| Canvas

    class Canvas,Admin,Chrome client
    class GinAPI,Contract,Intake api
    class PG17,River,NATS,R2 queue
    class Vertex,OpenAI,Stability ai
```

### 2️⃣ 任务提交与处理主链路 (End-to-End Sequence)

```mermaid
sequenceDiagram
    autonumber
    actor Designer as 🎨 视觉设计师
    participant Canvas as 💻 Vue 3 Canvas Workspace
    participant API as 🚀 Go API (Gin + sqlc)
    participant Queue as 🗄️ River Task Queue (PG 17)
    participant AI as ⚡ Multi-Provider AI Engine
    participant Storage as ☁️ Cloudflare R2 / Local Storage

    Designer->>Canvas: 编排画布节点 & 提交 Prompt / 参考图
    Canvas->>API: POST /api/user/tasks (校验配额与 GenerationContract 契约)
    API->>Queue: PG 事务写入 Task 并入队 River Worker
    Queue->>AI: 发起毫秒级实时生成 (Multi-Provider Dispatch)
    AI-->>Queue: 返回 Base64 / 图像数据或变体
    Queue->>Storage: 持久化至 Cloudflare R2 / S3
    Storage-->>Canvas: SSE / 轮询实时推送回写节点 (支持 1~4 变体占位)
    Canvas-->>Designer: 在画布中并行呈现对比成果
```

---

## 💻 技术栈矩阵 (Tech Stack Matrix)

<p align="left">
  <!-- Frontend -->
  <img src="https://img.shields.io/badge/Vue.js_3-4FC08D?style=flat-square&logo=vuedotjs&logoColor=white" alt="Vue 3" />
  <img src="https://img.shields.io/badge/Vite-646CFF?style=flat-square&logo=vite&logoColor=white" alt="Vite" />
  <img src="https://img.shields.io/badge/TypeScript-3178C6?style=flat-square&logo=typescript&logoColor=white" alt="TypeScript" />
  <img src="https://img.shields.io/badge/Tailwind_CSS_v4-06B6D4?style=flat-square&logo=tailwindcss&logoColor=white" alt="Tailwind CSS v4" />
  <img src="https://img.shields.io/badge/Vue_Flow-10B981?style=flat-square&logo=vue.js&logoColor=white" alt="Vue Flow" />
  <!-- Backend -->
  <img src="https://img.shields.io/badge/Go_1.26+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go" />
  <img src="https://img.shields.io/badge/Gin_Framework-008080?style=flat-square&logo=go&logoColor=white" alt="Gin" />
  <img src="https://img.shields.io/badge/sqlc-Typed_Query-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="sqlc" />
  <img src="https://img.shields.io/badge/River_Queue-PG17_Native-10B981?style=flat-square&logo=postgresql&logoColor=white" alt="River Queue" />
  <!-- Storage & Infra -->
  <img src="https://img.shields.io/badge/PostgreSQL_17-4169E1?style=flat-square&logo=postgresql&logoColor=white" alt="PostgreSQL 17" />
  <img src="https://img.shields.io/badge/Cloudflare_R2-F38020?style=flat-square&logo=cloudflare&logoColor=white" alt="Cloudflare R2" />
  <img src="https://img.shields.io/badge/NATS_JetStream-375BD2?style=flat-square&logo=nats&logoColor=white" alt="NATS" />
  <img src="https://img.shields.io/badge/Docker-2496ED?style=flat-square&logo=docker&logoColor=white" alt="Docker" />
</p>

| 服务层级 | 技术选型与用途 |
| :--- | :--- |
| **Web Frontend** | Vue 3 (Composition API), Vite, TypeScript, Tailwind CSS v4, PrimeVue v4, Vue Flow |
| **Go API Backend** | Go 1.26+, Gin Framework, sqlc (Typed Query Engine), River Queue (PG 17 Native Queue) |
| **Auth Service** | Better Auth, Fastify, TypeScript, OTP & Google OAuth 2.0 |
| **Database & Cache** | PostgreSQL 17 (Triggers, JSONB, Composite Indexes), Usage Per-process LRU Cache |
| **Message & Events** | NATS JetStream (Audit Trail & Event Fanout), SSE Realtime Adapter |
| **Deploy & Infra** | Cloudflare Pages, VPS Reverse Proxy, Docker Compose, GHCR Registry |

---

## ⚡ 本地服务端口地图 (Default Ports)

| 服务层级 | 服务名称 | 本地监听端口 | 访问说明 / 健康检查 |
| :--- | :--- | :--- | :--- |
| **Web Frontend** | `apps/web` | `http://localhost:5173` | Vue 3 SPA 画布界面与 Admin 登录 |
| **Go API** | `apps/api` | `http://localhost:6000` | `/api/health` 核心业务与 Task 接口 |
| **Auth Service** | `apps/auth` | `http://localhost:6001` | `/api/health` 认证与 OAuth 回调服务 |
| **NATS Event** | `NATS` | `4222` / `8222` | 事件广播与实时 Fanout 服务 |

---

## 🛠️ 快速开始 (Quickstart Guide)

> [!TIP]
> **环境要求**：Node.js `>= 22.23.2` \| Go `>= 1.26.5` \| PostgreSQL `>= 17`

<details open>
<summary><b>1️⃣ 启动本地数据库与依赖 (Docker Compose)</b></summary>

```bash
cd docker
cp .env.example .env
cp migration.env.example migration.env
docker compose -f compose.local.yml up -d
```
</details>

<details open>
<summary><b>2️⃣ 初始化数据库 Schema 与 Auth 服务</b></summary>

```bash
# 初始化 Go API Schema
cd apps/api && cp .env.example .env
set -a && source ../../docker/migration.env && set +a
go run ./cmd migrate

# 初始化并启动 Auth 服务
cd ../auth && cp .env.example .env
npm install
set -a && source ../../docker/migration.env && set +a
npm run migrate && npm run dev
```
</details>

<details open>
<summary><b>3️⃣ 启动 Go API 与 Web 前端</b></summary>

```bash
# 启动 Go API Backend (端口 :6000)
cd apps/api && go run ./cmd

# 启动 Web 前端 (端口 :5173)
cd apps/web && npm install && npm run dev
```
</details>

<details>
<summary>🔑 <b>点击展开：初始化超级管理员账号命令</b></summary>

```bash
cd apps/api
INIT_ADMIN_EMAIL='admin@ideaboard.art' \
INIT_ADMIN_DISPLAY_NAME='Admin' \
INIT_ADMIN_PASSWORD='your-strong-password' \
  go run ./cmd init-admin
```
*注：`INIT_ADMIN_PASSWORD` 必须为 10–128 个字符；命令执行前 Auth Service 必须已启动。*
</details>

---

## 📂 仓库架构结构 (Repository Structure)

```text
IdeaBoard/
├── 📁 apps/
│   ├── 💻 web/                  # Vue 3 SPA、Canvas Workspace、Admin UI
│   ├── 🚀 api/                  # Go API、Tasking Worker、Multi-Provider AI Adapters
│   └── 🔒 auth/                 # Better Auth 服务
├── 📁 .github/
│   ├── workflows/            # GitHub Actions CI/CD 流水线 (api, web, auth, docker)
│   └── actions/              # 自定义部署 Action 模块
├── 📁 docker/                   # 本地开发与生产 Docker Compose 编排
├── 📁 deploy/                   # Nginx 反向代理与 Cloudflare WAF 配置
├── 📁 scripts/                  # CI 校验、Migration 契约与部署自动化脚本
└── 📁 docs/                     # 架构文档、部署 Runbooks 与视觉资产
```

---

## 📖 核心文档入口 (Documentation Sitemap)

| 文档 | 说明与用途 |
| :--- | :--- |
| 📘 [DESIGN.md](./DESIGN.md) | 系统设计、模块边界、关键 invariants |
| 🛠️ [DEVELOPMENT.md](./DEVELOPMENT.md) | 本地开发指南、代码入口与验证矩阵 |
| 🎨 [apps/web/DESIGN.md](./apps/web/DESIGN.md) | Agent 做前端 UI 改动时的规范与 UI 执行协议 |
| 🚢 [docs/runbooks/deployment.md](./docs/runbooks/deployment.md) | Cloudflare + Docker Compose 部署、升级与回滚 Runbook |
| 🔐 [docs/runbooks/auth-operations.md](./docs/runbooks/auth-operations.md) | Auth 运营与高风险身份操作边界 |
| 🛡️ [docs/runbooks/data-protection.md](./docs/runbooks/data-protection.md) | PostgreSQL backup/restore 与密钥材料处理 |

---

## 🧪 自动化测试验证 (Verification)

运行日常最小验证集：
```bash
cd apps/web && npm run test:run      # 前端单元测试
cd apps/api && go test ./...         # 后端 Go 单元测试
```

---

<p align="center">
  <sub>本项目为闭源私有项目，未经授权不得复制、分发或使用。保留所有权利。</sub><br>
  <sub>Copyright © 2026 IdeaBoard. All rights reserved.</sub>
</p>
