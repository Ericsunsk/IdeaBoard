# <div align="center">IdeaBoard 部署手册</div>

<p align="center">
  <strong>Cloudflare Pages + VPS Docker Compose 的部署、升级与回滚手册。</strong>
</p>

<p align="center">
  目标运行形态：<code>Cloudflare Pages + Cloudflare DNS/Proxy + VPS Reverse Proxy + GHCR + Docker Compose</code>
</p>

<p align="center">
  <code>私有</code>
  <code>闭源</code>
  <code>内部运维文档</code>
</p>

<p align="center">
  <a href="#deployment-profile">部署概览</a> ·
  <a href="#recommended-docker">Docker 部署</a> ·
  <a href="#cloudflare-edge-hardening">Cloudflare 加固</a> ·
  <a href="#operations">日常运维</a> ·
  <a href="#upgrade-and-rollback">升级回滚</a>
</p>

> [!IMPORTANT]
> 本文档仅供 IdeaBoard 授权成员在内部部署与运维场景下使用。项目为闭源私有项目，未经授权不得复制、分发或对外传播。
>
> 账号安全与业务运维的职责边界请同时参考 [auth-operations.md](./auth-operations.md)；PostgreSQL backup / restore 的密钥材料处理规则见 [data-protection.md](./data-protection.md)。

<a id="deployment-profile"></a>
## 部署概览

本文档覆盖当前推荐生产路径：`Cloudflare Pages + Cloudflare DNS/Proxy + VPS Reverse Proxy + GHCR + Docker Compose`。

```mermaid
graph LR
    Browser((浏览器)) --> Pages["Cloudflare Pages<br>app.example.com"]
    Browser --> ApiDomain["api.example.com"]
    ApiDomain --> CFProxy["Cloudflare DNS/Proxy"]
    CFProxy --> EdgeProxy["VPS Reverse Proxy<br>:443"]
    EdgeProxy -- "/api/auth" --> Auth["ideaboard-auth<br>127.0.0.1:6001"]
    EdgeProxy -- "/api /uploads" --> API["ideaboard-api<br>127.0.0.1:6000"]
    Auth --> Postgres[("PostgreSQL volume")]
    API --> Postgres
    API --> NATS["NATS JetStream volume"]
```

生产职责边界：

- Cloudflare Pages 托管 `apps/web/dist`。
- Cloudflare DNS/Proxy 暴露 `api.example.com` 并提供 WAF / Rate Limiting / Cache Rules。
- VPS 宿主机 Nginx 只反代 `/api/*`、`/api/auth/*`、`/uploads/*` 到 Docker Compose 中的 API/Auth。
- Docker Compose 管理 API、Auth、PostgreSQL、NATS JetStream 与 `api-uploads` volume。
- River 任务队列写入 PostgreSQL；JetStream 只承载状态事件、audit 与实时 fanout。

任务 retry 分两层：`taskruntime` 的 `defaultTaskRetryLimit=3` 是 DB/provider failover 的业务 retry 上限；River `MaxAttempts=25` 是 worker panic、process error、临时基础设施故障的队列层 retry。Worker timeout、无容量 backoff 与 `MaxAttempts` 是代码持有的 typed policy；worker concurrency 通过 `RIVER_WORKER_CONCURRENCY` 配置，默认 5、范围 1–64，重启 API/Worker 后生效。NATS retention 固定为 7 天。Admin 只管理 Egress allowlist，按请求读取并在保存后即时生效。

Pages 项目建议配置：

| 项目 | 值 |
|------|----|
| Root directory | `apps/web` |
| Build command | `npm run build` |
| Build output directory | `dist` |
| Environment variable | `NODE_VERSION=22.23.2` |
| Environment variable | `VITE_API_BASE_URL=https://api.example.com/api` |
| Auth public origin | Derived from `VITE_API_BASE_URL` by removing the trailing `/api` |

VPS 侧至少同步：

```bash
# /opt/stacks/ideaboard/docker/.env
APP_BASE_URL=https://app.example.com
API_BASE_URL=https://api.example.com
```

这两个值必须是没有 path、凭证、query 或 fragment 的 http(s) Origin。它们是唯一事实源：`APP_BASE_URL` 用于浏览器跳转与分享链接，`API_BASE_URL` 用于 Stripe Webhook 和本地上传素材；API 启动时会 fail closed 校验。

Cloudflare DNS 配置：

```text
Type: A 或 AAAA
Name: api
Value: VPS 公网 IP
Proxy status: Proxied
```

VPS 防火墙只开放 SSH、80、443。API/Auth 容器只绑定 `127.0.0.1:6000`、`127.0.0.1:6001` 和仅供监控使用的 `127.0.0.1:9091`，不要把 `6000/6001/9091/PostgreSQL/NATS` 暴露到公网。

