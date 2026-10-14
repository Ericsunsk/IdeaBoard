# IdeaBoard 本地依赖手册

`docker/` 目录同时包含本地开发依赖和生产 Docker Compose stack：

- `compose.local.yml`：本地开发用 PostgreSQL + NATS
- `compose.yml`：生产 VPS 用 `宿主机反代 + API + Auth + PostgreSQL + NATS`
- `compose.npm.yml`：可选的 Nginx Proxy Manager external network override

生产环境请优先使用 [docs/runbooks/deployment.md](../docs/runbooks/deployment.md) 中的 GHCR + Docker Compose 部署方式。

## 快速启动

```bash
cd docker
cp .env.example .env
cp migration.env.example migration.env
# migration.env 的 DB_MIGRATION_PASSWORD 必须与 .env 的 POSTGRES_PASSWORD 相同。
# compose.local.yml 仅启动本地开发依赖（PostgreSQL + NATS），不包含应用容器。
docker compose -f compose.local.yml up -d
```

`migration.env` 只供本地 API/Auth migration command 临时加载，已由 Git 忽略，不要把其中的 schema owner credential 复制到长期运行的 API/Auth `.env`。完整迁移顺序与命令见 [DEVELOPMENT.md](../DEVELOPMENT.md)。

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

| 项目                          | 说明                                                                |
| ----------------------------- | ------------------------------------------------------------------- |
| 镜像                          | `postgres:17-alpine`                                                |
| 容器名                        | `ideaboard-postgres`                                                |
| 宿主机端口                    | `127.0.0.1:${POSTGRES_PORT:-5432}`                                  |
| 默认数据库                    | `ideaboard`                                                         |
| Schema owner / migration user | `ideaboard` / `${POSTGRES_PASSWORD}`                                |
| API runtime user              | `ideaboard_api` / `${API_DB_PASSWORD}`，无 Credits 表写权限         |
| Auth runtime user             | `ideaboard_auth` / `${AUTH_DB_PASSWORD}`，仅 Better Auth tables DML |

> [!TIP]
> compose 初始化不再内置默认管理员账号；若你还会启动完整后端，请先启动 `apps/auth`，再执行 `cd apps/api && INIT_ADMIN_EMAIL='admin@example.com' INIT_ADMIN_DISPLAY_NAME='Admin' INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin`。

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
docker/compose.npm.yml
docker/.env.example
deploy/templates/nginx/ideaboard.conf.template
```

生产部署命令：

```bash
cp docker/.env.example docker/.env
nano docker/.env
IDEABOARD_STACK_DIR="$PWD" ./scripts/deploy-stack.sh update \
  --manifest release-manifest.json
