# FENG 服务器部署步骤

本文档基于已经生成好的发布包进行部署，适用于 `Linux amd64` 服务器。  
发布包命名规则：

- 部署包：`feng-bundle-${TAG}-linux-amd64.tar.gz`
- 示例下载：`https://github.com/2837123/Feng-AI/releases/download/v0.1.0/feng-bundle-v0.1.0-linux-amd64.tar.gz`
- SHA256：以 Release 页面（`SHA256SUMS.txt`）为准

以下步骤默认使用：

- 前端：`Nginx` 托管静态文件
- 后端：`systemd` 守护 `feng-server`
- 数据库：`MySQL 5.7+`
- 缓存：`Redis 7+`

如果你的服务器已经有 `MySQL`、`Redis`、`Nginx`，可以跳过对应安装步骤。

## 1. 服务器准备

确认服务器满足以下条件：

- 系统：`Linux amd64`
- 已开放端口：`80`（Nginx），`8080`（后端仅本机回环即可）
- 已安装：`tar`、`nginx`、`mysql`、`redis`

如果是 Ubuntu / Debian，可以执行：

```bash
sudo apt update
sudo apt install -y nginx redis-server mysql-server
```

如果需要转矢量功能，还要安装：

```bash
sudo apt install -y potrace
```

## 2. 获取发布包到服务器

从 GitHub Release 拉取：

```bash
ssh root@你的服务器IP
cd /root
TAG=v0.1.0
BUNDLE="feng-bundle-${TAG}-linux-amd64.tar.gz"
curl -L -o "$BUNDLE" \
  "https://github.com/2837123/Feng-AI/releases/download/${TAG}/${BUNDLE}"
```

校验文件完整性：

```bash
cd /root
sha256sum "$BUNDLE"
```

输出中应包含文件名：

```text
<sha256>  $BUNDLE
```

## 3. 解压到部署目录

建议部署目录：

```text
/opt/feng/
├── backend/
├── frontend/
├── deploy/
└── logs/
```

执行：

```bash
mkdir -p /opt/feng/releases
mkdir -p /opt/feng/logs
cd /opt/feng/releases
tar -xzf "/root/${BUNDLE}"
RELEASE_DIR="/opt/feng/releases/feng-bundle-${TAG}-linux-amd64"
rm -rf /opt/feng/backend /opt/feng/frontend /opt/feng/deploy
cp -R "${RELEASE_DIR}/backend" /opt/feng/backend
cp -R "${RELEASE_DIR}/frontend" /opt/feng/frontend
cp -R "${RELEASE_DIR}/deploy" /opt/feng/deploy
```

## 4. 准备 MySQL 和 Redis

### 4.1 创建数据库

进入 MySQL：

```bash
mysql -uroot -p
```

执行：

```sql
CREATE DATABASE feng DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'feng'@'127.0.0.1' IDENTIFIED BY '请替换为强密码';
GRANT ALL PRIVILEGES ON feng.* TO 'feng'@'127.0.0.1';
FLUSH PRIVILEGES;
```

### 4.2 启动 Redis

确认 Redis 已运行：

```bash
sudo systemctl enable redis-server
sudo systemctl restart redis-server
sudo systemctl status redis-server --no-pager
```

如果你的系统服务名是 `redis`，把上面的 `redis-server` 替换成 `redis`。

## 5. 配置后端环境变量

复制环境变量模板：

```bash
cp /opt/feng/backend/.env.example /opt/feng/backend/.env
```

编辑：

```bash
nano /opt/feng/backend/.env
```

环境变量字段的完整说明统一维护在 `DEVELOPMENT.md` 的“环境变量（后端）”章节。

这里仅列生产部署必须确认项：

- `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`：必须与线上 MySQL 实际账号一致
- `REDIS_HOST/REDIS_PORT/REDIS_AUTH`：必须与线上 Redis 配置一致
- `APP_PORT`：与 systemd/Nginx 反向代理端口一致（默认 `8080`）
- `JWT_SECRET`：必须替换为足够长的随机字符串

额外建议：

- 生产环境不要继续使用默认数据库账号 `root/root123`
- 如果 Redis 开启了密码，必须配置 `REDIS_AUTH`

## 6. 初始化管理员账号

进入后端目录并加载环境变量：

```bash
cd /opt/feng/backend
set -a
source /opt/feng/backend/.env
set +a
./feng-server init-admin
```

默认管理员账号：

- 用户名：`admin`
- 密码：`admin123`

首次登录后请立即修改密码。

## 7. 配置 systemd 启动后端