<a id="cloudflare-edge-hardening"></a>
## Cloudflare 加固

生产 API 域名默认使用 `Proxied`；临时切到 `DNS only` 时，Cloudflare WAF、Rate Limiting、Cache Rules 和源站 allowlist 均不再生效，上线前必须切回并复验。

- Origin 防火墙：VPS 安全组 / `ufw` 只开放 SSH、80、443；80/443 入站只允许 Cloudflare IP ranges 到达源站，或使用等价的源站防火墙策略。
- 真实客户端 IP：宿主机反代层只信任 Cloudflare 来源，并通过 `CF-Connecting-IP` 恢复真实客户端 IP，再向 API/Auth 传递 `X-Real-IP`、`X-Forwarded-For`、`X-Forwarded-Proto`。
- Cache Rules：对 `/api/*`、`/api/auth/*`、SSE 任务状态接口、上传写入接口，以及所有带 `Authorization` 或 session Cookie 的请求设置 bypass cache。`/uploads/*` 可单独按素材访问策略缓存。
- Metrics：API 使用独立 Prometheus listener；compose 默认仅将 `9091` 绑定到宿主机 `127.0.0.1`。监控系统抓取 `/metrics`，Nginx/Cloudflare 不得代理该端口或路径。
- Rate Limiting / WAF：至少覆盖 `/api/auth/*`、管理员接口、上传接口、任务提交接口。对 `POST /api/user/rewards/redeem` 按可信客户端 IP 设置初始 `60 attempts / 10 minutes` coarse limit；Cloudflare 使用 `CF-Connecting-IP`，源站反代仅在请求确由 Cloudflare allowlist 到达时信任该值。该 edge rule 与应用内 per-user `10 attempts / 10 minutes` limiter 共同生效。首次启用建议先用 log / challenge / captcha 灰度观察误伤，再切换到 block。
- Pages 环境隔离：production 与 preview 使用独立 `VITE_API_BASE_URL`、`VITE_PUBLIC_ASSET_BASE_URL`；preview 不连接生产 API/Auth/数据库。
- HTTPS / HSTS：SSL/TLS 使用 `Full (strict)`；Always Use HTTPS 和 HSTS 先灰度，不要一开始启用 `preload` 或 `includeSubDomains`。

<a id="recommended-docker"></a>
## Docker 部署

推荐方式是 CI 构建镜像并推送到 GHCR，VPS 只拉取镜像和启动容器：

```text
GitHub Actions
  -> ghcr.io/<owner>/<repo>/ideaboard-api@sha256:<digest>
  -> ghcr.io/<owner>/<repo>/ideaboard-auth@sha256:<digest>
  -> ghcr.io/<owner>/<repo>/ideaboard-auth-migrate@sha256:<digest>

VPS
  -> ideaboard update --manifest release-manifest.json
```

### 1. 构建并发布镜像

仓库已内置 [docker-images.yml](../../.github/workflows/docker-images.yml)。触发方式：

- push strict SemVer `vMAJOR.MINOR.PATCH` tag
- 在 GitHub Actions 手动运行 workflow，并填写明确的 commit SHA 或 strict SemVer tag `ref`

生产部署不要把 VPS 端 bootstrap 绑定到 `main`；请始终使用明确的发布 tag 或 commit SHA。镜像部署值必须使用 GitHub Actions 输出的 digest 引用。

Docker Images workflow 只消费目标 commit 已成功的 `CI Gate`，然后构建、扫描、签名并发布 immutable `sha-<commit>` images。workflow 会上传 `release-manifest` artifact；strict SemVer tag 发布还会把 `release-manifest.json` 和六份按 `linux/amd64`、`linux/arm64` 拆分的 CycloneDX SBOM 附到同一 GitHub Release。该 manifest 是推荐升级输入，记录本次 workflow 的 `repository` / `tag` / `ref` / `commit` / `run_id`、三张 GHCR digest 镜像引用、各平台 SBOM 文件名与 Cosign OIDC identity。

严格 SemVer tag（例如 `v0.1.0`）还会触发同一 workflow 内的 Production deployment；手动 workflow 可以构建、扫描、签名 artifact，但不会自动进入 Production deployment。deployment job 只下载本次 workflow 生成的 `release-manifest`，在建立 SSH 连接前验证 manifest 的 tag、commit、ref 和三张 image digest，然后把 manifest 通过 stdin 发送给 VPS 上的 restricted forced command。VPS entrypoint 继续负责 checkout 对应 commit、Cosign 验签、backup、migration、health、smoke 和 release state；GitHub Actions 不重新 build、不解析 mutable image tag，也不读取应用运行时 Secret。

### Production 自动部署通道

先在 VPS 上以 `root` 安装由 root 管理的固定 entrypoint：

