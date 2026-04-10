# FENG Docker 开发环境

这个目录用于本地开发时启动 `MySQL + Redis`。当前 `docker-compose.yml` 仅面向开发和内部调试环境。

## 快速启动

在 `docker/` 目录下执行：

```bash
docker compose up -d
```

## 服务说明

### MySQL 5.7

- 容器名：`feng-mysql`
- 端口：`3306`
- Root 密码：`root123`
- 默认数据库：`feng`
- 默认业务用户：`feng / feng123`

补充：

- `mysql:5.7` 在 Apple Silicon 上没有原生 `arm64` 镜像
- 当前 compose 已显式配置 `platform: linux/amd64`，M 系列 Mac 可直接启动

### Redis 7

- 容器名：`feng-redis`
- 端口：`6379`
- 持久化：`appendonly yes`
- 当前未配置密码

## 常用命令

```bash
# 启动服务
docker compose up -d

# 停止服务
docker compose down

# 查看状态
docker compose ps

# 查看日志
docker compose logs -f mysql
docker compose logs -f redis

# 重启服务
docker compose restart

# 删除容器和卷（慎用）
docker compose down -v
```

## 连接信息

### MySQL

```bash
mysql -h localhost -P 3306 -u root -proot123
```

也可以用 GUI 客户端连接：

- Host：`localhost`
- Port：`3306`
- User：`root`
- Password：`root123`

### Redis

```bash
redis-cli -h localhost -p 6379
```

也可以用 GUI 客户端连接：

- Host：`localhost`
- Port：`6379`

## 常见问题

### `docker-compose: command not found`

新版 Docker Desktop 推荐使用 `docker compose`，不是旧的 `docker-compose`。

### `no matching manifest for linux/arm64/v8`

这是 Apple Silicon 上拉取 `mysql:5.7` 时的常见报错。当前仓库已经在 compose 里固定为 `linux/amd64`，拉取最新配置后重试即可。
