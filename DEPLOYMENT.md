# <div align="center">IdeaBoard Linux 部署手册</div>

<p align="center">
  <strong>适用于标准 Linux 服务器的部署、升级与回滚手册。</strong>
</p>

<p align="center">
  目标运行形态：<code>Nginx + systemd + MySQL + Redis</code>
</p>

<p align="center">
  <code>私有</code>
  <code>闭源</code>
  <code>内部运维文档</code>
</p>

<p align="center">
  <a href="#deployment-profile">部署概览</a> ·
  <a href="#release-artifact">发布产物</a> ·
  <a href="#install-flow">安装流程</a> ·
  <a href="#runtime-config">运行配置</a> ·
  <a href="#upgrade-and-rollback">升级回滚</a>
</p>

---

> [!IMPORTANT]
> 本文档仅供 IdeaBoard 授权成员在内部部署与运维场景下使用。项目为闭源私有项目，未经授权不得复制、分发或对外传播。

<a id="deployment-profile"></a>
## 部署概览

本文档适用于 `Linux amd64` 服务器，以 `Nginx + systemd + MySQL + Redis` 为基础运行方式。

### 目标拓扑

```text
互联网
  -> Nginx :80
    -> apps/web/dist
    -> /api /uploads proxy
      -> ideaboard-server :6000
        -> MySQL
        -> Redis
        -> OSS / 上游 AI 渠道
```

### 当前约定

| 项目 | 当前约定 |
|------|----------|
| 部署根目录 | `/opt/ideaboard` |
| systemd 服务名 | `ideaboard` |
| Nginx 配置名 | `ideaboard.conf` |
| 后端监听端口 | `6000` |
| 发布包名 | `ideaboard-bundle-<tag>-linux-amd64.tar.gz` |
| API 可执行文件 | `apps/api/ideaboard-server` |

<a id="release-artifact"></a>
## 发布产物

标准发布包由以下脚本生成：

```bash
./scripts/package-deploy.sh
```

产物结构示例：

```text
release/v0.1.0/ideaboard-bundle-v0.1.0-linux-amd64/
├── apps/
│   ├── api/
│   │   ├── ideaboard-server
│   │   ├── uploads/
│   │   └── .env.example
│   └── web/
│       └── dist/
├── deploy/
│   ├── nginx/
│   │   └── ideaboard.conf
│   └── systemd/
│       └── ideaboard.service
├── README.md
├── DEVELOPMENT.md
└── DEPLOYMENT.md
```

## 前置条件

服务器建议满足：

- 系统：`Linux amd64`
- 端口：`80`
- 软件：`nginx`、`mysql`、`redis-server`、`tar`
- 可选依赖：`potrace`，用于转矢量能力

Ubuntu / Debian 参考安装命令：

```bash
sudo apt update
sudo apt install -y nginx redis-server mysql-server
sudo apt install -y potrace
```

<a id="install-flow"></a>
## 安装流程

### 1. 下载并校验发布包

```bash
ssh root@你的服务器IP
cd /root
TAG=v0.1.0
BUNDLE="ideaboard-bundle-${TAG}-linux-amd64.tar.gz"
curl -L -o "$BUNDLE" \
  "https://github.com/2837123/Feng-AI/releases/download/${TAG}/${BUNDLE}"
sha256sum "$BUNDLE"
```

校验值以 GitHub Release 页面中的 `SHA256SUMS.txt` 为准。

### 2. 解压到部署目录

推荐目录结构：

```text
/opt/ideaboard/
├── apps/
│   ├── api/
│   └── web/
├── deploy/
├── logs/
└── releases/
```

执行：

```bash
mkdir -p /opt/ideaboard/releases /opt/ideaboard/logs
cd /opt/ideaboard/releases
tar -xzf "/root/${BUNDLE}"
RELEASE_DIR="/opt/ideaboard/releases/ideaboard-bundle-${TAG}-linux-amd64"

mkdir -p /opt/ideaboard/apps /opt/ideaboard/deploy
cp -R "${RELEASE_DIR}/apps/." /opt/ideaboard/apps/
cp -R "${RELEASE_DIR}/deploy/." /opt/ideaboard/deploy/
```

> [!IMPORTANT]
> 上面的覆盖步骤会合并复制新版本文件。不要顺手删除已有的 `.env`、上传目录或数据库数据。