```bash
install -o root -g root -m 0755 \
  /opt/stacks/ideaboard/scripts/production-deploy-entrypoint.sh \
  /usr/local/libexec/ideaboard-production-deploy
```

为 GitHub Actions 创建一把仅用于 Production deployment 的 Ed25519 key。只把 public key 加入 VPS 的 `/root/.ssh/authorized_keys`，并固定 forced command：

```text
restrict,command="/usr/local/libexec/ideaboard-production-deploy" ssh-ed25519 <public-key> ideaboard-production-deploy
```

`restrict` 会关闭 PTY、port forwarding、agent forwarding 和 X11 forwarding；entrypoint 还会拒绝任何非空的 `SSH_ORIGINAL_COMMAND`。workflow 的 SSH 调用不发送 remote command，只通过 stdin 传输 `release-manifest.json`。不要把 unrestricted 管理员 key 复用为 deployment key。

在 GitHub repository 中创建名为 `production` 的 Environment，并只配置以下连接值：

| 类型 | 名称 | 用途 |
| ---- | ---- | ---- |
| Variable | `VPS_HOST` | VPS 的已验证 hostname 或 IPv4 address |
| Variable | `VPS_SSH_PORT` | SSH port，例如 `5522` |
| Variable | `VPS_DEPLOY_USER` | forced command 所属用户；当前部署为 `root` |
| Secret | `VPS_DEPLOY_PRIVATE_KEY` | 上述专用 Ed25519 private key |
| Secret | `VPS_KNOWN_HOSTS` | 与 `VPS_HOST`、`VPS_SSH_PORT` 精确匹配的 pinned OpenSSH host-key entry |

`VPS_KNOWN_HOSTS` 必须通过现有可信管理通道核对服务器 host-key fingerprint 后填写；不要在首次 deployment 中盲目信任 `ssh-keyscan` 的结果。数据库密码、Better Auth、Stripe、Resend、OAuth、encryption key 和 GHCR runtime credential 继续只保存在 VPS 的 root-only `.env`，不得复制到 GitHub Environment。

发布时创建并 push 严格 SemVer tag：

```bash
git tag v0.1.0 <verified-main-commit>
git push origin v0.1.0
```

Production deployment 使用固定 concurrency group 串行执行，新的发布不会取消正在进行的发布。SSH connection timeout 为 15 秒，远程 deployment 最长运行 25 分钟；失败时 workflow fail closed，并在 GitHub job summary 中记录 tag、commit、三张 image digest、失败阶段和受控 recovery hint。restricted entrypoint 会丢弃部署过程的原始 stdout/stderr，SSH 只返回 allow-listed JSON 结果；GitHub 不接收未脱敏的容器日志或 inspection output。migration 后失败不会自动 restore PostgreSQL，仍按本 runbook 的人工回滚流程处理。

发布路径先推送 immutable SHA image，通过 BuildKit 为 multi-arch image 生成 max-mode provenance 和 SBOM attestation，并生成各平台 durable CycloneDX SBOM；固定版本且经过 repo-local SHA-256 校验的 Grype 分别扫描 `linux/amd64` 与 `linux/arm64`，拦截 `high` 及以上漏洞；Cosign keyless OIDC 对最终 image digest 签名，全部成功后才推广 mutable aliases。Grype 版本与 archive trust anchor 分别由 [`.grype-version`](../../.grype-version) 和 [`.grype-checksums`](../../.grype-checksums) 管理，例外统一维护在 [`.grype.yaml`](../../.grype.yaml)，新增例外时只加窄范围规则并记录原因与复查日期。

`ideaboard update --manifest ...` 默认会在 pull 前执行 Cosign keyless 验签。VPS 必须先安装 `cosign`；离线排障时才临时设置 `COSIGN_VERIFY=false`。手动验签示例：

```bash
cosign verify \
  --certificate-identity "https://github.com/<github-owner>/<github-repo>/.github/workflows/docker-images.yml@refs/heads/main" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "ghcr.io/<github-owner>/<github-repo>/ideaboard-api@sha256:<digest>"
```

### 2. 首次 VPS 初始化

先从目标 GitHub Release 下载同一批次的 `release-manifest.json`，再用明确的 tag 或 commit 初始化：

```bash
TOKEN="your-github-token"
REF="v0.1.0" # 改为目标 tag 或 commit SHA
MANIFEST="/tmp/release-manifest.json"
curl -fsSL \
  -H "Authorization: token ${TOKEN}" \
  -H "Accept: application/vnd.github.raw" \
  "https://api.github.com/repos/Ericsunsk/IdeaBoard/contents/scripts/bootstrap-vps.sh?ref=${REF}" \
  -o /tmp/bootstrap-vps.sh
GITHUB_TOKEN="${TOKEN}" bash /tmp/bootstrap-vps.sh \
  --ref "${REF}" \
  --manifest "${MANIFEST}"
```

