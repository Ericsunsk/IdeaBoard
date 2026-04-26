# 更新日志

本文档记录 IdeaBoard 的正式发布变更。

## v0.1.0 - 2026-04-26

首个发布版基线，提供完整的 AI 画布工作流、后台运营配置、身份认证与标准 Linux 部署能力。

### 核心能力

- 提供用户画布工作台、画板管理、用户中心与管理后台。
- 支持线稿成款、款式创新、局部改款、换背景、换脸、双图融合、多色配色、定向换色、面料上身、单视角展示、商品展示、图案生成、高清增强与画布延伸。
- 支持任务落库、Redis 队列、Worker 异步执行、账号池调度、状态轮询与结果持久化。
- 支持 `Local / Kodo / S3-compatible` 存储配置。

### 认证与后台

- 登录、注册、登出与 session 校验由 Better Auth Service 接管。
- Go API 负责业务授权、用户档案、角色/status 判定与高风险身份操作编排。
- 管理后台支持用户封禁/解封、会话吊销、伪装登录/停止伪装、角色升降级、受控彻底注销、开放注册开关与防刷限流设置。
- 后台系统提示词按 workflow feature 配置，并使用当前 canonical contract。

### 部署与文档

- 标准部署形态为 `Nginx + systemd + Go API + Better Auth Node Service + MySQL + Redis`。
- 发布包由 `scripts/package-deploy.sh` 生成，包含前端静态产物、Go API 二进制、Auth Service 构建产物、systemd 与 Nginx 模板。
- 文档覆盖项目入口、开发 contract、系统提示词配置、Auth 运维边界、本地依赖与 Linux 部署流程。
