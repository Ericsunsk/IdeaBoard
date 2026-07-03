# IdeaBoard

<p align="center">
  <img src="./docs/assets/ideaboard-wordmark.png" alt="IdeaBoard" width="461">
</p>

<p align="center">
  <strong>为图像设计与电商展示场景构建的 AI 画布工作流平台。</strong>
</p>

<p align="center">
  在统一画布中完成素材输入、节点编排、任务执行、结果沉淀与后台配置。
</p>

<p align="center">
  <img src="https://img.shields.io/badge/repository-private-4b5563?style=flat-square" alt="仓库：私有" />
  <img src="https://img.shields.io/badge/source-closed-374151?style=flat-square" alt="源码：闭源" />
  <img src="https://img.shields.io/badge/distribution-internal_only-1f2937?style=flat-square" alt="分发：仅限内部" />
</p>

## 产品概览

IdeaBoard 是一套围绕 AI 图像设计工作流打造的全栈系统。前端提供 Canvas Workspace、画板管理和管理后台；后端负责鉴权、任务提交、提示词装配、账号池调度、异步执行、结果持久化、统计聚合和对象存储接入。

<p align="center">
  <img src="./docs/assets/ideaboard-preview.png" alt="IdeaBoard 画布工作流预览" width="1400">
</p>

当前主链路：

```text
Canvas 节点编排 -> Workflow feature contract -> Task 提交 -> River Worker 执行 -> Storage 持久化 -> SSE 结果回写
```

## 能力范围

| 领域 | 当前能力 |
| ---- | -------- |
| 画布工作流 | Canvas Board、节点编排、连接、执行、结果回写 |
| 图像生成/编辑 | 草图成图、风格延展、局部重绘、抠图、多图融合、图案生成、HD、扩图 |
| 任务运行时 | PostgreSQL/River 队列、账号池调度、失败重试、SSE 状态推送 |
| 管理后台 | 用户、账号、渠道、提示词、存储、任务监控、统计仪表盘 |
| 存储 | Local / S3-compatible object storage |
| Auth | Better Auth 登录、注册、session、角色、封禁、受控注销 |

## 技术栈

| 层 | 技术 |
| -- | ---- |
| Web | Vue 3, Vite, TypeScript, Tailwind CSS v4, PrimeVue v4 unstyled, Pinia, Vue Flow |
| API | Go, Gin, sqlc, River |
| Auth | Better Auth, Fastify, TypeScript |
| Data | PostgreSQL 17 |
| Events | NATS JetStream |
| Deploy | Cloudflare Pages, VPS reverse proxy, Docker Compose, GHCR |

默认本地端口：

| 服务 | 端口 |
| ---- | ---- |
| Web | `5173` |
| Go API | `6000` |
| Auth Service | `6001` |
| NATS | `4222` / `8222` |

## 快速开始

环境要求：

| 组件 | 版本 |
| ---- | ---- |
| Node.js | `22.18+` |
| Go | `1.26.4+` |
| PostgreSQL | `17+` |

1. 启动本地依赖：

```bash
cd docker
cp .env.example .env
# 至少修改 POSTGRES_PASSWORD
docker compose -f compose.local.yml up -d
```

2. 启动 Auth：

```bash
cd apps/auth
cp .env.example .env
npm install
npm run migrate
npm run dev
```

3. 启动 Go API：

```bash
cd apps/api
cp .env.example .env
go run ./cmd migrate
go run ./cmd
```

4. 启动 Web：

```bash
cd apps/web
npm install
npm run dev
```

5. 初始化管理员：

```bash
cd apps/api
INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin
```

访问地址：

| 地址 | 用途 |
| ---- | ---- |
| `http://localhost:5173` | Web |
| `http://localhost:5173/admin/login` | Admin login |
| `http://localhost:6000/api/health` | Go API health |
| `http://localhost:6001/api/health` | Auth health |

更完整的本地开发、验证和代码入口见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

## 仓库结构

```text
IdeaBoard/
├── apps/
│   ├── web/                  # Vue SPA、Canvas Workspace、Admin UI
│   ├── api/                  # Go API、Tasking、Worker、domain contexts
│   └── auth/                 # Better Auth service
├── docker/                   # 本地依赖与生产 Docker Compose
├── deploy/                   # Reverse proxy templates
├── docs/
│   ├── agents/               # Agent workflow docs
│   ├── assets/               # README assets
│   └── runbooks/             # 部署、运营、安全、数据保护手册
├── DESIGN.md                 # System design contract
├── DEVELOPMENT.md            # Developer handbook
└── AGENTS.md                 # Agent/coding rules
```

## 文档入口

| 文档 | 用途 |
| ---- | ---- |
| [DESIGN.md](./DESIGN.md) | 系统设计、模块边界、关键 invariants |
| [DEVELOPMENT.md](./DEVELOPMENT.md) | 本地开发、代码入口、验证矩阵 |
| [apps/web/DESIGN.md](./apps/web/DESIGN.md) | Agent 做前端 UI 改动时的执行规则 |
| [docs/runbooks/deployment.md](./docs/runbooks/deployment.md) | Cloudflare + Docker Compose 部署、升级、回滚 |
| [docs/runbooks/auth-operations.md](./docs/runbooks/auth-operations.md) | Auth 运营与高风险身份操作边界 |
| [docs/runbooks/data-protection.md](./docs/runbooks/data-protection.md) | PostgreSQL backup/restore 与密钥材料处理 |
| [docs/runbooks/system-prompts.md](./docs/runbooks/system-prompts.md) | Feature System Prompt 配置与评审 |
| [apps/api/CONTEXT.md](./apps/api/CONTEXT.md) | Backend context map 与 dependency direction |
| [apps/web/src/features/canvas/CONTEXT.md](./apps/web/src/features/canvas/CONTEXT.md) | Canvas domain language |
| [docker/README.md](./docker/README.md) | 本地 PostgreSQL/NATS 与 Docker stack 说明 |

## 验证建议

日常最小验证：

```bash
cd apps/web && npm run test:run
cd apps/api && go test ./...
```

跨层或上线前验证见 [DEVELOPMENT.md](./DEVELOPMENT.md#验证矩阵) 和 [docs/runbooks/deployment.md](./docs/runbooks/deployment.md)。

<p align="center">
  <sub>本项目为闭源私有项目，未经授权不得复制、分发或使用。保留所有权利。</sub>
</p>
