# 更新日志

本文档记录 FENG 项目的对外发布变更。

## v0.01 - 2026-04-10

这是当前仓库的首个正式发布版本，重点完成了安全修复、任务链路修复、文档补齐和发布产物整理。

### 新增

- 新增部署文档 `DEPLOYMENT.md`
- 新增版本文件 `VERSION`
- 新增后端安全请求组件 `backend/pkg/safehttp/`
- 新增本地发布产物目录 `release/v0.01/`
- 新增发布校验文件 `RELEASE_INFO.txt` 和 `SHA256SUMS.txt`

### 修复

- 修复 JWT Secret 硬编码问题，改为 `JWT_SECRET` 配置优先
- 修复鉴权只信任旧 Token 权限的问题，改为回源校验用户当前 `role/status`
- 修复用户任务详情存在的越权读取问题
- 修复图片下载、分割、转矢量等入口的 SSRF 风险
- 修复任务进入等待队列后无法重新调度的问题
- 修复任务创建成功但 Redis 入队失败后留下假 `pending` 的问题
- 修复停用账号后旧任务仍可能继续被分配的问题
- 修复前后端任务提交参数契约不一致的问题
- 修复前端 `401` 处理卡死和 `localStorage` 脏数据导致白屏的问题
- 修复后端错误统一返回 `HTTP 200` 导致前端和网关语义失真的问题

### 调整

- 根 `README.md` 已与当前代码结构、真实路由和启动方式对齐
- `docker/README.md` 已更新为 `docker compose` 用法
- `frontend/README.md` 已替换默认 Vite 模板说明
- 根 `.gitignore` 已补充 `.playwright-cli/`、`.idea/` 和 `release/`
- 前端 `package.json` 版本更新为 `0.0.1`

### 构建产物

当前发布产物如下：

- `feng-server-darwin-arm64-v0.01`
- `feng-server-linux-amd64-v0.01`
- `frontend-dist-v0.01.tar.gz`

### 说明

- Git Tag 使用 `v0.01`
- 前端包版本使用兼容 `SemVer` 的 `0.0.1`
