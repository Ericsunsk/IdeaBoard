# FENG — AI 服装设计平台

FENG 是一个面向服装行业的 Web 应用，提供从线稿、款式创新到图案处理的 AI 辅助设计能力。当前仓库采用 `Vue 3 + Vite + TypeScript` 前端，以及 `Golang + Gin + GORM + Redis` 后端。

## 技术栈

### 前端

- `Vue 3`
- `Vite`
- `TypeScript`
- `Naive UI`
- `Vue Router`
- `Pinia`

### 后端

- `Golang`
- `Gin`
- `GORM`
- `MySQL 5.7`
- `Redis 7`

## 功能与路由

用户端页面统一挂载在 `/user/*`，管理后台挂载在 `/admin/*`。

### 用户端

| 模块 | 路由 | 说明 |
|------|------|------|
| 首页 | `/user/home` | 图文生图入口 |
| 用户中心 | `/user/center` | 账号信息、密码修改、任务查看 |
| 线稿生成 | `/user/line-draft` | 从服装图生成线稿 |
| 线稿成款 | `/user/sketch-to-design` | 从线稿生成效果图 |
| 局部改款 | `/user/partial-edit` | 局部编辑款式 |
| 款式创新 | `/user/style-innovate` | 基于原图做款式变化 |
| 风格融合 | `/user/style-fusion` | 融合参考风格 |
| 系列配色 | `/user/series-color` | 批量生成配色方案 |
| 定向换色 | `/user/directed-recolor` | 精准换色 |
| 面料上身 | `/user/fabric-on-body` | 面料贴合展示 |
| 图案上身 | `/user/pattern-on-body` | 图案贴合展示 |
| 模特多角度 | `/user/multi-angle` | 生成模特多角度图 |
| 虚拟 3D | `/user/virtual-3d` | 虚拟展示 |
| 人台变模特 | `/user/mannequin-to-model` | 人台图转模特图 |
| 定向换脸 | `/user/face-swap` | 模特换脸 |
| 换背景 | `/user/bg-swap` | 更换背景 |
| 款式上身 | `/user/garment-on-model` | 虚拟试穿 |
| 图案生成 | `/user/pattern-gen` | 生成原创图案 |
| 画风复刻 | `/user/style-clone` | 复刻参考画风 |
| 四方连续 | `/user/seamless-tile` | 无缝拼接图案 |
| 图案融合 | `/user/pattern-fusion` | 多图案融合 |
| 工艺融合 | `/user/craft-fusion` | 工艺效果融合 |
| 图案涂改 | `/user/pattern-inpaint` | 局部修改图案 |
| 图案变清晰 | `/user/pattern-upscale` | 图案超分 |
| 转矢量图 | `/user/to-vector` | 位图转 SVG |
| 抠图褪底 | `/user/remove-bg` | 抠图去背景 |
| 高清放大 | `/user/hd-upscale` | 图片超分辨率 |
| 模糊变清晰 | `/user/deblur` | 去模糊 |
| 画布延伸 | `/user/canvas-extend` | 扩展画布 |

### 管理后台

| 模块 | 路由 |
|------|------|
| 登录 | `/admin/login` |
| 用户管理 | `/admin/users` |
| 账号管理 | `/admin/accounts` |
| 监控 | `/admin/monitor` |
| 风格卡管理 | `/admin/style-cards` |
| 绘图记录 | `/admin/drawing-records` |
| 渠道管理 | `/admin/channels` |
| OSS 配置 | `/admin/oss-config` |
| 分割配置 | `/admin/seg-config` |
| 线稿配置 | `/admin/line-draft-config` |
| 人脸库 | `/admin/face-library` |
| 背景库 | `/admin/bg-library` |
| 模特库 | `/admin/model-library` |
| 姿势库 | `/admin/pose-library` |
| 图案风格库 | `/admin/pattern-style-library` |
| 功能提示词配置 | `/admin/feature-prompts` |

## 项目结构