`REF` 与 manifest 必须来自同一发布批次。Bootstrap 不接受隐式默认分支或 latest release；缺少 `--ref` 或 `--manifest` 会在 clone、配置写入和部署前直接失败。强制重装可加 `IDEABOARD_FORCE=1`；脚本会备份旧目录并恢复 `docker/.env`。Bootstrap 会在部署前检查 `cosign`，除非你已经在 `docker/.env` 里显式设置 `COSIGN_VERIFY=false`。

至少修改 `/opt/stacks/ideaboard/docker/.env`：

- `APP_BASE_URL`
- `API_BASE_URL`
- `POSTGRES_PASSWORD`
- `API_DB_PASSWORD`
- `AUTH_DB_PASSWORD`
- `AUTH_INTERNAL_KEY`
- `APP_SECRETS_ENCRYPTION_KEY`
- `BETTER_AUTH_SECRET`
- `NATS_PASSWORD`

### 3. 后续部署 / 升级

只允许使用同一发布批次的 manifest：

```bash
ideaboard update --manifest release-manifest.json
```

`ideaboard update --manifest ...` 会执行：

1. 读取 manifest 中的三张 immutable image digest 并写入 `.env`；缺少 manifest 会 fail closed。
2. 校验当前 stack checkout 与 manifest 的完整 commit SHA 一致；拒绝 mutable tag/latest，不从 registry tag 反查 digest。
3. 创建 `.env` snapshot 与 `docker/releases/history/*.json` 发布记录。
4. 执行 preflight、Cosign 验签、pull、backup、migration、重启与 smoke gate。
5. 成功后写入 `docker/releases/current.json`，并把上一版保存为 `docker/releases/previous.json`。

Migration 前失败会恢复 `.env` snapshot；migration 后失败不会自动 restore DB，但会输出 release id、失败阶段、backup 路径和 previous digest。

自动 Production entrypoint 会先 checkout manifest commit。手动更新时也必须先让 `/opt/stacks/ideaboard` 指向同一 commit；不一致时部署会在 pull、backup 和 migration 前失败。需要重新铺设 checkout 时，使用同一 `REF` 与 manifest 重新运行 `bootstrap-vps.sh --force`，不要让部署脚本在执行过程中自更新。

### 4. Backup Hardening

> [!IMPORTANT]
> PostgreSQL backup 包含 `api_account_secrets`、`storage_profile_secrets` 等密钥表，可能含有上游 API key、S3 access key/secret、endpoint 与 bucket 配置。`postgres-*.dump[.age|.gpg]` 必须按 secret artifact 处理。

- 每个 PostgreSQL 与 uploads backup 会生成同名 `.sha256` 文件，例如 `postgres-ideaboard-YYYYMMDD-HHMMSS.dump.sha256`、`postgres-ideaboard-YYYYMMDD-HHMMSS.dump.age.sha256`、`api-uploads-YYYYMMDD-HHMMSS.tar.gz.sha256`。
- 设置 `BACKUP_ENCRYPTION_RECIPIENT` 后，backup 会先加密再为最终落盘 artifact 生成 checksum。`age1...`、`ssh-*` 或 `age:<recipient>` 使用 age；`gpg:<recipient>` 或其他值按 GPG recipient 处理。
- `ideaboard doctor` 会展示最近 backup 的路径、`checksum=valid|invalid|missing|unchecked`、是否加密，以及最近 release 记录中的 backup 阶段状态。
- 当前 DB restore 仍需人工确认；脚本不会在 migration 后失败时自动回滚数据库。

当前 Docker stack 中唯一的关系型数据库是 PostgreSQL；stack 同时管理 API、Auth、NATS JetStream 与 uploads volume。首次部署或切换部署形态前，请先确认目标 PostgreSQL 中已有需要保留的数据，或明确按空库初始化。

<a id="api-database-baseline"></a>

### 5. API 业务库 baseline

`ideaboard` 是固定 schema owner；`ideaboard_api` 与 `ideaboard_auth` 是彼此独立的 limited runtime roles。三个 roles 必须使用不同密码；长期运行的 API/Auth 容器只接收各自 runtime credential，只有一次性的 `api-migrate` / `auth-migrate` 容器接收 owner credential。

空 PostgreSQL 库由 API migration 以 `20260814000000_baseline.sql` 初始化业务 schema，并写入 version、filename、SHA-256 checksum 和 applied timestamp；API runtime 不再自动迁移。Docker 部署会先运行 `api-migrate`，授予 runtime 常规业务表 DML，并将 Credits Ledger 收紧为只读 + typed Credit Commands；再运行 Auth migration，为 Auth runtime role 授予 Better Auth tables DML 与 `get_auth_runtime_settings()` 的 `EXECUTE`，最后启动服务。`DB_MIGRATION_USER` / `DB_MIGRATION_PASSWORD` 只注入一次性的 migration process，不得进入长期运行的 API/Auth runtime；本地示例见 `docker/migration.env.example`。

