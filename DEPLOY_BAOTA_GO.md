# <div align="center">IdeaBoard Baota Go Deployment Guide</div>

<p align="center">
  <strong>适用于宝塔面板 Go 项目模式的部署手册。</strong>
</p>

<p align="center">
  前后端资源内嵌进单个 Go 二进制，由宝塔负责进程托管与反向代理。
</p>

<p align="center">
  <code>Private</code>
  <code>Closed Source</code>
  <code>Internal Operations Document</code>
</p>

<p align="center">
  <a href="#deployment-profile">部署概览</a> ·
  <a href="#artifact-shape">发布形态</a> ·
  <a href="#setup-flow">部署流程</a> ·
  <a href="#validation">验证</a> ·
  <a href="#upgrade-and-rollback">升级回滚</a>
</p>

---

> [!IMPORTANT]
> 本文档仅供 IdeaBoard 授权成员在内部部署与运维场景下使用。项目为闭源私有项目，未经授权不得复制、分发或对外传播。

## Deployment Profile

本文档适用于前后端打成单可执行文件后，通过宝塔面板以 Go 项目方式运行的场景。

### At a Glance

| 项目 | 当前约定 |
|------|----------|
| 发布包名 | `ideaboard-bt-go-<tag>-linux-amd64` |
| 部署目录 | `/www/wwwroot/ideaboard` |
| 可执行文件 | `app/ideaboard-server` |
| 默认端口 | `6000` |
| 静态资源 | 已内嵌到 Go 二进制 |

## Artifact Shape

宝塔方案由以下脚本生成：

```bash
./scripts/package-bt-go.sh
```

输出结构示例：

```text
release/v0.1.0/ideaboard-bt-go-v0.1.0-linux-amd64/
├── app/
│   └── ideaboard-server
└── .env
```

该方案会将前端静态资源内嵌到 Go 二进制中，部署目录只需保留：

- `app/ideaboard-server`
- `.env`

## Setup Flow

### 1. 本地打包

```bash
./scripts/package-bt-go.sh
```

或显式指定版本：

```bash
./scripts/package-bt-go.sh v0.1.0
```

### 2. 上传到服务器

将 `release/<tag>/ideaboard-bt-go-<tag>-linux-amd64/` 上传到服务器，例如：

```text
/www/wwwroot/ideaboard/
├── app/
│   └── ideaboard-server
└── .env
```

### 3. 配置环境变量

编辑 `.env`，至少确认：

- `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`
- `REDIS_HOST/REDIS_PORT/REDIS_AUTH`
- `APP_PORT=6000`
- `JWT_SECRET`

完整变量说明见 [DEVELOPMENT.md](./DEVELOPMENT.md)。

### 4. 在宝塔中创建 Go 项目

推荐配置：

- 项目目录：`/www/wwwroot/ideaboard`
- 启动命令：`./app/ideaboard-server`
- 监听端口：`6000`

说明：

- 前端已内嵌到二进制，不需要单独配置静态站点目录。
- 反向代理仍建议由宝塔的 Nginx 站点承担。

### 5. 初始化管理员账号

在服务器目录执行：

```bash
cd /www/wwwroot/ideaboard
set -a
source ./.env
set +a
./app/ideaboard-server init-admin
```

默认管理员账号：

- 用户名：`admin`
- 密码：`admin123`

> [!IMPORTANT]
> `.env` 中的数据库账号只用于后端连接 MySQL，不是网站管理员登录账号。管理员登录信息来自业务库中的 `users` 表；首次部署后建议立即修改默认管理员密码。

## Validation

```bash
curl http://127.0.0.1:6000/api/health
```

预期返回：

```json
{"code":0,"data":{"status":"ok"},"message":"success"}
```

部署后至少确认：

1. 宝塔 Go 项目状态为运行中。
2. `/api/health` 返回成功。
3. 页面可正常访问且不出现静态资源 404。
4. 管理员可以登录。
5. 任务提交流程正常。

## Upgrade and Rollback

### Upgrade

1. 使用新版本重新执行 `./scripts/package-bt-go.sh`。
2. 上传新的 `app/` 与 `.env` 模板到服务器。
3. 保留原有 `.env` 的实际配置值。
4. 在宝塔中重启 Go 项目。
5. 重新执行健康检查。

### Rollback

如果新版本有问题：

1. 用上一个稳定版本的 `app/ideaboard-server` 覆盖当前二进制。
2. 保留现有 `.env`。
3. 在宝塔中重启 Go 项目。
4. 再次检查 `/api/health`。
