# FENG — AI 服装设计平台

一个面向服装行业的 AI 辅助设计平台，提供从线稿到成品的全链路 AI 设计工具。采用 **Vue 3 + Golang** 全栈技术架构。

---

## 技术栈

### 前端

- **Vue 3** — Composition API + `<script setup>`
- **Vite** — 构建与开发服务器
- **Naive UI** — 组件库
- **TypeScript** — 类型安全
- **Vue Router** — 路由管理
- **Pinia** — 状态管理

### 后端

- **Golang** — 高性能后端
- **Gin** — HTTP 框架
- **GORM** — ORM
- **MySQL 5.7** — 关系型数据库
- **Redis 7** — 缓存 & 任务队列

---

## 功能模块

### 图・文生图

基础的文本/图片生成功能，支持自由创作。

### 服装实验室

| 功能 | 路由 | 说明 |
|------|------|------|
| 线稿生成 | `/sketch-gen` | 从服装图片生成线稿 |
| 线稿成款 | `/sketch-to-design` | 从线稿生成服装效果图 |
| 局部改款 | `/partial-edit` | 涂抹选区 + 文字描述局部修改 |
| 款式创新 | `/style-innovate` | 基于原图进行款式变化 |
| 风格融合 | `/style-fusion` | 将参考图风格融合到原图 |
| 系列配色 | `/series-color` | 为服装生成系列配色方案 |
| 定向换色 | `/directed-recolor` | 涂抹选区 + 选择目标颜色精准换色 |
| 面料上身 | `/fabric-on-body` | 将面料贴合到服装上 |
| 图案上身 | `/pattern-on-body` | 将图案贴合到服装上 |

### 摄影商拍

| 功能 | 路由 | 说明 |
|------|------|------|
| 模特多角度 | `/multi-angle` | 生成模特多角度展示图 |
| 虚拟 3D | `/virtual-3d` | 服装 3D 虚拟展示 |
| 人台变模特 | `/mannequin-to-model` | 人台展示图转为模特上身图 |
| 定向换脸 | `/face-swap` | 模特换脸 |
| 换背景 | `/bg-swap` | 更换拍摄背景 |
| 款式上身 | `/garment-on-model` | 服装虚拟试穿 |
| 款式上身 Pro | `/garment-on-model-pro` | 高精度服装虚拟试穿 |

### 图案工作室

| 功能 | 路由 | 说明 |
|------|------|------|
| 图案生成 | `/pattern-gen` | AI 生成原创图案 |
| 画风复刻 | `/style-clone` | 复刻参考图画风 |
| 四方连续 | `/seamless-tile` | 生成无缝拼接图案 |
| 图案融合 | `/pattern-fusion` | 多图案融合 |
| 工艺融合 | `/craft-fusion` | 将工艺效果融入图案 |
| 图案涂改 | `/pattern-inpaint` | 局部修改图案 |
| 图案变清晰 | `/pattern-upscale` | 图案超分辨率（阿里云 ImageEnhance 超分 API，后端已接入） |

### AI 工具箱

| 功能 | 路由 | 说明 |
|------|------|------|
| 转矢量图 | `/to-vector` | 位图转矢量（后端调用 potrace 输出 SVG，支持参数调节） |
| 抠图褪底 | `/remove-bg` | 智能抠图去背景 |
| 高清放大 | `/hd-upscale` | 图片超分辨率放大 |
| 模糊变清晰 | `/deblur` | 图片去模糊 |
| 画布延伸 | `/canvas-extend` | AI 扩展画布 |

### 管理后台

| 模块 | 说明 |
|------|------|
| 用户管理 | 用户列表、权限管理 |
| 账户管理 | 充值、消费记录 |
| 渠道管理 | AI 渠道配置 |
| 绘图记录 | 全平台任务记录查看 |
| 风格卡管理 | 预设风格卡的增删改查 |
| 线稿配置 | 线稿生成相关参数 |
| 功能提示词配置 | 各功能的系统提示词模板（支持 `{{变量}}` 模板语法） |
| OSS 配置 | 对象存储配置 |
| 分割配置 | 图像分割服务配置 |
| 监控 | 系统运行状态 |

---

## 提示词模板系统

管理后台支持为每个功能配置系统提示词，使用 `{{变量}}` 语法实现动态替换。

