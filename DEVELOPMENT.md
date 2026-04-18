# FENG 开发文档

本文档聚焦开发信息，包含功能模块、提示词模板系统与项目结构。

## 功能模块

### 图・文生图

基础的文本/图片生成功能，支持自由创作。

### 服装实验室

| 功能 | 路由 | 说明 |
|------|------|------|
| 线稿生成 | `/line-draft` | 从服装图片生成线稿 |
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

### 图案工作室

| 功能 | 路由 | 说明 |
|------|------|------|
| 图案生成 | `/pattern-gen` | AI 生成原创图案 |
| 画风复刻 | `/style-clone` | 复刻参考图画风 |
| 四方连续 | `/seamless-tile` | 生成无缝拼接图案 |
| 图案融合 | `/pattern-fusion` | 多图案融合 |
| 工艺融合 | `/craft-fusion` | 将工艺效果融入图案 |
| 图案变清晰 | `/pattern-upscale` | 图案超分辨率（阿里云 ImageEnhance 超分 API，后端已接入） |

### AI 工具箱

| 功能 | 路由 | 说明 |
|------|------|------|
| 转矢量图 | `/to-vector` | 位图转矢量（后端调用 potrace 输出 SVG，支持参数调节） |
| 抠图褪底 | `/remove-bg` | 调用阿里云分割接口完成抠图并保存到画廊 |
| 高清放大 | `/hd-upscale` | 调用阿里云超分接口完成高清放大并保存到画廊 |
| 模糊变清晰 | `/deblur` | 规划中 |
| 画布延伸 | `/canvas-extend` | 规划中 |

### 管理后台

| 模块 | 说明 |
|------|------|
| 用户管理 | 用户列表、权限管理 |
| 账户管理 | API 账号池管理（渠道、模型、密钥、并发、启停） |
| 渠道管理 | AI 渠道配置 |
| 绘图记录 | 全平台任务记录查看 |
| 风格卡管理 | 预设风格卡的增删改查 |
| 线稿配置 | 线稿生成相关参数 |
| 功能提示词配置 | 各功能的系统提示词模板（支持 `{{变量}}` 模板语法） |
| OSS 配置 | 对象存储配置 |
| 分割配置 | 图像分割服务配置 |
| 监控 | 系统运行状态 |

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

## 项目结构

```text
FENG/
├── frontend/                    # 前端项目
│   ├── src/
│   │   ├── api/                 # API 封装
│   │   ├── assets/              # 静态资源
│   │   ├── components/          # 公共组件
│   │   ├── router/              # 路由配置
│   │   ├── stores/              # Pinia 状态管理
│   │   ├── types/               # TypeScript 类型定义
│   │   ├── utils/               # 工具函数
│   │   └── views/
│   │       ├── user/            # 用户端页面
│   │       └── admin/           # 管理后台页面
│   ├── package.json
│   └── vite.config.ts
├── backend/                     # 后端项目
│   ├── cmd/                     # 入口文件
│   │   └── main.go
│   ├── internal/
│   │   ├── config/              # 配置加载
│   │   ├── handler/             # HTTP 处理器
│   │   ├── middleware/          # 中间件
│   │   ├── model/               # 数据模型
│   │   ├── repository/          # 数据仓库层
│   │   ├── service/             # 业务逻辑层
│   │   └── worker/              # 异步任务消费者
│   └── pkg/                     # 公共包
│       ├── database/
│       ├── redis/
│       └── response/
├── docker/                      # Docker 配置
├── DEPLOYMENT.md                # 服务器部署详细步骤
├── DEVELOPMENT.md               # 开发文档（本文件）
└── README.md                    # 项目总览与快速部署
```

## 环境变量（后端）

| 变量名 | 默认值 | 说明 |
|--------|--------|------|
| `DB_HOST` | `localhost` | MySQL 主机 |
| `DB_PORT` | `3306` | MySQL 端口 |
| `DB_USER` | `root` | MySQL 用户 |
| `DB_PASSWORD` | `root123` | MySQL 密码 |
| `DB_NAME` | `feng` | 数据库名 |
| `REDIS_HOST` | `localhost` | Redis 主机 |
| `REDIS_PORT` | `6379` | Redis 端口 |
| `REDIS_AUTH` | 空 | Redis 密码 |
| `APP_PORT` | `6000` | 后端端口 |
| `JWT_SECRET` | 空 | JWT 密钥（生产环境必须设置） |

说明：后端会在启动时自动加载 `.env`（也支持通过 `ENV_FILE=/path/to/.env` 指定配置文件）。