Schema 变更必须经过同一条 Schema Release lifecycle：在 `apps/api` 中修改 domain fragment 或 manifest 后运行 `BASE_SHA=HEAD API_TEST_DATABASE_DSN='<test-dsn>' go run ./cmd/schema-release --write-artifact` 生成 `internal/db/schema/schema.sql`，提交 snapshot 与 append-only migration；发布前和 CI 使用相同 command 但不带 `--write-artifact`，以只读方式检查 manifest assembly、snapshot drift、sqlc generation、migration immutability 和 PostgreSQL catalog semantics。不要调用已移除的旧 schema assembly 或 migration history scripts。

非空数据库不能直接套用该 baseline。若你从其他形态迁入数据，标准做法是先导出需要保留的数据，再在空库上初始化当前 baseline，最后用受控脚本导入业务数据；不要手动修改 `schema_migrations` 绕过 baseline 校验。

当前 API migration 保护会拒绝四类风险状态：未知历史 migration 版本；历史 filename/checksum 与当前 embedded migration 不一致；baseline 未应用但 schema 已有 API-owned 业务表；baseline 已标记应用但当前 schema 缺少 baseline 业务表。`schema_migrations` 由 database trigger 强制 append-only。Better Auth 的认证表可以先存在，不计入 API baseline 冲突；不过标准部署顺序仍是 API baseline first、Better Auth migration second。Clean-break 后禁止只回滚 API/Auth image；需要同时恢复数据库 backup 或重新初始化 Fresh Database。

PostgreSQL init script 只在空 volume 创建 API/Auth runtime roles。项目尚未上线时若本地 volume 早于该角色分离方案，直接删除并重建开发 volume；不要把 owner credential 配回任何长期运行的应用 runtime。

### 6. 初始化管理员

```bash
docker compose -f /opt/stacks/ideaboard/docker/compose.yml --env-file /opt/stacks/ideaboard/docker/.env run --rm \
  -e INIT_ADMIN_EMAIL='admin@example.com' \
  -e INIT_ADMIN_DISPLAY_NAME='Admin' \
  -e INIT_ADMIN_PASSWORD='your-strong-password' \
  api init-admin
```

### 6.1 Auth Email 上线门禁

Fresh Database 默认关闭公开注册，且邮件 readiness 为 `not_configured`。按以下顺序上线，不能先开放注册：

1. 保持公开注册关闭，确认 API 与 Auth 使用同一份稳定的 `APP_SECRETS_ENCRYPTION_KEY`。更换该 key 会导致已保存的 Resend API Key 无法解密。
2. 在 Resend 完成发件 Domain verification，并创建仅通过部署 Secret 注入或 Admin 表单提交的 production API Key；不要把明文写入仓库、Issue 或日志。
3. 使用 `init-admin` 创建的账号通过 Email + Password 登录 Admin。该登录不依赖 Resend readiness。
4. 在 Admin `系统设置` 保存 Resend API Key、发件人和 Reply-To，然后发送 Test Email。只有配置摘要显示 `readiness=ready` 且收件箱确认收到邮件，才算邮件链路通过。
5. 依次 smoke 注册 OTP 自动登录、Email + Password 登录、Password reset、Email change 和 Session revoke；最后按运营计划开放公开注册。

邮件未达到 `ready` 时，注册、Password reset 和 Email change 必须返回不可用；已有 Email + Password 登录必须继续可用。Test Email 失败时保持公开注册关闭，修正 Domain/API Key 后重新测试。

Auth rollback 不删除身份或业务数据：先关闭公开注册，再清除或替换 Resend 配置。此时 Email workflow fail closed，但已有 Email + Password 登录保持可用。恢复时重新保存配置并完成 Test Email，再决定是否开放注册。

### 7. VPS 反代

#### Nginx Proxy Manager

Nginx Proxy Manager 与 IdeaBoard 运行在同一 Docker host 时，使用仓库提供的 Compose override，不要修改 canonical `docker/compose.yml`：

```bash
docker network inspect npm-nw >/dev/null 2>&1 || docker network create npm-nw
```

在 `/opt/stacks/ideaboard/docker/.env` 设置：

```text
IDEABOARD_COMPOSE_OVERRIDE_FILE=docker/compose.npm.yml
NPM_NETWORK_NAME=npm-nw
```