| 变量 | 适用功能 | 说明 |
|------|----------|------|
| `{{user_prompt}}` | 通用 | 用户输入的描述文本 |
| `{{strength}}` | 款式创新 | 改变幅度（1-10） |
| `{{category}}` | 风格融合、系列配色 | 用户选择的品类 |
| `{{color_type}}` | 系列配色 | 配色类型（women/men/kids/reference） |
| `{{colors}}` | 系列配色 | 颜色列表（如 `#8b3a3a, #6b2d3e`） |
| `{{target_color}}` | 定向换色 | 目标颜色（如 `#ff5500`） |

---

## 项目结构

```
FENG/
├── frontend/                    # 前端项目
│   ├── src/
│   │   ├── api/                 # API 封装
│   │   ├── assets/              # 静态资源
│   │   ├── components/          # 公共组件（ImageUploader 等）
│   │   ├── router/              # 路由配置
│   │   ├── stores/              # Pinia 状态管理
│   │   ├── types/               # TypeScript 类型定义
│   │   ├── utils/               # 工具函数
│   │   └── views/
│   │       ├── user/            # 用户端页面
│   │       │   ├── Home.vue              # 图・文生图
│   │       │   ├── LineDraft.vue          # 线稿生成
│   │       │   ├── SketchToDesign.vue     # 线稿成款
│   │       │   ├── PartialEdit.vue        # 局部改款
│   │       │   ├── StyleInnovate.vue      # 款式创新
│   │       │   ├── StyleFusion.vue        # 风格融合
│   │       │   ├── SeriesColor.vue        # 系列配色
│   │       │   ├── DirectedRecolor.vue    # 定向换色
│   │       │   ├── FeaturePage.vue        # 通用功能页
│   │       │   └── Layout.vue             # 用户端布局
│   │       └── admin/           # 管理后台页面
│   │           ├── Users.vue              # 用户管理
│   │           ├── Accounts.vue           # 账户管理
│   │           ├── Channels.vue           # 渠道管理
│   │           ├── DrawingRecords.vue     # 绘图记录
│   │           ├── StyleCards.vue         # 风格卡管理
│   │           ├── FeaturePromptConfig.vue # 功能提示词配置
│   │           ├── OSSConfig.vue          # OSS 配置
│   │           ├── SegConfig.vue          # 分割配置
│   │           ├── Monitor.vue            # 监控
│   │           └── Layout.vue             # 后台布局
│   ├── package.json
│   └── vite.config.ts
├── backend/                     # 后端项目
│   ├── cmd/                     # 入口文件
│   │   └── main.go
│   ├── internal/
│   │   ├── config/              # 配置加载
│   │   ├── handler/             # HTTP 处理器
│   │   │   ├── account_handler.go
│   │   │   ├── admin_handler.go
│   │   │   ├── auth_handler.go
│   │   │   ├── channel_handler.go
│   │   │   ├── feature_prompt_handler.go
│   │   │   ├── line_draft_handler.go
│   │   │   ├── oss_handler.go
│   │   │   ├── seg_handler.go
│   │   │   ├── vectorize_handler.go     # 转矢量图（potrace）
│   │   │   └── style_card_handler.go
│   │   ├── middleware/          # 中间件（鉴权等）
│   │   ├── model/               # 数据模型
│   │   ├── repository/          # 数据仓库层
│   │   ├── service/             # 业务逻辑层
│   │   └── worker/              # 异步任务消费者
│   │       └── consumer.go
│   └── pkg/                     # 公共包
│       ├── database/
│       ├── redis/
│       └── response/
├── docker/                      # Docker 配置
│   ├── docker-compose.yml
│   └── mysql/
└── README.md
```

---

## 快速开始

### 1. 启动数据库服务

```bash
cd docker
docker compose up -d
```

默认会启动：

- **MySQL 5.7**（root/root123，初始化库 `feng`）
- **Redis 7**

说明：Apple Silicon（M 系列）机器已在 `docker/docker-compose.yml` 中兼容 `mysql:5.7` 的 `linux/amd64` 平台。

### 2. 启动后端服务

```bash
cd backend
go run cmd/main.go
```

后端服务：http://localhost:8080

健康检查：http://localhost:8080/api/health

