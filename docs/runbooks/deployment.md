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
    Browser((浏览器)) --> Gateway["ideaboard.art<br>same-origin Gateway"]
    Gateway --> CFProxy["Cloudflare DNS/Proxy"]
    CFProxy --> EdgeProxy["VPS Reverse Proxy<br>:443"]
    EdgeProxy -- "/api/auth" --> Auth["ideaboard-auth<br>127.0.0.1:6001"]
    EdgeProxy -- "/api" --> API["ideaboard-api<br>127.0.0.1:6000"]
    Auth --> Postgres[("PostgreSQL volume")]
    API --> Postgres
    API --> NATS["NATS JetStream volume"]
```

生产职责边界：

- Cloudflare Pages 托管 `apps/web/dist`，并由 `ideaboard.art` Gateway 暴露应用。
- Cloudflare DNS/Proxy 暴露 `ideaboard.art` 并提供 WAF / Rate Limiting / Cache Rules。
- VPS 宿主机 Nginx 反代同域 `/api/*`、`/api/auth/*` 到 Docker Compose 中的 API/Auth。媒体通过 Media Asset、public avatar/logo API 返回图片字节；imgproxy 只接受 API 内部的签名渲染请求。
- Docker Compose 管理 API（http-only）、API Worker（worker-only）、Auth、PostgreSQL、NATS JetStream 与 `api-uploads` volume。`api` 与 `api-worker` 是同一镜像、同一数据库的双进程（ADR-0021，`API_MODE` 区分），不是微服务。
- River 任务队列写入 PostgreSQL；JetStream 只承载状态事件、audit 与实时 fanout。全部 River 队列由 `api-worker` 消费（含 drawing/GPU 长耗时、`media_reclamation`、sweep、对账），`api` 只负责 API/SSE 与事务内入队；periodic 调度只跑在 worker 侧。`api-worker` 保持单副本，水平扩展只加 `api`。

任务 retry 分三层：网关内部重试（单请求最多 5 次，Cloudflare 侧 fallback 链）；网关耗尽后 transient 失败（429/5xx/超时/网络）允许最多 2 次 worker 层重投（reset + River backoff 整任务重投，新 token，以 `tasks.retry_count` 为界，见 ADR-0010）；River `MaxAttempts=25` 是 worker panic、process error、临时基础设施故障的队列层 retry（整任务重投）。Worker timeout（网关 300s/worker 360s/River 17m 嵌套）与 `MaxAttempts` 是代码持有的 typed policy；worker concurrency 通过 `RIVER_WORKER_CONCURRENCY` 配置，默认 5、范围 1–64，重启 API/Worker 后生效。NATS retention 固定为 7 天。Admin 只管理 Egress allowlist（AI 出口唯一是 Cloudflare AI Gateway，图片下载经 safehttp 做 SSRF 防护），按请求读取并在保存后即时生效。

Pages 项目建议配置：

| 项目                    | 值                                                                                         |
| ----------------------- | ------------------------------------------------------------------------------------------ |
| Root directory          | `apps/web`                                                                                 |
| Build command           | `npm run build`                                                                            |
| Build output directory  | `dist`                                                                                     |
| Environment variable    | `NODE_VERSION`（与根目录 `.node-version` 一致）                                            |
| Environment variable    | Leave `VITE_API_BASE_URL` and `VITE_PUBLIC_ASSET_BASE_URL` empty (same-origin Gateway)     |
| Pages Function variable | `IDEABOARD_API_ORIGIN=<current VPS API HTTPS origin>` (required; no retired-host fallback) |
| Auth public origin      | Derived from the current browser origin (`ideaboard.art`)                                  |

Infisical `prod` 至少配置：

```dotenv
PUBLIC_ORIGIN=https://ideaboard.art
```

`PUBLIC_ORIGIN` 必须是没有 path、凭证、query 或 fragment 的 http(s) Origin。`PUBLIC_ORIGIN` 是 canonical source，用于浏览器跳转与分享链接；后端公开 API 地址恒等于 `PUBLIC_ORIGIN`（`API_BASE_URL` 已删除，无 override，原用于 Stripe Webhook 和服务间 API 地址的独立 host 场景已收敛到同域 Gateway）。浏览器请求必须通过 `ideaboard.art` 的 same-origin Gateway，API 启动时会 fail closed 校验。

Cloudflare DNS 配置：

```text
Type: A 或 AAAA
Name: @
Value: VPS 公网 IP（`ideaboard.art` Gateway）
Proxy status: Proxied
```

VPS 防火墙只开放 SSH、80、443。API/Auth 容器只绑定 `127.0.0.1:6000`、`127.0.0.1:6001` 和仅供监控使用的 `127.0.0.1:9091`，不要把 `6000/6001/9091/PostgreSQL/NATS` 暴露到公网。

<a id="cloudflare-edge-hardening"></a>

## Cloudflare 加固

生产 Gateway 域名默认使用 `Proxied`；临时切到 `DNS only` 时，Cloudflare WAF、Rate Limiting、Cache Rules 和源站 allowlist 均不再生效，上线前必须切回并复验。

- Origin 防火墙：VPS 安全组 / `ufw` 只开放 SSH、80、443；80/443 入站只允许 Cloudflare IP ranges 到达源站，或使用等价的源站防火墙策略。
- 真实客户端 IP：宿主机反代层只信任 Cloudflare 来源，并通过 `CF-Connecting-IP` 恢复真实客户端 IP，再向 API/Auth 传递 `X-Real-IP`、`X-Forwarded-For`、`X-Forwarded-Proto`。
- Cache Rules：对 `/api/*`、`/api/auth/*`、SSE 任务状态接口、上传写入接口，以及所有带 `Authorization` 或 session Cookie 的请求设置 bypass cache。图片响应的缓存策略由 API 的 `Cache-Control` 决定，网关不得延长缓存。
- Metrics：API 使用独立 Prometheus listener；compose 默认仅将 `9091` 绑定到宿主机 `127.0.0.1`。监控系统抓取 `/metrics`，Nginx/Cloudflare 不得代理该端口或路径。
- Rate Limiting / WAF：至少覆盖 `/api/auth/*`、管理员接口、上传接口、任务提交接口。对 `POST /api/v1/workspaces/{workspace_id}/rewards/redemptions` 按可信客户端 IP 设置初始 `60 attempts / 10 minutes` coarse limit；Cloudflare 使用 `CF-Connecting-IP`，源站反代仅在请求确由 Cloudflare allowlist 到达时信任该值。该 edge rule 与应用内 per-user `10 attempts / 10 minutes` limiter 共同生效。首次启用建议先用 log / challenge / captcha 灰度观察误伤，再切换到 block。
- Pages 环境隔离：production 的 `VITE_API_BASE_URL`、`VITE_PUBLIC_ASSET_BASE_URL` 必须为空并使用同域 Gateway；preview 不连接生产 API/Auth/数据库。
- HTTPS / HSTS：SSL/TLS 使用 `Full (strict)`；Always Use HTTPS 和 HSTS 先灰度，不要一开始启用 `preload` 或 `includeSubDomains`。

<a id="recommended-docker"></a>

## Docker 部署

CI 构建镜像，验证源码与构建来源，生成完整签名部署包；VPS 验签后运行包内 Compose 与部署脚本。VPS 不保存 Git repository、应用源码、Dockerfile 或 GitHub source token。

```text
CI: same-SHA CI Gate → component fingerprint/reuse → scan/sign images
    → source/provenance verification → deployment-bundle.tar → Cosign sign-blob
VPS: installed verifier → strict signature/content checks → versioned bundle
     → Infisical → image signature checks → backup/migration/health/smoke
```

### 1. 构建并发布

仓库内置 [docker-images.yml](../../.github/workflows/docker-images.yml)：push strict SemVer `vMAJOR.MINOR.PATCH` tag 发布并部署 Production；手动运行 workflow 时必须填写明确的 commit SHA 或 strict SemVer tag，手动构建不生成 Production 部署包。

发布只复用同一 source commit 在 canonical `ci.yml`、`main` push 中成功完成的 `CI Gate`，同时验证 repository、event、workflow path、SHA 与 gate result。main CI 按受影响 context 验证；未知影响或可信基线不可用时全量执行。

每个 component 的 build-input fingerprint 覆盖 tracked build context、Dockerfile/ignore、target/platform 和构建步骤，Auth/Realtime 还覆盖 canonical Node pin。只有上一成功发布的 source run、tag→commit、ancestry、相同输入与精确 Cosign signer identity 都可信时，才复用 immutable digest；证据不足则重新构建。五张 image 全部通过最新 Grype database 的 High/Critical gate 后生成 schema v2 `release-manifest.json`。新建 image 使用 keyless OIDC 签名；复用 image 保留原始 signer。当前 Production 使用 `linux/arm64`。BuildKit provenance 与 SBOM 保存在 image attestations。

Component fingerprint 的 v3 policy 连续覆盖 workflow 全局配置、preflight、checkout/setup 与真实 build，排除 build 完成后的 scan、签名和 metadata 步骤。当前已核对的 inline shell、引用脚本、action identity 与输入 bindings 由 source commit 内 `release-artifacts.py` 的 `BUILD_INPUT_CLOSURE` 记录；原有 action pin 和 scalar build input 改动自动改变 fingerprint。新增任意 shell、action 或外部文件输入无法证明闭包时，使用完整 tracked tree 指纹并禁用 reuse，仍正常构建；重新启用前须审核实际依赖并更新该 source 的闭包证据，不自动刷新。历史 v2/v3 metadata 继续按原 source 的算法与证据验证；v2 → v3 首次规划保守重建五张 image，后续相同输入才复用。每个 digest 的 fresh Grype DB、High/Critical scan、精确签名和可信 provenance 门禁保持必需。

Production 发布产出三个不可拆分的 GitHub Release assets：

- `release-manifest.json`：发布 repository/tag/ref/commit/run，以及 API、API migrator、Auth、Auth migrator、Realtime 的 immutable digest 和每个 component 的 source/fingerprint/signer。
- `deployment-bundle.tar`：确定性、未压缩、至多 64 MiB 的部署包。只包含 `.infisical.yaml`、Compose、环境模板、PostgreSQL role 初始化、部署/Infisical/manifest 校验脚本，以及 manifest 和 `deployment-index.json`。Index 绑定发布身份与每个文件的 SHA-256/mode。
- `deployment-bundle.sigstore.json`：Cosign `sign-blob` 的 Sigstore bundle，至多 1 MiB；签名覆盖整个 tar，包括 Compose、可执行脚本和 image digest 清单。

CI 从 manifest 指定 commit 的 Git objects 读取包内容，验证源码指纹、构建来源和五张 image 的精确签名后签署 tar。VPS 只使用独立安装的 verifier，严格要求 signer identity `https://github.com/Ericsunsk/IdeaBoard/.github/workflows/docker-images.yml@refs/tags/<release-tag>` 和 issuer `https://token.actions.githubusercontent.com`。验签成功后才解析及解包；拒绝未知/缺失/重复路径、traversal、symlink/hardlink/device、异常 mode、超限或 hash/身份不一致。

Production job 下载并验证三个 assets，确认独立 manifest 与签名包中的 manifest 完全一致，然后通过受限 SSH 发送。失败发布重试或回滚使用 [Deploy Existing Production Release](../../.github/workflows/deploy-production.yml)，只填写具有完整签名部署包的 strict SemVer `release_tag`；该 workflow 不 build、scan、sign 或 push images。两条入口共用 `production-deployment` concurrency 与 `production` Environment，migration 开始后的失败不自动重试或恢复数据库。

### Production 自动部署通道

通过可信管理通道把已审核的本地文件安装到 VPS，安装来源独立于待验签的包；不从待部署 tar 中执行 installer：

```bash
install -o root -g root -m 0755 /secure/provision/production-deploy-entrypoint.sh \
  /usr/local/libexec/ideaboard-production-deploy
install -o root -g root -m 0755 /secure/provision/deployment-bundle.py \
  /usr/local/libexec/ideaboard-deployment-bundle
```

VPS 预装 Python 3、jq、flock、Infisical CLI、Docker Compose 与 Cosign。Verifier 和 entrypoint 为 root-owned、不可由 group/other 写入。Git、gh 与源码 token 不属于 runtime 依赖。

GitHub Actions 使用专用 Ed25519 deployment key；public key 通过 forced command 安装：

```text
restrict,command="/usr/local/libexec/ideaboard-production-deploy" ssh-ed25519 <public-key> ideaboard-production-deploy
```

`restrict` 禁用 PTY、port/agent/X11 forwarding；entrypoint 拒绝参数与非空 `SSH_ORIGINAL_COMMAND`。SSH 不发送 remote command，stdin 仅包含 `IDEABOARD_DEPLOY_V2`、只读 token 行、绑定 tag/commit/精确长度的 JSON header 和 tar/signature bytes。token 必须为单行、非空且不超过 8192 bytes；所有输入 bounded，额外/截断 bytes 与旧协议直接拒绝。token 不写入文件、日志或参数，Infisical 后的部署进程移除 token。

GitHub `production` Environment 配置：

| 类型 | 名称 | 用途 |
| --- | --- | --- |
| Variable | `VPS_HOST` | 已验证 hostname 或 IPv4 |
| Variable | `VPS_SSH_PORT` | SSH port，例如 `5522` |
| Variable | `VPS_DEPLOY_USER` | forced command 用户，当前 `root` |
| Secret | `VPS_DEPLOY_PRIVATE_KEY` | 专用 deployment private key |
| Secret | `VPS_KNOWN_HOSTS` | 经可信管理通道核对、匹配 host/port 的 pinned host key |
| Secret | `INFISICAL_TOKEN` | 只读 `prod` token，scope 覆盖 `/` 与 `/ai-proxy` |

业务 secret 由 Infisical Cloud 注入；GitHub 不读取业务值。`/ai-proxy` 是 secret path，不表示 VPS 保存该应用源码。PostgreSQL owner credential 留在 root-only `/var/lib/ideaboard/owner.env`；已有 database volume 而 owner file 缺失时部署拒绝继续，必须恢复同一口令。Fresh Database 首次部署才创建 owner credential。GHCR runtime credential 使用 VPS Docker credential store。

Host topology 由 root-only `/etc/ideaboard/deployment.json` 持有，例如 `{"npm_network":"npm-nw"}`。它在 Cloud 注入后生效，Cloud 不能覆盖本机路径、release state、Compose project 或签名要求。

VPS 文件职责固定：

| 路径 | 内容 |
| --- | --- |
| `/opt/stacks/ideaboard/bundles/<tag>/<bundle-sha>.<id>/` | 原始 tar/signature、传输身份与已验证 `content/` |
| `/opt/stacks/ideaboard/docker/images.env` | 当前五张 immutable image refs |
| `/opt/stacks/ideaboard/docker/releases/` | current/previous/history 与非 secret snapshots |
| `/var/lib/ideaboard/owner.env` | 本机 DB owner credential |
| `/etc/ideaboard/deployment.json` | 本机 topology |
| `/opt/stacks/ideaboard-backups/` | 数据备份与 checksum |

升级保留上述 state、凭据、backups、Docker volumes/images；应用源码和 `.git` 不进入 VPS。每次部署保留原始签名包及其 digest，回滚时重新验签完整目标包。

发布创建新 tag，不移动已发布 tag：

```bash
git tag v0.1.0 <verified-main-commit>
git push origin v0.1.0
```

SSH connect timeout 为 15 秒，部署最长 25 分钟。GitHub summary 只记录允许公开的 tag/commit/digests、失败阶段与 recovery hint；entrypoint 不向 GitHub 返回容器原始日志。发布串行执行，不取消正在进行的 migration。

### 2. 首次 VPS 初始化

先通过可信管理通道安装上述 runtime prerequisites、entrypoint、verifier 和已审核的 `bootstrap-vps.sh`。从同一个 Production GitHub Release 下载三个 assets，然后在 VPS 执行：

```bash
/secure/provision/bootstrap-vps.sh --ref v0.1.0 \
  --manifest /secure/release/release-manifest.json \
  --bundle /secure/release/deployment-bundle.tar \
  --signature /secure/release/deployment-bundle.sigstore.json
```

`INFISICAL_TOKEN` 通过可信 secret 通道注入进程。Bootstrap 只把本地 assets 交给已安装的 entrypoint，不 clone repository、不安装待验签代码、不接受隐式分支/latest 或强制覆盖 stack。签名、tag、commit、长度或必需文件不合格时 fail closed。

生产 secret 配置见 [Secrets runbook](./secrets-infisical.md)。`POSTGRES_PASSWORD` 属于本机 owner credential，禁止进入 Cloud。Image refs 不进 Cloud，由已验签 manifest 写入稳定 `images.env`。Release state 不保存业务 secret；migration 前失败可恢复 image snapshot，migration 后失败必须结合 backup 与当前 schema 评估，不能自动 DB restore。

### 3. 后续部署 / 升级

运行 Docker Images 的新 tag 发布，或运行 Deploy Existing Production Release 并选择完整签名包的目标 tag。部署入口依次执行：

1. 严格验签完整 bundle，校验身份、文件 allowlist、hash/mode 与 immutable manifest。
2. 保存版本化包与原始签名，使用稳定 images/release/backups 路径。
3. 从 Infisical 注入业务 secret，覆盖本机 topology/state 与强制签名策略。
4. 验证每张 image 的原始 signer，执行 preflight、pull、backup、API/Auth migration、启动与 smoke。
5. 成功写入 current/previous/history，记录 bundle digest、路径、源码身份和 image digests。

禁止直接运行 unsigned manifest、单独替换 Compose/scripts 或 image-only rollback。操作性检查可使用当前 release 的已验证脚本；从 `docker/releases/current.json` 的 `deployment_bundle.directory` 获取 `content` 路径，并显式传入稳定 `IMAGE_ENV_FILE`、`RELEASE_STATE_DIR`、`BACKUP_DIR` 和 owner file。更新和回滚统一经过受信 entrypoint 重新验签。

### 4. Backup Hardening

> [!IMPORTANT]
> PostgreSQL backup 包含 `api_account_secrets`、`storage_profile_secrets` 等密钥表，可能含有上游 API key、S3 access key/secret、endpoint 与 bucket 配置。`postgres-*.dump[.age|.gpg]` 必须按 secret artifact 处理。

- 每个 PostgreSQL 与 uploads backup 会生成同名 `.sha256` 文件，例如 `postgres-ideaboard-YYYYMMDD-HHMMSS.dump.sha256`、`postgres-ideaboard-YYYYMMDD-HHMMSS.dump.age.sha256`、`api-uploads-YYYYMMDD-HHMMSS.tar.gz.sha256`。
- 设置 `BACKUP_ENCRYPTION_RECIPIENT` 后，backup 会先加密再为最终落盘 artifact 生成 checksum。`age1...`、`ssh-*` 或 `age:<recipient>` 使用 age；`gpg:<recipient>` 或其他值按 GPG recipient 处理。
- `"${DEPLOY[@]}" doctor` 会展示最近 backup 的路径、`checksum=valid|invalid|missing|unchecked`、是否加密，以及最近 release 记录中的 backup 阶段状态。
- 当前 DB restore 仍需人工确认；脚本不会在 migration 后失败时自动回滚数据库。

当前 Docker stack 中唯一的关系型数据库是 PostgreSQL；stack 同时管理 API、Auth、NATS JetStream 与 uploads volume。首次部署或切换部署形态前，请先确认目标 PostgreSQL 中已有需要保留的数据，或明确按空库初始化。

<a id="api-database-baseline"></a>

### 5. API 业务库 baseline

`ideaboard` 是固定 schema owner；`ideaboard_api` 与 `ideaboard_auth` 是彼此独立的 limited runtime roles。三个 roles 必须使用不同密码；长期运行的 API/Auth 容器只接收各自 runtime credential，只有一次性的 `api-migrate` / `auth-migrate` 容器接收 owner credential。

空 PostgreSQL 库由 API migration 按 `apps/api/internal/db/migrations/` 中的当前 baseline 初始化业务 schema；Atlas 在 `atlas_schema_revisions` 中维护版本与校验信息，API runtime 不再自动迁移。Docker 部署会先运行 `api-migrate`，授予 runtime 常规业务表 DML，并将 Credits Ledger 收紧为只读 + typed Credit Commands；再运行 Auth migration，为 Auth runtime role 授予 Better Auth tables DML 与 `get_auth_runtime_settings()` 的 `EXECUTE`，最后启动服务。`PG_MIGRATION_USER` / `PG_MIGRATION_PASSWORD` 只注入一次性的 migration process，不得进入长期运行的 API/Auth runtime；本地示例见 `docker/migration.env.example`。

Schema 变更必须经过同一条 Schema Release lifecycle：在 `apps/api` 中修改 domain fragment 或 manifest 后运行 `BASE_SHA=HEAD API_TEST_DATABASE_DSN='<test-dsn>' go run ./cmd/schemarelease --write-artifact` 生成 `internal/db/schema/schema.sql`，提交 snapshot 与 append-only migration；发布前和 CI 使用相同 command 但不带 `--write-artifact`，以只读方式检查 manifest assembly、snapshot drift、sqlc generation、migration immutability 和 PostgreSQL catalog semantics。不要调用已移除的旧 schema assembly 或 migration history scripts。

非空数据库不能直接套用该 baseline。若你从其他形态迁入数据，标准做法是先导出需要保留的数据，再在空库上初始化当前 baseline，最后用受控脚本导入业务数据；不要手动修改 `atlas_schema_revisions` 绕过 migration 校验。

Atlas migration integrity 由 `atlas.sum` 与 `atlas_schema_revisions` 共同保证；API migration image 在部署前应用 versioned migrations，随后由 `finalize-migrate` 完成 River schema 与 runtime privilege setup。Better Auth 的认证表可以先存在，不计入 API baseline 冲突；不过标准部署顺序仍是 API baseline first、Better Auth migration second。Clean-break 后禁止只回滚 API/Auth image；需要同时恢复数据库 backup 或重新初始化 Fresh Database。

> [!WARNING]
> `api-migrate` 会在 `atlas migrate apply` 之前比对数据库记录的 Atlas revisions 与镜像内 migration 目录。若数据库记录了镜像里不存在的 version（错误的 image digest、回滚到旧 image、migration 文件被改名或删除、CI 变量错配），migration 会以 `clean break reset is not allowed` **失败**，并在日志中列出缺失的 version；**不会**自动清库。按顺序处理：
>
> 1. 先按上一节的签名包回滚流程确认部署的 digest 与 commit 是否与数据库的 migration history 匹配。绝大多数情况这是真正的故障，用匹配该 history 的 image 重跑即可，不要走清库路径。
> 2. 确认是需要保留数据的 clean break 时，先完成 PostgreSQL backup，并导出需要保留的数据。
> 3. 只有在确认要放弃当前全部数据后，才在 **一次性 migration 容器**上设置 `ATLAS_CLEAN_BREAK_ALLOW_RESET=true` 重跑 `api-migrate`。该值只在 migration 容器注入，绝不能进入长期运行的 API/Auth runtime。
> 4. drop 前 migrator 会记录 `current_database()`、缺失的 version 与 public schema 全部表的行数摘要；请保留这段日志。drop 完成后会立即执行 `atlas migrate apply` 重建当前 baseline，若该步失败，数据库处于空且无数据状态，需要按 backup 流程恢复。
> 5. 不要长期保留 `ATLAS_CLEAN_BREAK_ALLOW_RESET=true`：它会让后续任何一次 image/migration 错配都变成静默清库。clean break 完成后应立即移除该变量。

Migration workflow 使用 Atlas Community Edition 的 `atlas migrate validate` 完成 `atlas.sum` integrity、Fresh replay 和 PostgreSQL semantic validation；`atlas migrate lint` 需要 Atlas Pro 登录，不是部署或 CI 前置条件。新增 migration 由 `atlas migrate diff` 生成，必须与 `atlas.sum` 一起提交。`scripts/atlas-migration-check.sh` 只检查本次新增 SQL，并默认阻止 `DROP TABLE`、`DROP COLUMN`、`DROP CONSTRAINT`、`TRUNCATE` 等 destructive DDL（bare `DROP <object>` 与 `ALTER TABLE ... DROP <object>` 两种写法都命中）；若业务确实需要 destructive change，必须在 SQL 中写入 `-- destructive: approved`，并在受控 review 环境显式设置 `ATLAS_ALLOW_DESTRUCTIVE=1`。CI 使用隔离的 Fresh database 运行该 gate，然后对集成 database 执行两轮 Atlas apply，覆盖 Fresh 与 incremental idempotency 路径。

PostgreSQL init script 只在空 volume 创建 API/Auth runtime roles。项目尚未上线时若本地 volume 早于该角色分离方案，直接删除并重建开发 volume；不要把 owner credential 配回任何长期运行的应用 runtime。

### 6. 初始化管理员

```bash
docker exec \
  -e INIT_ADMIN_EMAIL='admin@example.com' \
  -e INIT_ADMIN_DISPLAY_NAME='Admin' \
  -e INIT_ADMIN_PASSWORD \
  ideaboard-api /app/ideaboard-server init-admin
```

`INIT_ADMIN_PASSWORD` 从可信 secret 通道注入当前 shell，避免明文参数与 shell history。命令消费已运行 API 容器的 runtime 配置，不依赖 repository 或额外 Compose 文件。

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

自动部署从 root-owned `/etc/ideaboard/deployment.json` 读取本机拓扑配置。该文件可选，必须是部署用户拥有、不可被 group/other 写入的普通文件（推荐 `root:root 0600`），只允许以下字段：

```json
{"npm_network":"npm-nw"}
```

受限 entrypoint 将其转换为 `IDEABOARD_COMPOSE_OVERRIDE_FILE=docker/compose.npm.yml` 和 `NPM_NETWORK_NAME=npm-nw`，在注入 Infisical 后传给部署进程。这些非 secret 配置不放入 Infisical，也不写进 `/var/lib/ideaboard/owner.env`。手动部署则直接以这两个进程环境变量运行部署命令；仅 encrypted offline 模式可从 `docker/.env` 读取。

PostgreSQL 初始化脚本由受限入口从验签包原子发布到 `/opt/stacks/ideaboard/docker/postgres/init-runtime-roles.sh`，Compose 通过宿主强制设置的 `IDEABOARD_POSTGRES_INIT_FILE` 绑定该稳定路径。相同脚本内容保持原文件，重复部署不会因版本目录变化重建 PostgreSQL；Infisical 无权改变该路径。

随后通过 Deploy Existing Production Release 部署目标签名包。部署脚本会把 `compose.npm.yml` 叠加到 canonical Compose 配置，使 API/Auth/Realtime 同时保留 IdeaBoard 内网并加入 NPM external network。NPM 的 Forward Hostname / IP 与 Forward Port 分别为 `ideaboard-api` / `6000`、`ideaboard-auth` / `6001`、`ideaboard-realtime` / `6002`；Realtime 的 WebSocket 路由需启用 WebSocket Support。不要使用容器自身的 `127.0.0.1`。PostgreSQL 与 NATS 只保留在 IdeaBoard 内网。

旧 stack 重建后，先确认 NPM 容器内可以解析并访问 `ideaboard-api:6000`、`ideaboard-auth:6001` 与 `ideaboard-realtime:6002/health`，再执行 `docker exec npm nginx -t && docker exec npm nginx -s reload`，刷新静态 upstream 的旧容器 IP；最后复验公开 health。

取消 NPM 接入时，先切换反代流量，再删除本机 topology 配置（手动模式移除上述环境变量）并执行 Deploy Existing Production Release 重部署当前签名包。Docker 会按新的目标配置重建 API/Auth，canonical Compose 文件不需要修改。

#### 宿主机 Nginx

在本地受信 repository 生成 Nginx 配置，经管理通道把生成文件复制到 VPS：

```bash
scripts/render-nginx-template.sh /tmp/ideaboard.nginx.conf
sudo cp /tmp/ideaboard.nginx.conf /etc/nginx/sites-available/ideaboard.conf
sudo ln -sf /etc/nginx/sites-available/ideaboard.conf /etc/nginx/sites-enabled/ideaboard.conf
sudo nginx -t
sudo systemctl reload nginx
```

如果 VPS 已经有 certbot 或自管的 `443 ssl` server block，可以把生成文件里的 `/api`、`/api/auth` location 合并进现有 `ideaboard.art` server block。同时必须在该 HTTPS vhost（以及 Nginx Proxy Manager 对应的 HTTPS Proxy Host）中保留 `api.ideaboard.art` 的独立 reject 规则，返回 `444` 或直接拒绝连接，避免 retired API hostname 在 TLS 层落入主站 vhost。

默认 upstream 是宿主机 loopback 端口：

```text
NGINX_API_UPSTREAM=http://127.0.0.1:6000
NGINX_AUTH_UPSTREAM=http://127.0.0.1:6001
NGINX_AUTH_HEALTH_UPSTREAM=http://127.0.0.1:6001/api/health
NGINX_REALTIME_UPSTREAM=http://127.0.0.1:6002
```

API trusted proxies 无 env 开关：本地只信 loopback，production 追加私网段
（`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`，见
`AppConfig.TrustedProxies`）。裸机/拆分部署须把反代放在上述范围内，否则
客户端 IP 与限流归因错误；Auth 侧确需覆盖才用 `AUTH_TRUST_PROXY`，API 侧改代码。

如需明确绑定主站域名，可设置 `NGINX_SERVER_NAME=ideaboard.art` 后重新执行 本地 `scripts/render-nginx-template.sh`。

如果 Gateway 域名使用 Cloudflare `Proxied`，并且 VPS 防火墙已限制 80/443 只接受 Cloudflare IP ranges，可以生成真实客户端 IP include：

```bash
scripts/render-cloudflare-real-ip-conf.sh /tmp/cloudflare-real-ip.conf
sudo cp /tmp/cloudflare-real-ip.conf /etc/nginx/cloudflare-real-ip.conf
```

渲染 Nginx template 前，在本地进程设置：

```text
NGINX_REAL_IP_INCLUDE=/etc/nginx/cloudflare-real-ip.conf
```

重新执行 本地 `scripts/render-nginx-template.sh` 并 reload Nginx 后，反代会通过 `CF-Connecting-IP` 恢复真实客户端 IP。不要在源站仍允许非 Cloudflare 公网直连时启用该 include。

### 8. 验证

先按[日常运维](#operations)初始化当前包的 `DEPLOY` 数组，再检查：

```bash
"${DEPLOY[@]}" ps
"${DEPLOY[@]}" smoke
curl http://127.0.0.1:6000/api/health
curl http://127.0.0.1:6001/api/health
curl "${PUBLIC_ORIGIN}/api/health"
curl "${PUBLIC_ORIGIN}/api/auth/get-session"
```

`/api/auth/get-session` 未登录时返回未认证是正常的；关键是宿主机反代服务能转发到 Auth Service。`"${DEPLOY[@]}" smoke` 会检查 PostgreSQL、NATS JetStream、本机 API/Auth health、River schema、默认 JetStream event stream，以及公网 `${PUBLIC_ORIGIN}/api/health`。

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

管理员只从当前成功发布记录选择已验签、root-owned 的 bundle。以下数组直接调用包内脚本，路径和 topology 在 Cloud 注入后固定；`INFISICAL_TOKEN` 由可信 secret 通道提供。每次切换发布后重新初始化，更新/回滚仍使用受信部署入口。

```bash
BUNDLE_DIR="$(jq -er '.deployment_bundle.directory' /opt/stacks/ideaboard/docker/releases/current.json)"
DEPLOY_ENV=(env -u INFISICAL_TOKEN -u IDEABOARD_COMPOSE_OVERRIDE_FILE -u NPM_NETWORK_NAME
  IDEABOARD_SECRETS_SOURCE=infisical
  "IDEABOARD_STACK_DIR=${BUNDLE_DIR}"
  "IDEABOARD_DEPLOYMENT_BUNDLE_DIR=${BUNDLE_DIR}"
  IMAGE_ENV_FILE=/opt/stacks/ideaboard/docker/images.env
  RELEASE_STATE_DIR=/opt/stacks/ideaboard/docker/releases
  BACKUP_DIR=/opt/stacks/ideaboard-backups
  IDEABOARD_POSTGRES_INIT_FILE=/opt/stacks/ideaboard/docker/postgres/init-runtime-roles.sh
  ENV_FILE=/var/lib/ideaboard/owner.env COMPOSE_PROJECT_NAME=ideaboard)
if [[ -f /etc/ideaboard/deployment.json ]]; then
  DEPLOY_ENV+=("IDEABOARD_COMPOSE_OVERRIDE_FILE=${BUNDLE_DIR}/docker/compose.npm.yml"
    "NPM_NETWORK_NAME=$(jq -er '.npm_network' /etc/ideaboard/deployment.json)")
fi
cd "${BUNDLE_DIR}"
DEPLOY=("${BUNDLE_DIR}/scripts/infisical-run.sh" --env prod --path / --path /ai-proxy --
  "${DEPLOY_ENV[@]}" "${BUNDLE_DIR}/scripts/deploy-stack.sh")
"${DEPLOY[@]}" preflight
"${DEPLOY[@]}" doctor
"${DEPLOY[@]}" doctor --deep
"${DEPLOY[@]}" status
"${DEPLOY[@]}" logs --tail=200 api api-worker auth postgres nats
"${DEPLOY[@]}" restart
"${DEPLOY[@]}" stop
```

停止服务使用上面的 `stop`。卸载与删除 volumes 是独立的 destructive 管理操作，先确认数据与 backup；不属于自动部署或回滚。

### Runtime 硬化（连接池 / 启动校验 / 重试矩阵）

数据库连接池为代码常量（无 env 开关）：

| 常量                       | 默认 | 说明               |
| -------------------------- | ---- | ------------------ |
| `DefaultPoolMaxOpenConns`  | 25   | runtime pgxpool 上限 |
| `DefaultRiverPoolMaxConns` | 10   | River pgxpool 上限 |

单实例连接预算 = 25 + 10 = 35，必须留出 migrations / CLI / fixtures 余量，远低于 PostgreSQL `max_connections`（默认 100）；总预算上限 `maxRuntimePoolBudget = 80` 由 `Config.Validate` 在启动期 enforcement。idle 与回收窗口同样为代码常量（见 `DefaultPoolMaxIdleConns` / `DefaultPoolMaxLifetimeMin` / `DefaultPoolMaxIdleTimeMin` / `DefaultRiverPoolMinConns`）。

启动期 fail-fast 链（按顺序，任一失败进程直接退出）：`Config.Validate`（DB 口令/SSL、非法 env、PUBLIC_ORIGIN、`APP_SECRETS_ENCRYPTION_KEY` presence）→ `AUTH_INTERNAL_KEY`（生产 32+ 字符非占位）→ imgproxy 配置 → secrets key 格式校验（base64 32-byte）→ runtime 权限校验。`PUBLIC_ORIGIN` 缺失不再等到首次拼 URL 才暴露。

River 重试矩阵是代码持有的 typed policy（事实源 `internal/platform/riverpolicy.RiverRetryMatrix`，insert 点引用同包常量，改预算只改一处）：

| Queue                                                                             | MaxAttempts | 说明                                                                                                                     |
| --------------------------------------------------------------------------------- | ----------- | ------------------------------------------------------------------------------------------------------------------------ |
| `drawing_default`（`drawing_generate` + settlement-only `drawing_final_failure`） | 25          | panic/process error/基础设施故障的队列层重试；provider transient 在预算内最多 2 次整任务重投（`tasks.retry_count` 为界） |
| `credit_reconciliation`                                                           | 5           | 对账卡住直接告警，不 churn                                                                                               |
| `payment_reconciliation` / `payment_checkout_shutdown`                            | 8           | Stripe/DB transient 重试                                                                                                 |
| `vision_billing`                                                                  | 5           | 快照对账                                                                                                                 |
| `media_reclamation`                                                               | 10          | 存储抖动偏 transient，预算高于其他维护队列                                                                               |

attempt 间隔为 River 默认指数 backoff + jitter。各队列的 depth/lag 都有独立 gauge（`ideaboard_river_queue_depth{queue}` / `ideaboard_river_queue_lag_seconds{queue}`，15s 刷新；队列清单以 `internal/platform/riverpolicy.RiverRetryMatrix` 为准），排障见 [backend-metrics.md](./backend-metrics.md) C 节（按 queue 逐个看，不要只看 `drawing_default`）。

日志默认脱敏：`internal/platform/applog` 对 credential/token/secret 类 key 直接 `[REDACTED]`，并擦除 value 中 `password=`/`token=` 对与 `scheme://user:password@host` 形态。排障贴日志时仍不要粘贴整段 DSN 或整块 env。

OpenAPI breaking-change 门禁：CI `API / OpenAPI Breaking Changes` job 用 oasdiff 对比可信 base→head，error 级 breaking（删 path/operation、新增必填、删响应字段、收紧 enum）默认失败；additive 变更通过。确属 intentional 的 breaking 须经明确审核，并在 `scripts/openapi-breaking-reviews.tsv` 记录 spec 路径、可信 base commit、base/head 文件 SHA-256 和审核来源。门禁仍执行实际比较，仅认可完全匹配的已审查转换；不同 spec、base 或文件内容的 breaking 继续失败。该记录同时适用于 PR 和 push，PR label 不再跳过比较。本地复现：`./scripts/openapi-breaking-check.sh <base-sha>`。

### Payments 事故处理

需要立即停止充值时，在 Admin Payments 设置将 `purchases_enabled` 设为 `false`。新的 Checkout 会被拒绝；River 会异步 Expire 仍有效的待支付 Stripe Checkout Session，重复执行安全幂等。不要删除 Payment Order、Refund、Dispute、Ledger 或 Webhook Inbox 来“取消”支付。

随后按以下顺序恢复：

1. 在 Stripe Dashboard 处理退款、Dispute、Webhook delivery 或账户能力问题，保留 provider object ID 作为关联依据。
2. 使用 Admin Payment Order/Dispute 读模型查看未收敛订单、退款 anomaly 和 Dispute 状态；不要把 Dashboard 状态手工写入 PostgreSQL。
3. 保持 River worker 运行，等待其对账重试；必要时重启 API/Worker 后确认 River job、PostgreSQL 和 Stripe 配置都健康。对账以 canonical Stripe Retrieve 收敛，Webhook 和 SSE 都不是支付事实源。
4. 仅当未结算订单、Refund 与 Dispute 已按预期收敛，并重新完成相关 smoke 后，才重新开启充值。

多实例部署中，Stripe 配置在每次请求时从 PostgreSQL 读取，Webhook 和 River 也通过 durable state/claim 协作；SSE 只提示本实例已提交的变更，客户端必须重新读取订单、Wallet 或历史。不要为此引入 WebSocket、Redis 或新的任务队列。

排障优先顺序：

1. `"${DEPLOY[@]}" doctor`
2. `"${DEPLOY[@]}" logs --tail=200 api api-worker auth postgres nats`
3. `"${DEPLOY[@]}" doctor --deep`
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

1. 先在 Cloudflare 或 reverse proxy 阻断 `/api/v1/workspaces/*/rewards/*` 与 `/api/control/v1/rewards/*`，保留 health endpoint，并确认外部请求已被 gateway 拒绝。
2. 再通过 Deploy Existing Production Release 部署已确认 schema 兼容的前一签名包，完成 API/Auth health、登录、Credits 与非 Rewards 核心路径 smoke；如回滚版本仍包含 Rewards routes，继续保留 gateway block，直到修复版本上线并通过验证。
3. 确认修复版本稳定后再移除 gateway block。不要为事故回滚临时引入 Feature Flag、兼容 endpoint 或 dual write。

一旦存在 Redemption，禁止执行会删除 Rewards 数据的 destructive down migration，也禁止删除对应 Credits Ledger facts。应用回滚保留 Rewards schema 与历史事实；旧 image 可以暂不使用这些表。DB restore 是独立、人工确认的操作，不随签名包回滚自动执行。

<a id="upgrade-and-rollback"></a>

## 升级与回滚

### 回滚决策

生产事故先停止受影响的写入：支付事故关闭充值，Rewards 事故按本手册阻断相关路由，注册事故关闭公开注册。止血生效后再选择签名包回滚 或向前修复，恢复写入前完成对应 smoke。

签名包回滚同时恢复目标 Compose、部署脚本与容器镜像；Web 由 Pages 发布流程单独管理。签名包回滚不撤销 PostgreSQL schema、River job 或已持久化的业务事实。回滚前确认目标签名包的 image 能处理当前 schema 与已有数据；不兼容时部署修复版本与新 migration，禁止盲目执行 destructive down migration。Clean-break 的数据库恢复规则见 [API 业务库 baseline](#api-database-baseline)。

涉及 River job 参数变化时，停止相关 enqueue，并在切换 worker 前确认在飞任务已结束、等待与重试 job 可由目标版本处理。结合 [backend-metrics.md](./backend-metrics.md) 的 queue depth/lag 与数据库中的 River job 状态检查，不能仅凭等待队列为空认定 worker 已排空。NATS JetStream 只有 7 天 retention；支付、Credits 和 Task 事实以 PostgreSQL 与 canonical provider 查询为准，不依赖 Webhook 或 SSE replay 做数据库恢复。

### Media storage cutover

Workspace Media 固定以 `builtin-local` + `storage_key` 定位对象；avatar 与 logo 使用当前 active profile，业务表和 Canvas 只持久化 Media Asset API URL。切换 Local / S3-compatible profile 时按以下顺序执行：

1. 先确认目标 profile 已通过 Admin storage test，并在隔离对象上完成 upload、read、公开 URL 和 delete smoke；确认 API 与 Web 使用同一 release manifest。
2. 更换 active profile 前，必须将全部 avatar 与 logo 按原 key 复制到目标存储并验证可读，再在 Admin 中 Apply 目标 profile；目标 profile 的 imgproxy credentials 与 source allowlist 必须同步配置。
3. 用真实用户流程验证 Canvas upload/reload、Admin logo、avatar、task output 和 Library materialization。当前没有可手动执行的 `media-reclamation scan` 命令（未实现）：不要拼装 `docker exec ... media-reclamation scan`；Media 收敛由 `media_reclamation` River queue worker 执行，按 [backend-metrics.md](./backend-metrics.md) C 节观察该 queue 的 depth/lag 与 worker 日志确认。
4. 切换后保留原 profile 及其对象至少一个 `GracePeriod`（当前 24 小时），只允许通过 Media reclamation worker 删除已确认无引用对象。确认稳定后再由 Admin 禁用旧 profile。

失败时先在 Admin Apply 上一个已验证 profile，并执行 API/Web smoke；不要回滚数据库或删除对象。若失败发生在 migration 之后，按本节签名包回滚 + backup 评估流程处理，Media Asset 的 `storage_key` 记录仍是事实源。

对象 orphan 检查必须以数据库引用反查 provider listing：导出 `media_assets` 中未归档对象的 `storage_key` 并归入 `builtin-local`，再按 active profile 纳入用户 avatar 与站点 logo 引用，与各 profile 的 object listing 做集合差异；差异对象先标记、观察一个 `GracePeriod`，禁止直接批量删除。Local profile 通过挂载的 `api-uploads` volume 只读扫描，S3-compatible profile 使用 provider CLI/API 的 prefix listing；扫描输出不得包含 access key、signed URL 或用户内容。

升级与回滚统一运行 [Deploy Existing Production Release](../../.github/workflows/deploy-production.yml)，输入目标 strict SemVer tag。目标发布必须有完整 tar、Sigstore signature 与一致 manifest；旧 manifest-only release 无部署资格。回滚同样重新验签整个目标包与五张 image，并执行 migration compatibility guard 和 smoke。不得组合旧 image 与新 Compose/scripts。DB restore 不自动执行，确认需要恢复数据后才按独立 backup restore 流程处理。

确认最近 backup：

```bash
"${DEPLOY[@]}" doctor
ls -lh /opt/stacks/ideaboard-backups/
"${DEPLOY[@]}" logs --tail=200 api api-worker auth postgres nats
```

受控 restore drill 建议至少记录：

1. 在隔离环境复制目标 backup、对应 `.sha256` 和必要的解密 key；不要直接在生产库上演练。
2. 对最终落盘 artifact 执行 `sha256sum -c <backup>.sha256`；如果 backup 已加密，校验通过后再用 age 或 gpg 解密到受控临时目录。
3. 对解密后的 PostgreSQL dump 执行 `pg_restore --list`，对 uploads tar 执行 `tar -tzf`。
4. 恢复前确认目标库可被覆盖、当前 migration 影响已评估，并记录是否需要同时恢复 uploads backup。
5. Restore 后运行 API/Auth health、关键登录路径和对象存储读取 smoke；涉及密钥表时按影响范围轮换上游 API key 与对象存储凭据。
6. 演练结束后清理解密后的 dump、tar、临时目录和本地下载副本。

数据保存在 Docker volumes：`postgres-data`、`nats-data`、`api-uploads`。不要使用 `docker compose down -v`，除非你明确要删除生产数据。

## AI Gateway 模型映射（单 AI 出口）

所有 AI 调用唯一出口是 Cloudflare AI Gateway；应用代码不装任何 vendor SDK，不存任何 provider key。provider 只挂在 CF 侧，代码里的 provider slug 是不透明路由 token。

- Service Model Pool entry 使用 `provider/model[@gateway-path]`；Submit 会把解析结果固化为不可变 `RoutingSnapshot`，worker 只执行快照，不读取 live pool。
- 网关 path 前缀：`openrouter` / `fal` 按 CF 原生 slug 直写，其它进 `custom-<slug>`。OpenAI 永不直连（挂在 OpenRouter 或网关自有槽位做非图像调用），因此没有 direct-images 形状。
- 响应形状按路由固定二选一：`openrouter` 走 chat-completions（含 `modalities: [text, image]` 的文生图），`fal` 走 media API（`{image:{url}}` / `{images:[{url}]}`，materializer 按 egress policy 下载落库）。
- `@path` 永远优先：fal 各模型路径、cutout/upscale/inpaint 类 custom 路由必须在映射里显式写 `@path`。
- 禁止事项：不上 Google 原生 `:generateContent`/`:predict` 路由（已删除对应方言代码）；upscale 只许 `fal-ai/esrgan`，禁止 `clarity-upscaler`（毛利为负）；vision 搜索走中央网关连接（openrouter 路由），模型与其它 feature 一样在模型映射里配（`vision` key，定价页"其它模型映射"区），搜索供应商设置里不再有 vision 配置面。
- Egress allowlist 需放行网关域名；fal 结果图床 host 由该 allowlist 覆盖（materializer 下载用）。

## ai-proxy Worker 部署（Cloudflare Git 直连）

VPS 镜像通道之外，`apps/ai-proxy`（唯一的 AI 出口适配器）由 Cloudflare Git 直连发布；GitHub `AI Proxy` lane 只验证、不部署。Dashboard 连接 Worker `ideaboard-ai-proxy`，production 分支绑 main。production 与 Preview base 的 Root directory 均为 `apps/ai-proxy`，Build command 均为 `npm run build`；Deploy command 分别为 `npx wrangler deploy` 与 **`npx wrangler versions upload --dry-run`**。

这里 Preview base 只做非 production 分支的构建验证：CLI `--dry-run` 在本地完成 bundle 后退出，不上传 Worker version、不部署、不生成 Preview URL，因此不需要 dev Gateway bindings 或 Secrets，也不需要 `[previews]`。这与已部署 Worker 的业务接口 `/v1/dry-run` 是两种操作。不要改为 `wrangler preview`、去掉 `--dry-run`，或为了让非 production 构建通过而复制 production Secrets。

Watch paths 在 production 与 Preview base 两边保持一致：Include 填 `apps/ai-proxy/**` 和 `.node-version`，Exclude 留空，路径相对仓库根目录。Worker 目录包含 package manifest / lock；根 `.node-version` 是 Cloudflare 构建实际使用的 Node version 输入。Go、前端和 CI 文件的普通变更不在触发范围内；Cloudflare 对空变更、3,000 个及以上文件或 20 个及以上 commits 的 push 可能绕过路径过滤，详见 [Build watch paths](https://developers.cloudflare.com/workers/ci-cd/builds/build-watch-paths/)。

生产变量仍在 Dashboard 配置：`[vars]` 非密钥（`CF_ACCOUNT_ID` / `CF_GATEWAY_ID`）可选 toml 明文或 Dashboard；Secrets 只放 Dashboard，值来自 Infisical prod `/ai-proxy` 的 `CF_AIG_TOKEN` / `WORKER_INTERNAL_SECRET`。生产推荐走 Infisical 原生 Cloudflare Workers 集成自动同步（project `ideaboard`、env `prod`、path `/ai-proxy`）。Preview base 不新增运行时密钥，不改 production bindings 或发布命令。

本地验证：`cd apps/ai-proxy && npm ci && npm run build && npx wrangler versions upload --dry-run`。Cloudflare 验收则先重新加载设置、核对 Preview base 命令包含 `--dry-run`，再重试一次既有非 production 失败构建，确认成功且日志含 `--dry-run: exiting now.`；检查 production 配置未变化。Watch paths 需保存后重新加载两边核对。回退时可恢复原 Watch paths；若恢复旧 Preview 命令，缺少 `[previews]` 的原始失败会重现，不能借回退触发 production 部署。

VPS 侧 prod `WORKER_BASE_URL` 填 production Worker 地址（`https://ideaboard-ai-proxy.<account>.workers.dev` 或自定域名），与 preview 无关。回滚 = CF dashboard 回滚 Worker 版本（无状态，无需数据回滚）。本地 bindings 与启动步骤见 [ai-proxy README](../../apps/ai-proxy/README.md#运行与验证)。

### Auth/Realtime production runtime

Auth、Auth migrator 和 Realtime 的 production runtime 使用 `.bun-image` 固定的官方 Bun Alpine immutable image；Node `.node-version` 继续用于 dependency install、build 和开发/CI tests。Dockerfile fallback 必须通过 handbook contract，与 canonical image 完全一致。保留 CA bundle，卸载 production 不需要的 APK package manager、TLS CLI 及其系统 OpenSSL 依赖；Bun 使用其内嵌 BoringSSL。服务以 image 自带 `bun` user 运行，healthcheck 显式调用 `bun`。

2026-11-22 的迁移验证覆盖实际 ARM64 image 的 Auth migration、PG `verify-full` TLS 正负用例、登录/session/sign-out、Node/Bun password hash 双向兼容与 Realtime WebSocket。Bun 是 Fastify alternative runtime，未来升级需重跑兼容验证。锁定 Grype 0.110.0 内的 Syft 1.42.3 尚不 catalog Bun binary：其 High/Critical gate 覆盖系统与应用 dependencies，不能证明 Bun 本身无漏洞。每次 runtime 升级必须额外核对 [Bun 官方 security advisories](https://github.com/oven-sh/bun/security/advisories) 及精确 BoringSSL commit source。此次 Bun 的四项官方 advisory 均已在所选版本修复；内嵌 BoringSSL `fe703e9faa3bfd22b04eb192066856557b7e015c` 的 PEM stack cleanup 已修正，且不含此次 OpenSSL DANE/CMS verification/QUIC transport 受影响实现，独立 source 审查补足 binary scan 盲区。回退使用上一份完整 verified bundle；不要单独回退到带已知 High/Critical CVE 的旧 Node patch。
