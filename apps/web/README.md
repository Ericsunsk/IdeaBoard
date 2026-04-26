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

## 开发说明

- 当前业务 API 客户端的 `baseURL` 固定为 `/api`
- 开发环境通过 `vite.config.ts` 将 `/api/auth` 代理到 `http://localhost:6001`
- 开发环境通过 `vite.config.ts` 将其余 `/api` 和 `/uploads` 代理到 `http://localhost:6000`
- 如果需要切换本地后端地址，请修改 `vite.config.ts`

## 主要目录

```text
apps/web/
├── src/app/        # 入口、router、store 装配
├── src/pages/      # 路由级页面容器
├── src/features/   # 领域功能模块
├── src/shared/     # 共享 API、UI、types、utils、assets
└── vite.config.ts  # 开发代理
```

## 相关文档

- [根目录 README.md](../../README.md)：项目总体说明
- [docker/README.md](../../docker/README.md)：本地 MySQL / Redis 启动说明