```text
Feng-AI-main/
├── frontend/           # Vue 3 前端
│   ├── src/api/        # API 封装
│   ├── src/router/     # 路由配置
│   ├── src/stores/     # Pinia store
│   ├── src/views/      # 用户端与管理后台页面
│   └── vite.config.ts  # Dev proxy 配置
├── backend/            # Gin 后端
│   ├── cmd/main.go     # 入口与初始化命令
│   ├── internal/       # handler / service / repository / middleware
│   └── pkg/            # response / redis / safehttp 等公共包
├── docker/             # MySQL / Redis 本地开发环境
└── README.md
```

## 快速开始

### 1. 启动 MySQL 和 Redis

```bash
cd docker
docker compose up -d
```

默认会启动：

- `MySQL 5.7`，端口 `3306`
- `Redis 7`，端口 `6379`

说明：

- `docker/docker-compose.yml` 已为 Apple Silicon 的 `mysql:5.7` 增加 `platform: linux/amd64`
- 当前默认账号与密码仅适合本地开发或内部调试

### 2. 启动后端

```bash
cd backend
go run cmd/main.go
```

默认地址：`http://localhost:8080`

健康检查：

```bash
curl http://localhost:8080/api/health
```

### 3. 启动前端

```bash
cd frontend
npm install
npm run dev
```

默认地址：`http://localhost:5173`

### 4. 初始化管理员账号

```bash
cd backend
go run cmd/main.go init-admin
```

默认管理员账号：

- 用户名：`admin`
- 密码：`admin123`

## 环境变量

### 后端

后端从 `backend/internal/config/config.go` 和鉴权中间件读取配置。

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DB_HOST` | `localhost` | MySQL Host |
| `DB_PORT` | `3306` | MySQL Port |
| `DB_USER` | `root` | MySQL User |
| `DB_PASSWORD` | `root123` | MySQL Password |
| `DB_NAME` | `feng` | Database Name |
| `REDIS_HOST` | `localhost` | Redis Host |
| `REDIS_PORT` | `6379` | Redis Port |
| `REDIS_AUTH` | 空 | Redis Password |
| `APP_PORT` | `8080` | Backend Port |
| `JWT_SECRET` | 无默认固定值 | JWT 签名密钥；未设置时后端会生成一次性开发密钥，服务重启后旧 token 会失效 |

### 前端

当前前端没有启用 `VITE_API_BASE_URL` 这类运行时环境变量。

- Dev 环境通过 `frontend/vite.config.ts` 代理 `/api` 和 `/uploads` 到 `http://localhost:8080`
- 如果需要切换本地后端地址，请直接修改 `vite.config.ts` 中的 `server.proxy`

## 开发命令

### 前端

```bash
cd frontend
npm install
npm run dev
npm run build
```

### 后端

```bash
cd backend
go mod tidy
go run cmd/main.go
go build ./...
```

## 核心交互流程

```text
用户上传图片
  -> 前端提交任务参数
  -> 后端创建任务并写入 Redis 队列
  -> Worker 消费任务并调用外部 AI 渠道
  -> 前端轮询任务状态
  -> 展示结果或失败信息
```

补充：

- `POST /api/user/vectorize` 为同步接口，直接返回 `svg_url`
- `POST /api/user/upscale` 为同步增强接口，处理完成后直接返回结果地址

## 关键 API

- `GET /api/health`：服务健康检查
- `POST /api/auth/login`：用户登录
- `POST /api/auth/register`：用户注册
- `POST /api/user/upload`：上传参考图
- `GET /api/user/info`：获取当前用户信息
- `POST /api/user/vectorize`：转矢量图
- `GET /api/user/vectorize/check`：检查 `potrace` 是否可用
- `POST /api/user/upscale`：图案超分

## 外部依赖

### potrace

`/user/to-vector` 依赖 `potrace`。

- macOS：`brew install potrace`
- Ubuntu / Debian：`apt install potrace`
- CentOS：`yum install potrace`

后端会优先尝试常见系统路径，找不到时回退到 `PATH` 中的 `potrace`。

## 相关文档

- `docker/README.md`：本地 Docker 环境说明
- `frontend/README.md`：前端开发说明

## 许可证

本项目为闭源私有项目，未经授权不得复制、分发或使用。All Rights Reserved.
