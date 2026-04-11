# 宝塔 Go 项目部署（前后端单可执行）

本方案会把前端静态资源内嵌到 Go 后端，最终发布产物只保留两项：

- `app/`（可执行目录，内含 `feng-server`）
- `.env`（配置文件）

默认端口：`6000`。

## 1. 本地打包

在项目根目录执行：

```bash
./scripts/package-bt-go.sh
```

可选指定版本号：

```bash
./scripts/package-bt-go.sh v0.1.0
```

打包结果示例：

```text
release/v0.1.0/feng-bt-go-v0.1.0-linux-amd64/
├── app/
│   └── feng-server
└── .env
```

## 2. 上传到服务器

将 `release/<tag>/feng-bt-go-<tag>-linux-amd64/` 上传到服务器，例如：

```text
/www/wwwroot/feng/
├── app/
│   └── feng-server
└── .env
```

## 3. 修改配置

编辑 `.env`，至少确认以下字段：

- `DB_HOST/DB_PORT/DB_USER/DB_PASSWORD/DB_NAME`
- `REDIS_HOST/REDIS_PORT/REDIS_AUTH`
- `APP_PORT=6000`
- `JWT_SECRET`

## 4. 宝塔创建 Go 项目

在宝塔面板创建 Go 项目时，填写：

- 项目目录：`/www/wwwroot/feng`
- 启动命令：`./app/feng-server`
- 监听端口：`6000`

说明：前端已内嵌到二进制，无需额外 Nginx 静态目录。

## 5. 验证

```bash
curl http://127.0.0.1:6000/api/health
```

预期包含：

```json
{"code":0,"data":{"status":"ok"},"message":"success"}
```