随后使用目标 release manifest 执行 `ideaboard update --manifest release-manifest.json`。部署脚本会把 `compose.npm.yml` 叠加到 canonical Compose 配置，使 API/Auth 同时保留 IdeaBoard 内网并加入 NPM external network。NPM 的 Forward Hostname / IP 使用 `ideaboard-api`，Forward Port 使用 `6000`；不要使用容器自身的 `127.0.0.1`。PostgreSQL 与 NATS 只保留在 IdeaBoard 内网。

取消 NPM 接入时，先切换反代流量，再从 `.env` 删除上述两个变量并执行 `ideaboard up`。Docker 会按新的目标配置重建 API/Auth，canonical Compose 文件不需要修改。

#### 宿主机 Nginx

生成 Nginx 配置：

```bash
ideaboard nginx-conf /tmp/ideaboard.nginx.conf
sudo cp /tmp/ideaboard.nginx.conf /etc/nginx/sites-available/ideaboard.conf
sudo ln -sf /etc/nginx/sites-available/ideaboard.conf /etc/nginx/sites-enabled/ideaboard.conf
sudo nginx -t
sudo systemctl reload nginx
```

如果 VPS 已经有 certbot 或自管的 `443 ssl` server block，可以把生成文件里的 `/api`、`/api/auth`、`/uploads` location 合并进现有 API 域名 server block。

默认 upstream 是宿主机 loopback 端口：

```text
NGINX_API_UPSTREAM=http://127.0.0.1:6000
NGINX_AUTH_UPSTREAM=http://127.0.0.1:6001
NGINX_AUTH_HEALTH_UPSTREAM=http://127.0.0.1:6001/api/health
```

如需明确绑定 API 域名，可设置 `NGINX_SERVER_NAME=api.example.com` 后重新执行 `ideaboard nginx-conf`。

如果 API 域名使用 Cloudflare `Proxied`，并且 VPS 防火墙已限制 80/443 只接受 Cloudflare IP ranges，可以生成真实客户端 IP include：

```bash
ideaboard cloudflare-real-ip-conf /tmp/cloudflare-real-ip.conf
sudo cp /tmp/cloudflare-real-ip.conf /etc/nginx/cloudflare-real-ip.conf
```

然后在 `/opt/stacks/ideaboard/docker/.env` 设置：

```text
NGINX_REAL_IP_INCLUDE=/etc/nginx/cloudflare-real-ip.conf
```

重新执行 `ideaboard nginx-conf` 并 reload Nginx 后，反代会通过 `CF-Connecting-IP` 恢复真实客户端 IP。不要在源站仍允许非 Cloudflare 公网直连时启用该 include。

### 8. 验证

```bash
docker compose -f /opt/stacks/ideaboard/docker/compose.yml --env-file /opt/stacks/ideaboard/docker/.env ps
ideaboard smoke
curl http://127.0.0.1:6000/api/health
curl http://127.0.0.1:6001/api/health
curl "${API_BASE_URL}/api/health"
curl "${API_BASE_URL}/api/auth/get-session"
```

`/api/auth/get-session` 未登录时返回未认证是正常的；关键是宿主机反代服务能转发到 Auth Service。`ideaboard smoke` 会检查 PostgreSQL、NATS JetStream、本机 API/Auth health、River schema、默认 JetStream event stream，以及公网 `${API_BASE_URL}/api/health`。

对象存储 live smoke 默认不依赖外部凭据；缺少 env 时会自动 skip。需要在隔离的 R2/S3-compatible bucket 上验证 upload/read/public URL/delete/profile reload contract 时，只通过 shell env 或 CI secret 注入凭据，不要把 secret 写入仓库、issue、日志或未加密 artifact：

```bash
cd apps/api
API_TEST_S3_ENDPOINT='<account>.r2.cloudflarestorage.com' \
API_TEST_S3_ACCESS_KEY_ID='...' \
API_TEST_S3_SECRET_ACCESS_KEY='...' \
API_TEST_S3_BUCKET='ideaboard-smoke' \
API_TEST_S3_ACCESS_DOMAIN='https://uploads.example.com' \
API_TEST_S3_REGION='auto' \
API_TEST_S3_FORCE_PATH_STYLE='false' \
go test ./internal/media/storage -run TestS3CompatibleUploadReadPublicDeleteSmoke -count=1
```

该 smoke 会在 `uploads/smoke/s3-compatible/` 下创建临时 PNG object，验证 SDK read、公开访问 URL、重新加载 profile 后读取、删除后不可读，并在失败时尽量清理临时 object。`API_TEST_S3_FORCE_PATH_STYLE` 仅在目标 endpoint 要求 path-style 时设为 `true`。

### 9. Stripe Test Mode smoke

Stripe smoke **默认不运行**，CI 也不得因它依赖真实 Stripe。只允许在隔离的 Stripe Test Mode Account、专用测试用户和可回收的测试订单上显式 opt-in；不要向生产 Account、Live API Key 或真实用户执行该流程。

