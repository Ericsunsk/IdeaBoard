# IdeaBoard 本地依赖手册

`docker/` 目录同时包含本地开发依赖和生产 Docker Compose stack：

- `compose.local.yml`：本地开发用 `MySQL + Redis`
- `compose.yml`：生产 VPS 用 `Cloudflared + Gateway + API + Auth + MySQL + Redis`

生产环境请优先使用根目录 [DEPLOYMENT.md](../DEPLOYMENT.md) 中的 GHCR + Docker Compose 部署方式。


## 快速启动

```bash
cd docker
cp .env.example .env
# 至少修改 MYSQL_ROOT_PASSWORD 和 MYSQL_PASSWORD；不要使用弱默认密码。
docker compose -f compose.local.yml up -d
```

查看状态：

```bash
docker compose -f compose.local.yml ps
```

查看日志：

```bash
docker compose -f compose.local.yml logs -f mysql
docker compose -f compose.local.yml logs -f redis
```


## 服务说明

### MySQL

| 项目 | 说明 |
|------|------|
| 镜像 | `mysql:8.0` |
| 容器名 | `ideaboard-mysql` |
| 宿主机端口 | `127.0.0.1:${MYSQL_PORT:-3306}` |
| Root 密码 | `${MYSQL_ROOT_PASSWORD}`，必填 |
| 默认数据库 | `${MYSQL_DATABASE:-ideaboard}` |
| 默认业务用户 | `${MYSQL_USER:-ideaboard}` |
| 默认业务密码 | `${MYSQL_PASSWORD}`，必填 |

> [!NOTE]
> compose 已显式固定 `platform: linux/amd64`，便于 Apple Silicon 环境直接运行。

> [!TIP]
> compose 初始化不再内置默认管理员账号；若你还会启动完整后端，请先启动 `apps/auth`，再执行 `cd apps/api && INIT_ADMIN_PASSWORD='your-strong-password' go run ./cmd init-admin`。

### Redis

| 项目 | 说明 |
|------|------|
| 镜像 | `redis:8.6.2` |
| 容器名 | `ideaboard-redis` |
| 宿主机端口 | `127.0.0.1:${REDIS_PORT:-6379}` |
| 持久化 | `appendonly yes` |
| 默认密码 | `${REDIS_PASSWORD:-无}` |


## 连接信息

### MySQL

命令行：

```bash
mysql -h localhost -P 3306 -u ideaboard -p"$MYSQL_PASSWORD" ideaboard
```

GUI 客户端：

- Host：`localhost`
- Port：`3306`
- User：`ideaboard`
- Password：你在 `docker/.env` 中设置的 `MYSQL_PASSWORD`

### Redis

命令行：

```bash
redis-cli -h localhost -p 6379 ${REDIS_PASSWORD:+-a "$REDIS_PASSWORD"}
```

GUI 客户端：

- Host：`localhost`
- Port：`6379`
- Password：若设置了 `REDIS_PASSWORD`，客户端也必须同步填写；后端请同步配置 `REDIS_AUTH`


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
docker/nginx/ideaboard.prod.conf
```

基础命令：

```bash
cp docker/.env.example docker/.env
docker compose -f docker/compose.yml --env-file docker/.env pull
docker compose -f docker/compose.yml --env-file docker/.env up -d
```

推荐生产环境使用 `ideaboard update`，它会在 Auth migration 前把 MySQL dump 写入 `backups/mysql/`，并在启动后检查 `api` 和 `auth` 的 `/api/health`。如果 health gate 失败，请先查看 `gateway/api/auth/mysql` 日志，再决定是否把 `IMAGE_TAG` 改回上一版重跑 `ideaboard update`；DB restore 需要基于 backup 人工确认后执行。

生产数据在 Docker volumes 中持久化：`mysql-data`、`redis-data`、`api-uploads`。不要对生产 stack 执行 `docker compose down -v`。


## 常见问题

### `docker-compose: command not found`

请使用新版命令：

```bash
docker compose -f compose.local.yml up -d
```

### Apple Silicon 拉取 MySQL 报错

如果你在 Apple Silicon 上看到类似 `no matching manifest for linux/arm64/v8` 的错误，通常是镜像架构问题。当前仓库已在 compose 中固定 `linux/amd64`，拉取最新配置后重试即可。

### 后端连不上 MySQL

请检查：

1. `docker compose -f compose.local.yml ps` 中 MySQL 是否为 `Up`。
2. `apps/api/.env` 中 `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME` 是否与 compose 配置一致。
3. 若你改过 `MYSQL_DATABASE`、`MYSQL_USER` 或 `MYSQL_PASSWORD`，后端也必须同步修改。
