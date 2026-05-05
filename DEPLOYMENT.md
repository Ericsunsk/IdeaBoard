# <div align="center">IdeaBoard Linux 部署手册</div>

<p align="center">
  <strong>适用于标准 Linux 服务器的部署、升级与回滚手册。</strong>
</p>

<p align="center">
  目标运行形态：<code>Nginx + systemd + MySQL + Redis + Better Auth</code>
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


> [!IMPORTANT]
> 本文档仅供 IdeaBoard 授权成员在内部部署与运维场景下使用。项目为闭源私有项目，未经授权不得复制、分发或对外传播。
>
> 账号安全与业务运维的职责边界请同时参考 [docs/AUTH_OPERATION_RUNBOOK.md](./docs/AUTH_OPERATION_RUNBOOK.md)。

<a id="deployment-profile"></a>
## 部署概览

本文档适用于 `Linux amd64` 服务器，以 `Nginx + systemd + MySQL + Redis + Better Auth Node Service` 为基础运行方式。

### 目标拓扑

```text
互联网
  -> Nginx :80
    -> apps/web/dist
    -> /api/auth proxy
      -> ideaboard-auth :6001
        -> MySQL
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
| systemd 服务名 | `ideaboard`、`ideaboard-auth` |
| Nginx 配置名 | `ideaboard.conf` |
| 后端监听端口 | `6000` |
| 认证服务监听端口 | `6001` |
| 发布包名 | `ideaboard-bundle-<tag>-linux-amd64.tar.gz` |
| API 可执行文件 | `apps/api/ideaboard-server` |
| Auth 入口文件 | `apps/auth/dist/index.js` |

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
│   ├── auth/
│   │   ├── dist/
│   │   ├── package.json
│   │   ├── package-lock.json
│   │   └── .env.example
│   └── web/
│       └── dist/
├── deploy/
│   ├── nginx/
│   │   └── ideaboard.conf
│   └── systemd/
│       ├── ideaboard.service
│       └── ideaboard-auth.service
├── docs/
│   ├── AUTH_OPERATION_RUNBOOK.md
│   ├── architecture-constraints.md
│   └── assets/
├── docker/
│   └── README.md
├── README.md
├── DEVELOPMENT.md
├── SYSTEM_PROMPT.md
├── CHANGELOG.md
└── DEPLOYMENT.md
```


## 前置条件

服务器建议满足：

- 系统：`Linux amd64`
- 端口：`80`
- 软件：`nginx`、`mysql`、`redis-server`、`nodejs >= 20`、`npm`、`tar`
- 可选依赖：`potrace`，用于转矢量能力

Ubuntu / Debian 参考安装命令：

```bash
sudo apt update
sudo apt install -y nginx redis-server mysql-server nodejs npm
sudo apt install -y potrace
```

> [!NOTE]
> 如果发行版仓库里的 Node.js 版本低于 20，请通过 NodeSource、nvm 或你的标准运维镜像安装 Node.js 20+。

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
│   ├── auth/
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

### 3. 创建 systemd 运行用户并校准目录权限

首次部署建议执行：

```bash
id -u ideaboard >/dev/null 2>&1 || useradd --system --home /opt/ideaboard --shell /usr/sbin/nologin ideaboard
chown -R ideaboard:ideaboard /opt/ideaboard/apps /opt/ideaboard/deploy /opt/ideaboard/logs
```

说明：

- `ideaboard` 为 systemd 服务默认运行用户（非 root）。
- 升级时请保留该用户与目录权限设置，避免服务因权限不足启动失败。

### 4. 准备数据库与缓存

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
- `APP_ENV=production`（生产环境必须显式设置）
- `APP_PORT=6000`
- `APP_HOST=127.0.0.1`（建议反向代理场景启用）
- `AUTH_SERVICE_BASE_URL=http://127.0.0.1:6001`
- `AUTH_SERVICE_INTERNAL_KEY`（必须与认证服务的 `AUTH_INTERNAL_KEY` 完全一致）

建议：

- 不要在生产环境继续使用本地开发默认账号
- 若 Redis 开启密码，必须同步配置 `REDIS_AUTH`
- 若前面有 Nginx / 其他反向代理，建议设置 `APP_HOST=127.0.0.1`
- 生产环境请固定设置 `APP_ENV=production`，不要依赖默认值
- `AUTH_SERVICE_INTERNAL_KEY` 建议使用足够长的随机字符串，并只在 API 与 Auth Service 两侧共享

完整字段说明见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

### 2. 配置认证服务环境变量并安装运行依赖

```bash
cp /opt/ideaboard/apps/auth/.env.example /opt/ideaboard/apps/auth/.env
nano /opt/ideaboard/apps/auth/.env
cd /opt/ideaboard/apps/auth
npm ci
npm run migrate:dist
npm prune --omit=dev
```

> [!IMPORTANT]
> 启用 Better Auth Admin plugin 后，必须先执行 `npm run migrate:dist`（开发环境对应 `npm --prefix apps/auth run migrate`），以补齐 `user.role`、`user.banned`、`user.banReason`、`user.banExpires` 与 `session.impersonatedBy` 字段；不要手写 DDL。

