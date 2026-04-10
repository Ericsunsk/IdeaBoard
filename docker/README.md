# FENG 项目 Docker 环境说明

## 快速启动

在 `docker` 目录下运行以下命令启动数据库服务：

```bash
docker-compose up -d
```

## 服务说明

### MySQL 5.7
- **端口**: 3306
- **Root 密码**: root123
- **数据库名**: feng
- **用户名**: feng
- **密码**: feng123

### Redis 7
- **端口**: 6379
- **无密码**

## 常用命令

```bash
# 启动服务
docker-compose up -d

# 停止服务
docker-compose down

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f mysql
docker-compose logs -f redis

# 重启服务
docker-compose restart

# 删除所有数据（慎用）
docker-compose down -v
```

## 连接信息

### MySQL
```bash
mysql -h localhost -P 3306 -u root -proot123
```

或使用 MySQL 客户端工具连接：
- Host: localhost
- Port: 3306
- User: root
- Password: root123

### Redis
```bash
redis-cli -h localhost -p 6379
```

或使用 Redis 客户端工具连接：
- Host: localhost
- Port: 6379
