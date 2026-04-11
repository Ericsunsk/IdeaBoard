# FENG — AI 服装设计平台

面向服装行业的 AI 辅助设计平台，覆盖从线稿到商拍的核心流程，采用 `Vue 3 + Go` 全栈架构。

## 文档导航

- 开发文档（功能模块、提示词模板系统、项目结构）：`DEVELOPMENT.md`
- 服务器部署全量步骤（生产详细版）：`DEPLOYMENT.md`

## 技术栈

- 前端：`Vue 3`、`Vite`、`TypeScript`、`Naive UI`、`Pinia`
- 后端：`Go`、`Gin`、`GORM`
- 基础设施：`MySQL 5.7+`、`Redis 7+`

## 本地开发快速开始

1. 启动 MySQL 与 Redis（推荐使用仓库内 Docker）：

```bash
cd docker
docker compose up -d
```

2. 配置后端环境变量：

```bash
cd backend
cp .env.example .env
```

3. 启动后端：

```bash
cd backend
go run cmd/main.go
```

4. 启动前端：

```bash
cd frontend
npm install
npm run dev
```

5. （可选）初始化管理员账号：

```bash
cd backend
go run cmd/main.go init-admin
```

默认管理员：`admin / admin123`（首次登录后请立即修改）。

## 生产部署步骤

以下为最短可落地流程；完整细节与排障请看 `DEPLOYMENT.md`。

1. 准备服务器（`Linux amd64`），安装并启动 `nginx`、`mysql`、`redis`，如需转矢量功能额外安装 `potrace`。
2. 从 GitHub Release 下载发布包并校验：

```bash
TAG=v0.1.0
BUNDLE="feng-bundle-${TAG}-linux-amd64.tar.gz"
curl -L -o "$BUNDLE" \
  "https://github.com/2837123/Feng-AI/releases/download/${TAG}/${BUNDLE}"
sha256sum "$BUNDLE"
```

3. 解压到 `/opt/feng`（最终目录建议包含 `backend/`、`frontend/`、`deploy/`、`logs/`）。
4. 配置后端环境变量：

```bash
cp /opt/feng/backend/.env.example /opt/feng/backend/.env
```

最少需确认：`DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`、`REDIS_HOST/REDIS_PORT/REDIS_AUTH`、`APP_PORT`、`JWT_SECRET`。

5. 初始化管理员并启动服务：
   - 后端：使用 `/opt/feng/deploy/systemd/feng.service` 注册 `systemd`，并 `systemctl enable --now feng`
   - 前端：使用 `/opt/feng/deploy/nginx/feng.conf` 配置 `nginx` 并 `systemctl reload nginx`
6. 验证上线：

```bash
curl http://127.0.0.1:8080/api/health
curl -I http://127.0.0.1/
```

## 环境变量

后端环境变量已统一维护在 `DEVELOPMENT.md` 的“环境变量（后端）”章节，避免多处重复。

## 许可证

本项目为闭源私有项目，未经授权不得复制、分发或使用。All Rights Reserved.