执行前确认 Test Mode 的 Webhook URL 指向隔离环境，并仅从 shell env 或 CI secret 注入 `sk_test_*` Key。`API_TEST_STRIPE_SMOKE=1` 是唯一的 opt-in 开关；缺少它或缺少任一 Stripe 凭据时，smoke 必须 skip，而不是 fallback 到 Live：

```bash
cd apps/api
API_TEST_STRIPE_SMOKE=1 \
API_TEST_STRIPE_SECRET_KEY='sk_test_...' \
API_TEST_STRIPE_BASE_URL='https://staging-api.example.com' \
API_TEST_STRIPE_TAX_CODE='txcd_10105001' \
API_TEST_STRIPE_PAYMENT_METHOD_CONFIGURATION_ID='pmc_...' \
go test ./internal/payments -run TestStripeTestModeSmoke -count=1
```

通过标准：Checkout request 包含 `automatic_tax[enabled]=true`、`adaptive_pricing[enabled]=true`、Product Tax Code、`IDEABOARD` Statement Descriptor suffix，且未声明静态 `payment_method_types`（由 Stripe Dynamic Payment Methods 决定）；Webhook Endpoint 使用当前 API version 并订阅 Payments 支持的事件。验证完成后在 Stripe Dashboard 删除测试 Checkout/Customer，或记录其可回收范围。任何测试失败都保持 `purchases_enabled=false`，先修复配置或切回旧配置后重试。

### 10. Stripe Live launch gate

在首次设置 `purchases_enabled=true` 前，由负责人员逐项确认并留下执行记录。以下任一项不满足时，不得开启充值：

1. Stripe Account 是已验证、可收款的香港个人 Account，`charges_enabled=true`，并确认目标业务、结算币种和地区符合 Stripe 当前要求。
2. 仅使用 Live **Restricted API Key**；不要把 Secret Key、Webhook Secret 或其明文写入仓库、Issue、日志、截图、浏览器 console 或未加密 artifact。
3. Live Webhook Endpoint 为公开 HTTPS 的 `/api/webhooks/stripe`，使用当前 API version，已保存其独立 Webhook Secret，并在 Stripe Dashboard 成功投递一次受控测试事件。
4. Idea Product 已设置经税务确认的 Product Tax Code；Checkout 显示的 Statement Descriptor 与 `IDEABOARD` suffix 一致。
5. Stripe Tax 已启用，并逐个确认经营/收款地区的 Tax registrations。**当 Live Tax registrations 为 0 时这是硬阻断：保持 `purchases_enabled=false`，不得声称已自动代收税，也不得仅凭 Automatic Tax 参数上线。** 税务结论变化后重新完成本清单。
6. 在目标客户地区确认 WeChat Pay 在该 Live Account、币种和支付流中可用；它由 Stripe Dynamic Payment Methods 决定，不在应用中硬编码或承诺一定展示。
7. 用隔离的 Live 前置环境或 Stripe Dashboard 的受控最小金额验证 Checkout、Webhook、paid credit、退款和 Dispute 回收路径；不得用真实客户订单替代验收。

通过后才由管理员保存 Stripe 配置并开启充值。配置准备失败会保留旧配置；如 Live 验收、税务确认或 provider health 出现问题，先关闭充值作为回滚入口，再处理 Stripe Dashboard 与 River 对账。

<a id="operations"></a>
## 日常运维

常用命令：

```bash
ideaboard preflight
ideaboard doctor
ideaboard doctor --deep
ideaboard status
ideaboard logs --tail=200 api auth postgres nats
ideaboard restart
ideaboard stop
```

删除 stack：

```bash
ideaboard delete --yes
```

`delete` 默认保留 Docker volumes；同时删除数据加 `--volumes`。

### Payments 事故处理

需要立即停止充值时，在 Admin Payments 设置将 `purchases_enabled` 设为 `false`。新的 Checkout 会被拒绝；River 会异步 Expire 仍有效的待支付 Stripe Checkout Session，重复执行安全幂等。不要删除 Payment Order、Refund、Dispute、Ledger 或 Webhook Inbox 来“取消”支付。

随后按以下顺序恢复：

1. 在 Stripe Dashboard 处理退款、Dispute、Webhook delivery 或账户能力问题，保留 provider object ID 作为关联依据。
2. 使用 Admin Payment Order/Dispute 读模型查看未收敛订单、退款 anomaly 和 Dispute 状态；不要把 Dashboard 状态手工写入 PostgreSQL。
3. 保持 River worker 运行，等待其对账重试；必要时重启 API/Worker 后确认 River job、PostgreSQL 和 Stripe 配置都健康。对账以 canonical Stripe Retrieve 收敛，Webhook 和 SSE 都不是支付事实源。
4. 仅当未结算订单、Refund 与 Dispute 已按预期收敛，并重新完成相关 smoke 后，才重新开启充值。

