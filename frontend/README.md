# FENG Frontend

这是 FENG 的前端项目，基于 `Vue 3 + Vite + TypeScript + Naive UI`。

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

- 当前 `axios` 的 `baseURL` 固定为 `/api`
- Dev 环境通过 `vite.config.ts` 将 `/api` 和 `/uploads` 代理到 `http://localhost:8080`
- 如果需要切换本地后端地址，请修改 `vite.config.ts`

## 主要目录

```text
frontend/
├── src/api/        # API 封装
├── src/router/     # 路由与守卫
├── src/stores/     # Pinia store
├── src/views/      # 用户端 / 管理后台页面
├── src/components/ # 通用组件
└── vite.config.ts  # Dev proxy
```

## 相关文档

- 根目录 `README.md`：项目总体说明
- `../docker/README.md`：本地 MySQL / Redis 启动说明
