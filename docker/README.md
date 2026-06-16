# IdeaBoard 本地依赖手册

`docker/` 目录同时包含本地开发依赖和生产 Docker Compose stack：

- `compose.local.yml`：本地开发用 PostgreSQL + NATS
- `compose.yml`：生产 VPS 用 `宿主机反代 + API + Auth + PostgreSQL + NATS`

生产环境请优先使用根目录 [DEPLOYMENT.md](../DEPLOYMENT.md) 中的 GHCR + Docker Compose 部署方式。


## 快速启动

```bash
cd docker
cp .env.example .env
# 生产 stack 至少修改 POSTGRES_PASSWORD 和对外域名/密钥。
docker compose -f compose.local.yml up -d
```

查看状态：

```bash
docker compose -f compose.local.yml ps
```

查看日志：

```bash
docker compose -f compose.local.yml logs -f postgres
```


## 服务说明

### PostgreSQL

| 项目 | 说明 |
|------|------|
| 镜像 | `postgres:17-alpine` |
| 容器名 | `ideaboard-postgres` |
| 宿主机端口 | `127.0.0.1:${POSTGRES_PORT:-5432}` |
| 默认数据库 | `${POSTGRES_DB:-ideaboard}` |
| 默认业务用户 | `${POSTGRES_USER:-ideaboard}` |
| 默认业务密码 | `${POSTGRES_PASSWORD}`，必填 |

> [!TIP]
> compose 初始化不再内置默认管理员账号；若你还会启动完整后端，请先启动 `apps/auth`，再执行 `cd apps/api && INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin`。

## 连接信息

### PostgreSQL

命令行：

```bash
psql "postgres://ideaboard:$POSTGRES_PASSWORD@localhost:5432/ideaboard?sslmode=disable"
```

GUI 客户端：

- Host：`localhost`
- Port：`5432`
- User：`ideaboard`
- Password：你在 `docker/.env` 中设置的 `POSTGRES_PASSWORD`

## 常用操作

```bash
# 启动
docker compose -f compose.local.yml up -d

# 停止
docker compose -f compose.local.yml down

# 重启
docker compose -f compose.local.yml restart
```

> [!CAUTION]
> `docker compose down -v` 会删除容器和数据卷，清空本地开发数据。仅在明确要重置环境时使用。


## 生产 Stack

生产配置文件：

```text
docker/compose.yml
docker/.env.example
deploy/templates/nginx/ideaboard.conf.template
```

生产部署命令：

```bash
cp docker/.env.example docker/.env
nano docker/.env
IDEABOARD_STACK_DIR="$PWD" ./scripts/deploy-stack.sh update
```

默认生产入口是宿主机反代服务，当前用 `ideaboard nginx-conf /tmp/ideaboard.nginx.conf` 生成 API 域名的 Nginx 配置。默认 upstream 是 `127.0.0.1:6000/6001`；如果宿主机反代服务已能稳定直连 Docker bridge，可在 `docker/.env` 设置 `NGINX_API_UPSTREAM` / `NGINX_AUTH_UPSTREAM` 后重新生成配置。Cloudflare `Proxied` API 域名启用源站 allowlist 后，可用 `ideaboard cloudflare-real-ip-conf /tmp/cloudflare-real-ip.conf` 生成真实客户端 IP include。

后端镜像由 GitHub Actions 发布到 GHCR；发布路径会生成 SBOM、执行 Grype 高危漏洞 gate，并用 Cosign keyless 签名。生产 `.env` 必须填写 `IDEABOARD_API_IMAGE`、`IDEABOARD_AUTH_IMAGE` 与 `IDEABOARD_AUTH_MIGRATE_IMAGE` 的完整 digest 引用，`ideaboard update` 会在 pull 前自动验签。

API 运行时还必须设置 `APP_SECRETS_ENCRYPTION_KEY`，用于加密 API Key 与对象存储密钥；推荐用 `openssl rand -base64 32` 生成。

推荐生产环境使用 `ideaboard update`，它会在 API / Auth migrations 前把 PostgreSQL dump 写入 `/opt/stacks/ideaboard-backups/postgres-*.sql`，并在启动后检查 PostgreSQL、NATS JetStream、本机 API/Auth health、River schema、默认 JetStream event stream 和公网 `${API_BASE_URL}/api/health`。已启动的 stack 也可以随时执行 `ideaboard smoke` 复验。如果 health gate 失败，请先查看 `api/auth/postgres/nats` 日志，再决定是否把三张镜像 digest 改回上一版重跑 `ideaboard update`；DB restore 需要基于 backup 人工确认后执行。

> [!IMPORTANT]
> `postgres-*.sql` 会包含 `api_account_secrets`、`storage_profile_secrets` 等表中的上游 API key 与 S3 secret，必须按 secret artifact 处理：建议加密保存、限制传输与下载，restore 后轮换受影响的上游 API key 和对象存储凭据。

直接运行 `docker compose pull && docker compose up -d` 只适合本地排障；这条路径不会自动执行 Cosign 验签、API / Auth migrations、数据备份和 smoke gate。

生产数据在 Docker volumes 中持久化：`postgres-data`、`nats-data`、`api-uploads`。不要对生产 stack 执行 `docker compose down -v`。

当前 Docker stack 管理 PostgreSQL 数据库与 NATS JetStream 数据目录；部署脚本不会自动转换或导入其他数据库形态的数据。


## 常见问题

### `docker-compose: command not found`

请使用新版命令：

```bash
docker compose -f compose.local.yml up -d
```

### 后端连不上 PostgreSQL

请检查：

1. `docker compose -f compose.local.yml ps` 中 PostgreSQL 是否为 `Up`。
2. `apps/api/.env` 中 `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME` 是否与 compose 配置一致。
3. 若你改过 `POSTGRES_DB`、`POSTGRES_USER` 或 `POSTGRES_PASSWORD`，后端也必须同步修改。

> 说明：`APP_BASE_URL` 和 `API_BASE_URL` 只是 compose / deploy 层的输入；API 容器实际读取 `APP_PUBLIC_BASE_URL`、`APP_PUBLIC_ASSET_BASE_URL` 和 `AUTH_SERVICE_BASE_URL`，Auth 容器读取 `BETTER_AUTH_URL`。