部署时至少确认：

- `AUTH_HOST=127.0.0.1`
- `AUTH_PORT=6001`
- `AUTH_INTERNAL_KEY` 与 `/opt/ideaboard/apps/api/.env` 的 `AUTH_SERVICE_INTERNAL_KEY` 一致
- `AUTH_TRUSTED_ORIGINS` 包含你的正式访问源，例如 `https://example.com`
- `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME` 与 API 使用同一个业务库

> [!IMPORTANT]
> Better Auth 会在同一个 MySQL 库里维护 `user/session/account/verification` 等认证表。`npm run migrate:dist` 会按 `dist/auth.js` 中的配置创建或更新这些认证表；当前项目仍由 Go API 维护业务侧 `users` 表，并通过 `auth_user_id` 关联 Better Auth 用户。
>
> 后台 `用户管理` 页当前支持：角色升降级、身份侧封禁/解封、会话吊销/踢下线、伪装登录/停止伪装、受控彻底注销、开放注册开关、防刷限流设置；不开放后台改密。角色升降级与彻底注销均由 Go Admin API 编排，并依赖 `AUTH_SERVICE_INTERNAL_KEY` 调用 Auth Service internal endpoint；限流配置来源于后台设置并写入 `system_configs`，不通过环境变量注入。

### 3. 配置 systemd

```bash
cp /opt/ideaboard/deploy/systemd/ideaboard.service /etc/systemd/system/ideaboard.service
cp /opt/ideaboard/deploy/systemd/ideaboard-auth.service /etc/systemd/system/ideaboard-auth.service
systemctl daemon-reload
systemctl enable ideaboard-auth ideaboard
systemctl restart ideaboard-auth
systemctl restart ideaboard
systemctl status ideaboard-auth --no-pager
systemctl status ideaboard --no-pager
```

发布包内置的 systemd 服务默认包含：

- `ideaboard.service`：运行 Go API，读取 `/opt/ideaboard/apps/api/.env`
- `ideaboard-auth.service`：运行 Better Auth Node Service，读取 `/opt/ideaboard/apps/auth/.env`
- 两个服务都以 `ideaboard` 非 root 用户运行

查看日志：

```bash
journalctl -u ideaboard-auth -n 100 --no-pager
journalctl -u ideaboard -n 100 --no-pager
```

### 4. 初始化管理员账号

`init-admin` 会同时确保 Better Auth 侧账号存在，因此必须先启动 `ideaboard-auth`。

```bash
cd /opt/ideaboard/apps/api
set -a
source /opt/ideaboard/apps/api/.env
set +a
export INIT_ADMIN_PASSWORD='your-strong-password'
./ideaboard-server init-admin
```

初始化后的管理员账号：

- 用户名：`admin`
- 密码：由 `INIT_ADMIN_PASSWORD` 指定

> [!IMPORTANT]
> 数据库连接账号和网站管理员账号是两套信息。`/opt/ideaboard/apps/api/.env` 只决定服务如何连接 MySQL；后台登录账号来自 Better Auth 与业务库的关联数据。`init-admin` 如果未设置 `INIT_ADMIN_PASSWORD` 会直接拒绝执行。

### 5. 配置 Nginx

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

认证服务健康检查：

```bash
curl http://127.0.0.1:6001/api/health
```

Nginx 验证：

```bash
curl -I http://127.0.0.1/
curl http://127.0.0.1/api/health
```

上线前至少确认：

1. 首页可正常访问。
2. `/api/health` 返回成功。
3. `ideaboard-auth` 运行正常，登录/注册请求走 `/api/auth/*`。
4. 管理员可登录。
5. 图片上传正常。
6. `/uploads/*` 可以访问。
7. 任务提交后能看到状态流转与结果落地。


## 运维命令

重启后端与认证服务：

```bash
systemctl restart ideaboard-auth ideaboard
```

查看后端状态：

```bash
systemctl status ideaboard-auth --no-pager
systemctl status ideaboard --no-pager
```

查看后端日志：

```bash
journalctl -u ideaboard-auth -f
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
4. 保留原有 `/opt/ideaboard/apps/api/.env` 与 `/opt/ideaboard/apps/auth/.env`。
5. 在 `/opt/ideaboard/apps/auth` 执行 `npm ci && npm run migrate:dist && npm prune --omit=dev`。
6. 执行 `systemctl restart ideaboard-auth ideaboard`。
7. 再次执行健康检查与页面验证。

升级时不要删除：

- `/opt/ideaboard/apps/api/.env`
- `/opt/ideaboard/apps/api/uploads/`
- `/opt/ideaboard/apps/auth/.env`

### 回滚

如果新版本异常：

1. 回到 `/opt/ideaboard/releases/` 中上一个版本目录。
2. 重新复制上一版本的 `apps`、`deploy`。
3. 在 `/opt/ideaboard/apps/auth` 执行 `npm ci && npm run migrate:dist && npm prune --omit=dev`。
4. 执行 `systemctl restart ideaboard-auth ideaboard`。
5. 再次验证 `curl http://127.0.0.1:6000/api/health` 与 `curl http://127.0.0.1:6001/api/health`。