把打包产物里的 service 文件复制到系统目录：

```bash
cp /opt/feng/deploy/systemd/feng.service /etc/systemd/system/feng.service
```

重新加载并启动：

```bash
systemctl daemon-reload
systemctl enable feng
systemctl restart feng
systemctl status feng --no-pager
```

查看日志：

```bash
journalctl -u feng -n 100 --no-pager
```

如果启动失败，先检查：

- `/opt/feng/backend/.env` 是否存在
- MySQL 和 Redis 是否可连接
- `JWT_SECRET` 是否已配置

## 8. 配置 Nginx

复制站点配置：

```bash
cp /opt/feng/deploy/nginx/feng.conf /etc/nginx/sites-available/feng.conf
ln -sf /etc/nginx/sites-available/feng.conf /etc/nginx/sites-enabled/feng.conf
rm -f /etc/nginx/sites-enabled/default
```

测试配置并重载：

```bash
nginx -t
systemctl reload nginx
systemctl enable nginx
```

如果你有正式域名，把配置里的：

```nginx
server_name _;
```

改成你的域名，例如：

```nginx
server_name ai.example.com;
```

## 9. 验证部署

先检查后端健康接口：

```bash
curl http://127.0.0.1:8080/api/health
```

预期返回包含：

```json
{"code":0,"data":{"status":"ok"},"message":"success"}
```

再检查 Nginx：

```bash
curl -I http://127.0.0.1/
curl http://127.0.0.1/api/health
```

浏览器访问：

- `http://你的服务器IP/`

上线前至少确认：

1. 首页能正常打开
2. `/api/health` 返回成功
3. 管理员可以登录
4. 图片上传正常
5. `/uploads/*` 可以访问
6. 提交任务后可以轮询到状态

## 10. 常用运维命令

重启后端：

```bash
systemctl restart feng
```

查看后端状态：

```bash
systemctl status feng --no-pager
```

查看后端日志：

```bash
journalctl -u feng -f
```

重载 Nginx：

```bash
nginx -t && systemctl reload nginx
```

## 11. 更新版本

以后升级新版本时，重复以下流程：

1. 从 GitHub Release 下载新的 `tar.gz` 包到服务器
2. 解压到 `/opt/feng/releases/`
3. 用新版本覆盖 `/opt/feng/backend`、`/opt/feng/frontend`、`/opt/feng/deploy`
4. 保留原有 `/opt/feng/backend/.env`
5. 执行 `systemctl restart feng`
6. 检查 `curl http://127.0.0.1:8080/api/health`

建议不要在升级时删除：

- `/opt/feng/backend/.env`
- `/opt/feng/backend/uploads/`

## 12. 回滚方法

如果新版本有问题：

1. 回到 `/opt/feng/releases/` 中上一个版本目录
2. 重新复制旧版 `backend`、`frontend`、`deploy`
3. 执行 `systemctl restart feng`
4. 再次检查健康接口

## 13. 当前这次部署最短执行路径

如果你已经有一台装好 `Nginx + MySQL + Redis` 的服务器，最短流程就是：

```bash
ssh root@你的服务器IP
cd /root
TAG=v0.1.0
BUNDLE="feng-bundle-${TAG}-linux-amd64.tar.gz"
curl -L -o "$BUNDLE" \
  "https://github.com/2837123/Feng-AI/releases/download/${TAG}/${BUNDLE}"
sha256sum "$BUNDLE"
mkdir -p /opt/feng/releases /opt/feng/logs
cd /opt/feng/releases
tar -xzf "/root/${BUNDLE}"
RELEASE_DIR="/opt/feng/releases/feng-bundle-${TAG}-linux-amd64"
cp -R "${RELEASE_DIR}/backend" /opt/feng/backend
cp -R "${RELEASE_DIR}/frontend" /opt/feng/frontend
cp -R "${RELEASE_DIR}/deploy" /opt/feng/deploy
cp /opt/feng/backend/.env.example /opt/feng/backend/.env
nano /opt/feng/backend/.env
cd /opt/feng/backend && set -a && source .env && set +a && ./feng-server init-admin
cp /opt/feng/deploy/systemd/feng.service /etc/systemd/system/feng.service
systemctl daemon-reload && systemctl enable feng && systemctl restart feng
cp /opt/feng/deploy/nginx/feng.conf /etc/nginx/sites-available/feng.conf
ln -sf /etc/nginx/sites-available/feng.conf /etc/nginx/sites-enabled/feng.conf
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx
curl http://127.0.0.1:8080/api/health
```
