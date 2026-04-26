# IdeaBoard 宝塔 Go 部署手册（已停用）

> [!IMPORTANT]
> Better Auth 迁移后，登录/注册认证由独立的 `apps/auth` Node Service 接管。旧的“前后端资源内嵌进单个 Go 二进制”的宝塔 Go 项目模式无法承载 `/api/auth/*` 与 Better Auth session，因此本部署方式已停用。

## 推荐方式

请使用标准 Linux 部署包：

```bash
./scripts/package-deploy.sh
```

部署步骤见 [DEPLOYMENT.md](./DEPLOYMENT.md)。新拓扑包含：

- `ideaboard`：Go API，默认监听 `127.0.0.1:6000`
- `ideaboard-auth`：Better Auth Node Service，默认监听 `127.0.0.1:6001`
- `Nginx`：转发 `/api/auth/*` 到认证服务，转发 `/api/*` 与 `/uploads/*` 到 Go API

## 如果必须使用宝塔

可以继续用宝塔管理 Nginx 与进程，但需要分别创建两个项目或守护进程：

- Go 项目：启动 `/www/wwwroot/ideaboard/apps/api/ideaboard-server`
- Node 项目：在 `/www/wwwroot/ideaboard/apps/auth` 执行 `npm ci && npm run migrate:dist && npm prune --omit=dev` 后启动 `node dist/index.js`

同时需要在宝塔 Nginx 配置中添加：

```nginx
location /api/auth/ {
    proxy_pass http://127.0.0.1:6001;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location /api/ {
    proxy_pass http://127.0.0.1:6000;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

`AUTH_INTERNAL_KEY` 必须与 Go API 的 `AUTH_SERVICE_INTERNAL_KEY` 保持一致。
