# IdeaBoard 本地依赖手册

`docker/` 目录用于本地开发时快速启动 `MySQL + Redis`。它只面向开发与内部调试，不应直接视为生产环境模板。

## 1. 快速启动

```bash
cd docker
docker compose up -d
```

查看状态：

```bash
docker compose ps
```

查看日志：

```bash
docker compose logs -f mysql
docker compose logs -f redis
```

## 2. 服务说明

### MySQL

| 项目 | 说明 |
|------|------|
| 镜像 | `mysql:8.0` |
| 容器名 | `ideaboard-mysql` |
| 端口 | `3306` |
| Root 密码 | `${MYSQL_ROOT_PASSWORD:-root123}` |
| 默认数据库 | `${MYSQL_DATABASE:-ideaboard}` |
| 默认业务用户 | `${MYSQL_USER:-ideaboard}` |
| 默认业务密码 | `${MYSQL_PASSWORD:-ideaboard123}` |

补充：

- compose 已显式固定 `platform: linux/amd64`，便于 Apple Silicon 环境直接运行。
- 如果本地已有旧的 `mysql:5.7` 数据卷，升级到 8.0 前请先备份；不兼容时建议删除开发卷后重新初始化。
- 若你还会启动完整后端，推荐仍执行一次 `cd backend && go run cmd/main.go init-admin`，不要依赖初始化 SQL 里的种子管理员口径。

### Redis

| 项目 | 说明 |
|------|------|
| 镜像 | `redis:7` |
| 容器名 | `ideaboard-redis` |
| 端口 | `6379` |
| 持久化 | `appendonly yes` |
| 默认密码 | 无 |

## 3. 连接信息

### MySQL

命令行：

```bash
mysql -h localhost -P 3306 -u ideaboard -pideaboard123 ideaboard
```

GUI 客户端：

- Host：`localhost`
- Port：`3306`
- User：`ideaboard`
- Password：`ideaboard123`，或你通过 compose 环境变量覆盖后的值

### Redis

命令行：

```bash
redis-cli -h localhost -p 6379
```

GUI 客户端：

- Host：`localhost`
- Port：`6379`

## 4. 常用操作

启动：

```bash
docker compose up -d
```

停止：

```bash
docker compose down
```

重启：

```bash
docker compose restart
```

删除容器和卷：

```bash
docker compose down -v
```

该命令会清空本地开发数据，仅在你明确要重置环境时使用。

## 5. 常见问题

### `docker-compose: command not found`

请使用新版命令：

```bash
docker compose up -d
```

### Apple Silicon 拉取 MySQL 报错

如果你在 Apple Silicon 上看到类似 `no matching manifest for linux/arm64/v8` 的错误，通常是镜像架构问题。当前仓库已在 compose 中固定 `linux/amd64`，拉取最新配置后重试即可。

### 后端连不上 MySQL

请检查：

1. `docker compose ps` 中 MySQL 是否为 `Up`。
2. `backend/.env` 中 `DB_HOST`、`DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME` 是否与 compose 配置一致。
3. 若你改过 `MYSQL_DATABASE`、`MYSQL_USER` 或 `MYSQL_PASSWORD`，后端也必须同步修改。