### 3. 启动前端服务

```bash
cd frontend
npm install
npm run dev
```

前端服务：http://localhost:5173

### 4. 初始化管理员账号（可选）

```bash
cd backend
go run cmd/main.go init-admin
```

---

## 环境变量

### 后端

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| DB_HOST | localhost | MySQL 主机 |
| DB_PORT | 3306 | MySQL 端口 |
| DB_USER | root | MySQL 用户名 |
| DB_PASSWORD | root123 | MySQL 密码 |
| DB_NAME | feng | 数据库名 |
| REDIS_HOST | localhost | Redis 主机 |
| REDIS_PORT | 6379 | Redis 端口 |
| REDIS_AUTH | | Redis 密码 |
| APP_PORT | 8080 | 服务端口 |
| JWT_SECRET | | JWT 签名密钥（建议显式设置；未设置时本地开发会生成一次性密钥） |

说明：后端使用环境变量加载配置（`backend/internal/config/config.go`），未设置时会使用上述默认值。

### 前端

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| VITE_API_BASE_URL | /api | API 基础路径 |

说明：当前开发环境也可以直接通过 `vite.config.ts` 的 proxy 将 `/api` 转发到后端。

---

## 开发指南

### 前端

```bash
npm install          # 安装依赖
npm run dev          # 启动开发服务器
npm run build        # 构建生产版本
npm run preview      # 本地预览生产构建
```

### 后端

```bash
go mod tidy                      # 下载依赖
go run cmd/main.go               # 启动服务
go build -o server cmd/main.go   # 构建
```

初始化管理员账号（默认用户名 `admin`，密码 `admin123`）：

```bash
go run cmd/main.go init-admin
```

---

## 核心交互流程

```
用户上传图片 → 前端组件处理（涂抹/选色/描述）→ 提交任务（taskApi.submit）
    → 后端 SubmitTask（模板变量替换 + 任务入队）→ Worker 消费（调用 AI API）
    → 前端轮询任务状态 → 展示生成结果
```

补充：

- **转矢量图**：前端调用 `POST /api/user/vectorize`，后端直接同步返回 `svg_url`（不走异步队列）。
- **图案变清晰**：前端调用 `POST /api/user/upscale`，后端调用阿里云超分后将结果持久化并返回。

---

## 关键 API（节选）

- `POST /api/user/upload` 上传参考图（返回 `url`）
- `POST /api/user/vectorize` 转矢量图（返回 `svg_url`、`task_id`）
- `GET  /api/user/vectorize/check` 检测 potrace 是否可用
- `POST /api/user/upscale` 图案超分（返回 `image_url`、`task_id`）

---

## 依赖与运行要求

### 1) MySQL + Redis

- 开发环境推荐用 `docker/docker-compose.yml` 启动（命令使用 `docker compose up -d`）
- 生产环境（宝塔）可以用宝塔的 MySQL/Redis 服务（注意与环境变量一致）

### 2) potrace（转矢量图）

`/to-vector` 功能依赖 **potrace**。

- Linux 常用安装：`apt install potrace` 或 `yum install potrace`
- Windows 本地开发：下载 potrace 并加入 PATH

后端会优先尝试这些路径：

- `C:\Users\qingf\Desktop\下载\potrace-1.16.win64\potrace-1.16.win64\potrace.exe`
- `/usr/bin/potrace`
- `/usr/local/bin/potrace`

找不到则回退到 `PATH` 中的 `potrace`。

---

## 生产构建（简版）

### 后端（Linux 可执行文件）

在本机交叉编译（示例：linux/amd64，静态编译）：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o feng-server ./cmd
```

### 前端

```bash
cd frontend
npm run build
```

构建产物在 `frontend/dist/`。

---

## 常见问题

### 1) `potrace: command not found`

- 确认已安装 potrace
- 如果刚修改 PATH，需要重启终端/服务
- 可访问 `GET /api/user/vectorize/check` 验证

### 2) 上传图片后无法访问 `/uploads/...`

- 后端会将 `uploads/` 作为静态目录：`/uploads` → `./uploads`
- 生产部署时注意进程工作目录（确保 `uploads/` 在同级目录）

---

## 许可证

本项目为闭源私有项目，未经授权不得复制、分发或使用。All Rights Reserved.