```

默认生产入口是宿主机反代服务，当前用 `ideaboard nginx-conf /tmp/ideaboard.nginx.conf` 生成 `ideaboard.art` Gateway 的 Nginx 配置。默认 upstream 是 `127.0.0.1:6000/6001`；生产环境设置 `NGINX_SERVER_NAME=ideaboard.art` 后重新生成配置。如果宿主机反代服务已能稳定直连 Docker bridge，可在 `docker/.env` 设置 `NGINX_API_UPSTREAM` / `NGINX_AUTH_UPSTREAM` 后重新生成配置。Cloudflare `Proxied` Gateway 启用源站 allowlist 后，可用 `ideaboard cloudflare-real-ip-conf /tmp/cloudflare-real-ip.conf` 生成真实客户端 IP include。

使用 Nginx Proxy Manager 时，不要直接修改 `compose.yml`。先确保 external network 已存在：

```bash
docker network create npm-nw
```

然后在生产 `docker/.env` 启用正式 override：

```text
IDEABOARD_COMPOSE_OVERRIDE_FILE=docker/compose.npm.yml
NPM_NETWORK_NAME=npm-nw
```

`ideaboard update --manifest ...`、`up`、`restart`、`status`、`doctor` 与卸载流程都会加载该 override。NPM 应使用 Docker DNS 转发到 `ideaboard-api:6000`；仅在确实需要独立代理 Auth 时使用 `ideaboard-auth:6001`。PostgreSQL 与 NATS 不会加入 NPM network。

后端镜像由 GitHub Actions 发布到 GHCR；发布路径会生成 SBOM、执行 Grype 高危漏洞 gate，并用 Cosign keyless 签名。`ideaboard update --manifest ...` 从目标发布的 manifest 写入四张完整 digest 引用（API runtime、API migrator、Auth runtime、Auth migrator），并在 pull 前自动验签；不接受 mutable tag，也不从 tag 反查 digest。

API 运行时还必须设置 `APP_SECRETS_ENCRYPTION_KEY`，用于加密 API Key 与对象存储密钥；推荐用 `openssl rand -base64 32` 生成。

生产环境只使用 `ideaboard update --manifest release-manifest.json` 部署同一发布批次的四张镜像。脚本会先执行 preflight，在 API / Auth migrations 前把 PostgreSQL custom dump 写入 `/opt/stacks/ideaboard-backups/postgres-*.dump[.age|.gpg]`，并在启动后检查 PostgreSQL、NATS JetStream、本机 API/Auth health、River schema、默认 JetStream event stream 和公网 `${PUBLIC_ORIGIN}/api/health`（如设置 `API_BASE_URL` override，则使用 override）。成功后会写入 `docker/releases/current.json` 与 `previous.json`；已启动的 stack 可用 `ideaboard status` 查看 release 摘要，也可以执行 `ideaboard doctor` / `ideaboard doctor --deep` 排查运行态。如果 health gate 失败，请先查看 `api/auth/postgres/nats` 日志；需要回退运行镜像时用 `ideaboard rollback --to previous`，DB restore 需要基于 backup 人工确认后执行。

> [!IMPORTANT]
> `postgres-*.dump[.age|.gpg]` 会包含 `api_account_secrets`、`storage_profile_secrets` 等表中的上游 API key 与 S3 secret，必须按 secret artifact 处理：建议设置 `BACKUP_ENCRYPTION_RECIPIENT` 加密保存、限制传输与下载，restore 后轮换受影响的上游 API key 和对象存储凭据。

Backup hardening 运行约定：

- 每个 PostgreSQL 与 uploads backup 应带同名 `.sha256`；restore 前先对最终落盘 artifact 执行 `sha256sum -c <backup>.sha256`。
- `BACKUP_ENCRYPTION_RECIPIENT=age1...`、`ssh-*` 或 `age:<recipient>` 表示使用 age recipient 加密；`gpg:<recipient>` 或其他值按 GPG recipient 处理。未设置时 backup 仍是明文 secret artifact。
- `ideaboard doctor` 应显示最近 backup 路径、时间、checksum 校验状态、是否加密和最近 release 中的 backup 阶段状态。
- DB restore 仍需人工确认 migration 影响和 uploads 是否同步恢复；脚本不自动恢复数据库。

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
2. `apps/api/.env` 使用 API runtime credential，`apps/auth/.env` 使用 Auth runtime credential；两个 migration command 才使用固定 owner `ideaboard` 与 `POSTGRES_PASSWORD`。
3. owner、API runtime、Auth runtime 三个 roles 必须不同；本地旧 volume 没有 runtime roles 时，重建开发 volume。

> 说明：`PUBLIC_ORIGIN` 是浏览器跳转、分享链接和同域 Gateway 的 canonical public Origin；`API_BASE_URL` 仅在 API 独立 host 时作为 override，用于 Stripe Webhook 及服务间 API 地址，不能作为浏览器或本地上传素材的默认公开地址。浏览器统一访问 `ideaboard.art` 的 `/api/**`、`/api/auth/**`，媒体字节经 Media Asset 与 public avatar/logo API 交付。`PUBLIC_ORIGIN` 缺失或不是纯 http(s) Origin 时，API 会拒绝启动。`DB_MIGRATION_USER` / `DB_MIGRATION_PASSWORD` 只提供给一次性的 migration command/container，不进入长期运行的 API/Auth container；本地命令从 `docker/migration.env.example` 创建已被 Git 忽略的 `docker/migration.env`。