多实例部署中，Stripe 配置在每次请求时从 PostgreSQL 读取，Webhook 和 River 也通过 durable state/claim 协作；SSE 只提示本实例已提交的变更，客户端必须重新读取订单、Wallet 或历史。不要为此引入 WebSocket、Redis 或新的任务队列。

排障优先顺序：

1. `ideaboard doctor`
2. `ideaboard logs --tail=200 api auth postgres nats`
3. `ideaboard doctor --deep`
4. 检查 `/opt/stacks/ideaboard/docker/releases/current.json` 与 `previous.json`
5. 检查 `/opt/stacks/ideaboard-backups/`

<a id="rewards-operations-and-rollback"></a>
## Rewards 运维与回滚

### HMAC two-stage rotation

`REWARDS_CODE_HMAC_KEYS` 只保留 `current` 与可选 `previous`。轮换时不要直接删除旧 key：

1. 生成新的高熵 key 与唯一 version，在 secret 管理边界内把新 key 配为 `current`、原 key 配为 `previous`，然后部署 API。不要把 key 明文写入日志、Issue 或未加密 artifact。
2. 确认 API 启动成功，并用受控 RewardCode 验证新 Code 可创建和兑换、旧 Code 仍可兑换或 replay。重叠期间新 Code 只使用 `current`，lookup 同时接受 `current` 与 `previous`。
3. 只要任何 persisted RewardCode 仍使用旧 version，就保持重叠；确认不存在依赖旧 version 的记录后，才可移除 `previous` 并重新部署。
4. API startup 会 fail closed：若任何 persisted RewardCode 依赖未配置 key，进程拒绝启动。此时恢复上一份 secret 配置（新 key 为 `current`、旧 key 为 `previous`）并重新启动；不要通过改表绕过检查。

### Route disable 与回滚

Rewards 没有 Feature Flag。发生需要立即停止兑换或管理写入的事故时：

1. 先在 Cloudflare 或 reverse proxy 阻断 `/api/user/rewards/*` 与 `/api/admin/rewards/*`，保留 health endpoint，并确认外部请求已被 gateway 拒绝。
2. 再执行 `ideaboard rollback --to previous` 回滚 image，完成 API/Auth health、登录、Credits 与非 Rewards 核心路径 smoke；如回滚版本仍包含 Rewards routes，继续保留 gateway block，直到修复版本上线并通过验证。
3. 确认修复版本稳定后再移除 gateway block。不要为事故回滚临时引入 Feature Flag、兼容 endpoint 或 dual write。

一旦存在 Redemption，禁止执行会删除 Rewards 数据的 destructive down migration，也禁止删除对应 Credits Ledger facts。应用回滚保留 Rewards schema 与历史事实；旧 image 可以暂不使用这些表。DB restore 是独立、人工确认的操作，不随 image rollback 自动执行。

<a id="upgrade-and-rollback"></a>
## 升级与回滚

升级：

```bash
ideaboard update --manifest release-manifest.json
```

运行镜像回滚：

```bash
ideaboard rollback --to previous
```

脚本会把 previous manifest 中的三张 image digest 写回 `.env`、重新 pull/验签、启动服务并跑 smoke gate。DB restore 不自动执行，只有在确认 migration 需要回退数据时再人工按 backup 恢复。

确认最近 backup：

```bash
ideaboard doctor
ls -lh /opt/stacks/ideaboard-backups/
docker compose -f /opt/stacks/ideaboard/docker/compose.yml --env-file /opt/stacks/ideaboard/docker/.env logs --tail=200 api auth postgres nats
```

受控 restore drill 建议至少记录：

1. 在隔离环境复制目标 backup、对应 `.sha256` 和必要的解密 key；不要直接在生产库上演练。
2. 对最终落盘 artifact 执行 `sha256sum -c <backup>.sha256`；如果 backup 已加密，校验通过后再用 age 或 gpg 解密到受控临时目录。
3. 对解密后的 PostgreSQL dump 执行 `pg_restore --list`，对 uploads tar 执行 `tar -tzf`。
4. 恢复前确认目标库可被覆盖、当前 migration 影响已评估，并记录是否需要同时恢复 uploads backup。
5. Restore 后运行 API/Auth health、关键登录路径和对象存储读取 smoke；涉及密钥表时按影响范围轮换上游 API key 与对象存储凭据。
6. 演练结束后清理解密后的 dump、tar、临时目录和本地下载副本。

数据保存在 Docker volumes：`postgres-data`、`nats-data`、`api-uploads`。不要使用 `docker compose down -v`，除非你明确要删除生产数据。
