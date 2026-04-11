# FENG

> 面向服装行业的 AI 设计与商拍平台，覆盖从线稿、改款、图案生成到模特展示的核心工作流。

FENG 采用 `Vue 3 + Go` 全栈架构，前端负责交互与工作台体验，后端统一提供 API、任务处理、账户调度、图片上传和管理后台能力。项目同时支持传统 Linux 部署，以及适合宝塔面板的 Go 单体部署方案。

## 项目概览

| 维度 | 说明 |
|------|------|
| 产品定位 | AI 服装设计平台 |
| 前端技术 | `Vue 3`、`Vite`、`TypeScript`、`Naive UI`、`Pinia` |
| 后端技术 | `Go`、`Gin`、`GORM` |
| 基础设施 | `MySQL 5.7+`、`Redis 7+` |
| 默认端口 | `6000` |
| 部署方式 | 标准 Linux 部署、宝塔 Go 单体部署 |

## 核心能力

| 模块 | 代表能力 |
|------|----------|
| 服装实验室 | 线稿生成、线稿成款、局部改款、款式创新、风格融合、系列配色、定向换色 |
| 摄影商拍 | 模特多角度、虚拟 3D、人台变模特、换脸、换背景、款式上身 |
| 图案工作室 | 图案生成、画风复刻、四方连续、图案融合、工艺融合、图案涂改、图案超分 |
| AI 工具箱 | 转矢量图、抠图褪底、高清放大 |
| 管理后台 | 用户管理、账号池管理、渠道配置、任务记录、提示词模板、OSS 与分割配置 |

更完整的功能清单、路由和项目结构见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

## 文档导航

| 文档 | 用途 |
|------|------|
| [README.md](./README.md) | 项目总览、快速开始、部署入口 |
| [DEVELOPMENT.md](./DEVELOPMENT.md) | 开发文档、功能模块、提示词模板、目录结构 |
| [DEPLOYMENT.md](./DEPLOYMENT.md) | 标准 Linux 生产部署流程 |
| [DEPLOY_BAOTA_GO.md](./DEPLOY_BAOTA_GO.md) | 宝塔 Go 项目部署说明 |

## 快速开始

### 环境要求

| 组件 | 建议版本 |
|------|----------|
| Node.js | `18+` |
| Go | `1.22+` |
| MySQL | `5.7+` |
| Redis | `7+` |

### 1. 启动基础服务

推荐直接使用仓库内 Docker 配置启动 MySQL 和 Redis：

```bash
cd docker
docker compose up -d
```

### 2. 配置后端环境变量

```bash
cd backend
cp .env.example .env
```

至少确认这些字段：

- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PASSWORD`
- `DB_NAME`
- `REDIS_HOST`
- `REDIS_PORT`
- `REDIS_AUTH`
- `APP_PORT`
- `JWT_SECRET`

### 3. 启动后端

```bash
cd backend
go run cmd/main.go
```

### 4. 启动前端

```bash
cd frontend
npm install
npm run dev
```

### 5. 初始化管理员账号

```bash
cd backend
go run cmd/main.go init-admin
```

执行 `init-admin` 后会创建或重置管理员账号：

- 用户名：`admin`
- 密码：`admin123`

首次登录后应立即修改密码。

## 本地开发访问

| 地址 | 说明 |
|------|------|
| `http://localhost:5173` | 前端开发环境 |
| `http://localhost:6000/api/health` | 后端健康检查 |

前端开发代理已指向 `http://localhost:6000`，相关配置见 [frontend/vite.config.ts](./frontend/vite.config.ts)。

## 部署方式

### 方式一：宝塔 Go 单体部署

适合希望直接在宝塔中以 Go 项目方式运行的场景。当前方案会将前端静态资源内嵌到 Go 二进制中，减少部署链路。

打包命令：

```bash
./scripts/package-bt-go.sh
```

说明文档见 [DEPLOY_BAOTA_GO.md](./DEPLOY_BAOTA_GO.md)。

### 方式二：标准 Linux 部署

适合使用 `systemd + Nginx` 的常规生产环境。

打包命令：

```bash
./scripts/package-deploy.sh
```

说明文档见 [DEPLOYMENT.md](./DEPLOYMENT.md)。

## 推荐验证项

部署完成后，至少检查以下内容：

1. `curl http://127.0.0.1:6000/api/health` 返回成功。
2. 首页可以正常打开，且没有重定向循环。
3. 管理员账号可以登录。
4. 图片上传与 `uploads` 访问正常。
5. 关键功能页能正常提交任务。

## 项目结构

```text
FENG/
├── frontend/                 # Vue 3 前端
├── backend/                  # Go 后端
├── docker/                   # 本地依赖服务
├── scripts/                  # 打包与辅助脚本
├── DEVELOPMENT.md            # 开发文档
├── DEPLOYMENT.md             # 标准部署文档
├── DEPLOY_BAOTA_GO.md        # 宝塔部署文档
└── README.md                 # 仓库入口文档
```

## 许可证

本项目为闭源私有项目，未经授权不得复制、分发或使用。

`All Rights Reserved.`
