# IdeaBoard 前端项目

这是 IdeaBoard 的前端项目，基于 `Vue 3 + Vite + TypeScript + Naive UI`，包含用户画布、用户中心与管理后台。

## 启动

```bash
npm install
npm run dev
```

默认地址：`http://localhost:5173`

## 构建

```bash
npm run build
```

构建产物输出到 `dist/`。

## Cloudflare Pages 部署

用于“前端在 Cloudflare Pages，后端与数据库在 VPS，并通过 Cloudflare Tunnel 暴露 API 域名”的部署方式。

Pages 构建配置：

```text
Root directory: apps/web
Build command: npm run build
Build output directory: dist
Environment variable: NODE_VERSION=22.18.0
Environment variable: VITE_API_BASE_URL=https://api.example.com/api
Environment variable: VITE_AUTH_BASE_URL=https://api.example.com
```

可选素材域名变量：

```text
VITE_PUBLIC_ASSET_BASE_URL=https://assets.example.com
```

如果不设置 `VITE_PUBLIC_ASSET_BASE_URL`，前端会从 `VITE_API_BASE_URL` 自动派生素材域名，例如 `https://api.example.com/uploads/...`。

后端生产环境需要同步配置：

```text
APP_BASE_URL=https://你的前端域名
API_BASE_URL=https://api.example.com
```

## 开发说明

- 当前业务 API 客户端默认 `baseURL` 为 `/api`，生产可通过 `VITE_API_BASE_URL` 改为独立 API 域名
- 开发环境通过 `vite.config.ts` 将 `/api/auth` 代理到 `http://localhost:6001`
- 开发环境通过 `vite.config.ts` 将其余 `/api` 和 `/uploads` 代理到 `http://localhost:6000`
- 如果需要切换本地后端地址，请修改 `vite.config.ts` 或设置对应 `VITE_*` 环境变量

## 主要目录

```text
apps/web/
├── src/app/                  # 入口、router、store 装配
├── src/pages/                # 路由级页面容器
├── src/features/             # 领域功能模块
│   ├── user/                 # 用户侧 layout 与壳层
│   └── workspace/            # 画布、节点与执行链路
├── src/shared/               # 共享 API、auth、types、utils、assets
│   └── api/generated-query/  # Orval 生成代码，不手动编辑
└── vite.config.ts            # 开发代理
```

## 相关文档

- [根目录 README.md](../../README.md)：项目总体说明
- [docker/README.md](../../docker/README.md)：本地 MySQL / Redis 启动说明