### 3. 准备数据库与缓存

#### MySQL

```bash
mysql -uroot -p
```

```sql
CREATE DATABASE ideaboard DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER 'ideaboard'@'127.0.0.1' IDENTIFIED BY '请替换为强密码';
GRANT ALL PRIVILEGES ON ideaboard.* TO 'ideaboard'@'127.0.0.1';
FLUSH PRIVILEGES;
```

#### Redis

```bash
sudo systemctl enable redis-server
sudo systemctl restart redis-server
sudo systemctl status redis-server --no-pager
```

如果你的系统服务名是 `redis`，请替换为对应名称。

<a id="runtime-config"></a>
## 运行配置

### 1. 配置后端环境变量

```bash
cp /opt/ideaboard/apps/api/.env.example /opt/ideaboard/apps/api/.env
nano /opt/ideaboard/apps/api/.env
```

部署时至少确认：

- `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`
- `REDIS_HOST/REDIS_PORT/REDIS_AUTH`
- `APP_PORT=6000`
- `JWT_SECRET`

建议：

- 不要在生产环境继续使用本地开发默认账号
- 若 Redis 开启密码，必须同步配置 `REDIS_AUTH`
- `JWT_SECRET` 必须为足够长的随机字符串

完整字段说明见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

### 2. 初始化管理员账号

```bash
cd /opt/ideaboard/apps/api
set -a
source /opt/ideaboard/apps/api/.env
set +a
./ideaboard-server init-admin
```

默认管理员账号：

- 用户名：`admin`
- 密码：`admin123`

> [!IMPORTANT]
> 数据库连接账号和网站管理员账号是两套信息。`/opt/ideaboard/apps/api/.env` 只决定服务如何连接 MySQL；后台登录账号来自业务库的 `users` 表。首次部署完成后，建议立刻登录后台并修改默认管理员密码。

### 3. 配置 systemd

```bash
cp /opt/ideaboard/deploy/systemd/ideaboard.service /etc/systemd/system/ideaboard.service
systemctl daemon-reload
systemctl enable ideaboard
systemctl restart ideaboard
systemctl status ideaboard --no-pager
```

查看日志：

```bash
journalctl -u ideaboard -n 100 --no-pager
```

### 4. 配置 Nginx

```bash
cp /opt/ideaboard/deploy/nginx/ideaboard.conf /etc/nginx/sites-available/ideaboard.conf
ln -sf /etc/nginx/sites-available/ideaboard.conf /etc/nginx/sites-enabled/ideaboard.conf
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl reload nginx
systemctl enable nginx
```

如果有正式域名，请将：

```nginx
server_name _;
```

替换为真实域名。

## 验证清单

后端健康检查：

```bash
curl http://127.0.0.1:6000/api/health
```

Nginx 验证：

```bash
curl -I http://127.0.0.1/
curl http://127.0.0.1/api/health
```

上线前至少确认：

1. 首页可正常访问。
2. `/api/health` 返回成功。
3. 管理员可登录。
4. 图片上传正常。
5. `/uploads/*` 可以访问。
6. 任务提交后能看到状态流转与结果落地。

## 运维命令

重启后端：

```bash
systemctl restart ideaboard
```

查看后端状态：

```bash
systemctl status ideaboard --no-pager
```

查看后端日志：

```bash
journalctl -u ideaboard -f
```

重载 Nginx：

```bash
nginx -t && systemctl reload nginx
```

<a id="upgrade-and-rollback"></a>
## 升级与回滚

### 升级

升级新版本时，重复以下步骤：

1. 下载新的发布包。
2. 解压到 `/opt/ideaboard/releases/`。
3. 用新版本覆盖 `/opt/ideaboard/apps`、`/opt/ideaboard/deploy`。
4. 保留原有 `/opt/ideaboard/apps/api/.env`。
5. 执行 `systemctl restart ideaboard`。
6. 再次执行健康检查与页面验证。

升级时不要删除：

- `/opt/ideaboard/apps/api/.env`
- `/opt/ideaboard/apps/api/uploads/`

### 回滚

如果新版本异常：

1. 回到 `/opt/ideaboard/releases/` 中上一个版本目录。
2. 重新复制旧版 `apps`、`deploy`。
3. 执行 `systemctl restart ideaboard`。
4. 再次验证 `curl http://127.0.0.1:6000/api/health`。
