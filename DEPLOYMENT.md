# FENG 部署说明

本文档用于 `FENG` 项目的部署、发布和回滚。

说明：

- 仓库发布号建议使用 `SemVer`：`vMAJOR.MINOR.PATCH`（例如 `v0.1.0`）
- 前端 `package.json` 使用兼容 `npm` 的 `SemVer`：`0.0.1`

## 1. 部署目标

当前项目形态：

- 前端：`Vue 3 + Vite`，构建后产物位于 `frontend/dist/`
- 后端：`Gin` HTTP 服务
- 基础设施：`MySQL 5.7 + Redis 7`

推荐部署方式：

1. `MySQL / Redis` 继续用 Docker 或宿主机现成服务
2. 前端用 `Nginx` 托管静态文件
3. 后端用 `systemd` 或进程守护方式运行

## 2. 服务器要求

- Linux `amd64`
- `Go 1.22+`
- `Node.js 20+`
- `Nginx`
- `MySQL 5.7+`
- `Redis 7+`
- 如果启用转矢量功能，还需要 `potrace`

## 3. 关键环境变量

后端支持以下环境变量：

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
| `JWT_SECRET` | 无固定默认值 | 生产环境必须显式设置 |

生产建议：

- 不要依赖默认数据库密码
- `JWT_SECRET` 必须设置为高强度随机值
- 生产环境建议关闭调试日志落盘，或单独接日志系统

## 4. 本地打包

### 前端

```bash
cd frontend
npm install
npm run build
```

### 后端

当前机器本地构建：

```bash
cd backend
go build ./...
```

Linux 发布二进制：

```bash
cd backend
TAG=v0.1.0
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o feng-backend-${TAG}-linux-amd64 ./cmd/main.go
```

## 5. 推荐部署目录

```text
/opt/feng/
├── backend/
│   ├── feng-server
│   ├── .env
│   └── uploads/
├── frontend/
│   └── dist/
└── logs/
```

## 6. 首次部署步骤

### 6.1 启动数据库

如果沿用仓库内 Docker 配置：

```bash
cd docker
docker compose up -d
```

### 6.2 初始化管理员账号

```bash
cd backend
go run cmd/main.go init-admin
```

默认账号：

- 用户名：`admin`
- 密码：`admin123`

首次登录后建议立刻修改。

### 6.3 启动后端

```bash
cd backend
JWT_SECRET='replace-with-a-long-random-secret' APP_PORT=8080 ./feng-server
```

### 6.4 部署前端

将 `frontend/dist/` 发布到 `Nginx` 静态目录。

## 7. Nginx 示例

```nginx
server {
    listen 80;
    server_name _;

    root /opt/feng/frontend/dist;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /uploads/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## 8. systemd 示例

```ini
[Unit]
Description=FENG Backend
After=network.target

[Service]
WorkingDirectory=/opt/feng/backend
ExecStart=/opt/feng/backend/feng-server
Restart=always
RestartSec=3
Environment=APP_PORT=8080
Environment=DB_HOST=127.0.0.1
Environment=DB_PORT=3306
Environment=DB_USER=root
Environment=DB_PASSWORD=replace-me
Environment=DB_NAME=feng
Environment=REDIS_HOST=127.0.0.1
Environment=REDIS_PORT=6379
Environment=JWT_SECRET=replace-with-a-long-random-secret

[Install]
WantedBy=multi-user.target
```

## 9. 发布检查

发布后至少检查以下内容：

1. `GET /api/health` 返回成功
2. 用户登录正常
3. 管理员登录正常
4. 上传图片正常
5. `/uploads/*` 可访问
6. 提交任务后状态可轮询

## 10. GitHub Actions 自动发布

仓库已支持自动发布 Workflow：

1. 本地完成版本变更后，提交到 `main`
2. 推送一个符合 `v*` 规则的 tag，例如：

```bash
git tag -a v0.1.0 -m "发布 v0.1.0"
git push origin main
git push origin v0.1.0
```

3. GitHub Actions 会自动执行：

- 前端 `npm ci && npm run build`
- 后端构建 `linux/amd64` 和 `darwin/arm64` 二进制（命名：`feng-backend-<tag>-<os>-<arch>`）
- 打包前端产物（命名：`feng-frontend-<tag>.tar.gz`）
- 组装一体化部署包（命名：`feng-bundle-<tag>-linux-amd64.tar.gz`）
- 生成 `RELEASE_INFO.txt`
- 生成 `SHA256SUMS.txt`
- 自动创建或更新对应 GitHub Release

也支持在 GitHub Actions 页面通过 `workflow_dispatch` 手动输入 tag 重新发布。

## 11. 回滚建议

如果某个版本发布后要快速回滚：

1. 保留上一个可用版本的前端 `dist/`
2. 保留上一个可用版本的后端二进制
3. 回滚时优先替换前后端文件，再重启后端进程
4. 避免在回滚时做额外 schema 变更

## 12. 产物命名规范

- 后端：`feng-backend-<tag>-<os>-<arch>`
- 前端：`feng-frontend-<tag>.tar.gz`
- 部署包：`feng-bundle-<tag>-linux-amd64.tar.gz`
- 发布目录：`release/<tag>/`
