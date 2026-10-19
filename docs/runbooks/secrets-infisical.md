# Infisical Cloud Secrets（env 方案 P2）

本 runbook 是 Infisical 官方托管（Cloud）的唯一操作入口：组织/project 怎么建、
namespace 怎么定、业务怎么注入、dotenvx 是什么关系、轮换/上下线/审计怎么做、
备份与费用归谁、漂移门以哪边为准、将来如何迁回自托管。系统设计 contract 见
`DESIGN.md`，日常开发入口见 `DEVELOPMENT.md`，部署与数据保护见
`deployment.md` / `data-protection.md`。

> 状态：P2 从自托管切到 Infisical Cloud（用户已批准，小团队够用）。
> Infisical Cloud 是业务 secret 的 **primary 分发通道**；dotenvx 加密的
> `docker/.env` 降为 **offline fallback**（冻结副本，只读备用）。
> 业务 secret 两档永不同时进同一进程（`scripts/infisical-run.sh` 会拒绝包裹 `dotenvx`）。
> 本仓库不存任何自托管产物（无 compose、无本地 `INFISICAL_*` 自举文件）；
> 默认 site URL 为 `https://app.infisical.com`（`scripts/infisical-run.sh`
> 内 `CLOUD_SITE_URL` 单点常量，`--domain` / `INFISICAL_DOMAIN` 仅作覆盖用）。

## 1. 组织与 project 建法（Cloud dashboard）

1. 在 `https://app.infisical.com` 注册组织（org），开 SSO（有人就开，见 §4）。
2. 建单个 project `ideaboard`，建三个 env `dev` / `staging` / `prod`
   （`scripts/infisical-run.sh --env` 必填且只收这三个，与 env 名大小写一致）。
3. 不建 folder：secret 全放 root path `/`（注入即全集，见 §2）。
4. prod env 的写权限只给值班/发布人；dev 读权限给全员，prod 读默认不给新人。
5. CLI 与脚本侧用 `--projectId` 或 `infisical init` 绑定到 `ideaboard`，
   project ID 存在 `INFISICAL_PROJECT_ID`（CI secret / 本机 env，不进 git）。

## 2. Namespace：project × env（root path）

单个 project `ideaboard`（CLI `--projectId` 或 `infisical init` 绑定），
三个 env `dev` / `staging` / `prod`（`scripts/infisical-run.sh --env` 必填且只收这三个），
secret 全放 root path `/`，注入即全集（union injection，本就把全量 env 给每个 service）。

设计决策：per-service folder 在 union 注入下只是 theater（隔离还得靠 DB role），
小团队不建 folder；哪天要做按路径收窄 token scope 再分。
该设计与后端无关，未来迁回自托管时原样平移（见 §8）。

### 2.1 现有 25 个 Go env tag 的迁入对照

Go `env:"..."` 共 25 个（`apps/api/internal/platform/config/config.go`），
是“25 个 env tag”的计数口径。加上 Auth/Realtime 的 Node 侧 reads，
union 约 35 个 key，全部映射如下（值从 dotenvx 加密 `docker/.env` 原样迁入，不改值）：

| Key | Infisical 路径 | 消费者 | 备注 |
| --- | -------------- | ------ | ---- |
| `AUTH_INTERNAL_KEY` | `/` | api/auth/realtime | 单套内网 key，改一处 |
| `APP_SECRETS_ENCRYPTION_KEY` | `/` | api/auth | 主密钥；轮换走 §4 专用流程，不只换值 |
| `PUBLIC_ORIGIN` | `/` | api/auth | 每 env 一份（dev 与 prod 值不同） |
| `PG_HOST`/`PG_PORT`/`PG_DB`/`PG_SSLMODE` | `/` | api/auth(+migrate) | 拓扑值，非 secret 也随注入走，保证三端一致 |
| `AUTH_INTERNAL_URL` | `/` | api | 容器内默认 `http://auth:6001`，裸机才改写 |
| `API_INTERNAL_URL` | `/` | auth/realtime | 容器内默认 `http://api:6000` |
| `IMGPROXY_KEY`/`IMGPROXY_SALT` | `/` | api(+imgproxy sidecar 经 compose 传，非 Infisical) | 两 key 互不相同；imgproxy 容器由 Compose 插值读取注入的同一值，见 §3 |
| `NATS_PASSWORD` | `/` | api/worker | URL-safe 字符约束不变 |
| `BETTER_AUTH_SECRET` | `/` | auth | 生产 ≥32 高熵 |
| `RT_TICKET_SECRET` | `/` | api/realtime | 两端同名同值，改一处 |
| `REWARDS_CODE_HMAC_KEYS` | `/` | api | JSON keyring；轮换走 two-stage（§4） |
| `CREDITS_WELCOME_IDENTITY_KEY` | `/` | api | **fingerprint 绑定，误换 fail closed；非事故不轮换** |
| `RIVER_WORKER_CONCURRENCY` | `/` | api-worker | 非 secret 明文部署输入（compose 缺省 5），可随注入统一管理，但永不进 dotenvx 加密 `docker/.env` |
| `API_PG_USER`/`API_PG_PASSWORD` | `/` | api/api-worker/api-migrate | runtime role，与 owner 分离 |
| `AUTH_PG_USER`/`AUTH_PG_PASSWORD` | `/` | auth/auth-migrate | runtime role，与 owner 分离 |
| `AUTH_HOST`/`AUTH_PORT`/`AUTH_TRUST_PROXY` | `/` | auth | 监听面默认在代码，反代不在私网段才改写 |
| `RT_HOST`/`RT_PORT`/`RT_Y_COMPACT_EVERY`/`RT_Y_PERSIST_RETRIES` | `/` | realtime | 代码默认值面，无需每 env 设置 |

明确 **不进** Infisical 的：

| Key | 去向 | 原因 |
| --- | ---- | ---- |
| `POSTGRES_PASSWORD` | 本地 `docker/.env`（已有文件先解密，首次缺失时由 `ensure_env_file` 生成） | PostgreSQL owner 口令只供 Compose postgres / one-shot migrate；禁止进入 Cloud 和长期 API/Auth runtime |
| `PG_MIGRATION_USER`/`PG_MIGRATION_PASSWORD` | 仍在 `docker/migration.env`（本机 0600，gitignored），只进一次性 migrate 容器 | owner/runtime 分离；migrate 凭据永不进长期 runtime，也就不进 Infisical 分发面 |
| `VITE_*`（前端构建烘焙键，见 §11） | 不进 Infisical（键名由 `scripts/web-frontend-env-check.sh` 门禁，值按 §11 注入链处理） | Vite 把所有 `VITE_*` 烘焙进公开 bundle；生产正确值为空（同域 Gateway），存空字符串没有意义；未来某 `VITE_*` 需要非空生产值时再进 Cloud，注入形态不变 |
| `IDEABOARD_*_IMAGE`、`COSIGN_*`、`BACKUP_ENCRYPTION_RECIPIENT`、端口与 `NGINX_*`/`NPM_*` | image refs 独占 `docker/images.env`；其余通过部署进程 env 提供 | 部署输入非 secret（digest/upstream），`infisical-drift-check.sh` 会拒绝它们进 Infisical |
| `INIT_ADMIN_*`、`ATLAS_CLEAN_BREAK_ALLOW_RESET`、`API_MODE`、`APP_ENV`、`NODE_ENV`、`HEALTHCHECK_URL`、`ENV_FILE` | 进程本地/一次性 CLI 输入 | 启动拓扑或单次命令参数，不做 secret 分发 |

Cloud 模式下没有本地自举变量：以前自托管的 `INFISICAL_*`
（encryption key / auth secret / 自建库连接）整类消失，由官方托管，
本仓库与本机不再持有、不备份、不轮换它们。

迁入步骤（值不改，只换通道）：

```bash
# 1. 解密当前值（值不出屏，按需单值核对；私钥从密码管理器取，用完即清）
export DOTENV_PRIVATE_KEY='...'
dotenvx get API_PG_PASSWORD -f docker/.env
# 2. 在 Cloud dashboard 按上表建 project=ideaboard、env=dev/staging/prod，
#    secret 全放 root path `/`，逐 key 粘入解密值
# 3. 每 env 跑一遍漂移检查（§5），key 名全绿再切注入
```

## 3. 注入：`infisical run` 形态 + dotenvx 离线 fallback

唯一支持的 live 注入是 `scripts/infisical-run.sh`（薄封装 `infisical run --env --path ... -- <cmd>`，
默认连 Cloud `https://app.infisical.com`）：

```bash
# 生产 compose（单进程拿 union 全集，各 service 只读自己子集）
scripts/infisical-run.sh --env prod \
  -- scripts/deploy-stack.sh deploy --manifest /path/to/release-manifest.json

# 只渲染验证（不启动）
scripts/infisical-run.sh --env prod \
  -- scripts/deploy-stack.sh config

# 本地直跑（compose.local 只起 postgres/nats/imgproxy，api/auth 是宿主进程）
scripts/infisical-run.sh --env dev -- go run ./cmd          # apps/api
scripts/infisical-run.sh --env dev -- npm run dev           # apps/auth
scripts/infisical-run.sh --env dev -- npm run dev           # apps/realtime
```

`docker/images.env` 由首次 manifest deploy 自动创建并写入五个 immutable image refs；
后续 `up` 与 rollback 使用同一文件。手动渲染前需先完成该初始化（或从
`docker/images.env.example` 创建并填入 release manifest refs）。`deploy-stack.sh`
统一装配 Compose 输入：业务值只取 Cloud 注入，`POSTGRES_PASSWORD` 只取本地
`docker/.env`，image refs 只取 `docker/images.env`。owner 口令禁止进 Cloud，漂移门会拒绝。
部署机须能用 `DOTENV_PRIVATE_KEY` 或本地 `.env.keys` 解密现有 owner 口令；首次缺少
本地文件时 `ensure_env_file` 会生成并持久化 owner 口令，不生成 Cloud 业务 secret。
wrapper 始终移除注入的 `POSTGRES_PASSWORD` 和 `PG_MIGRATION_*`，
owner 值由部署脚本单独传给 Compose，再由 service environment maps 仅传给 postgres/migrate。

`--env` 必填（dev/staging/prod），`--path` 可重复（默认 `/`），
`--project-id` 缺省读 `INFISICAL_PROJECT_ID`。site URL 缺省即 Cloud
（脚本内 `CLOUD_SITE_URL` 常量显式 `--domain`，进程表可审计来源）；
只有 §8 迁回场景才用 `--domain` / `INFISICAL_DOMAIN` 覆盖。
本地 human 用 `infisical login`（CLI 默认即 Cloud），CI/VPS 用只读 service token
（`INFISICAL_TOKEN` env，见 §4）。

业务 secret 两档关系（不双轨并存跑；本地 owner 凭据独立于业务分发通道）：

- **Primary = Infisical Cloud live**（`infisical-run.sh`）：日常 dev、VPS 部署、CI 全部走它。
- **Fallback = dotenvx**（`dotenvx run -f docker/.env -- …` / `deploy-stack.sh` 内置解密）：
  Cloud 故障、断网、或 CLI 不可用时的离线逃生门。
  平时 `docker/.env` 冻结（只在 Cloud 侧轮换成功后由值班人单向同步一次，保持“最后已知好值”），
  故障时整命令切换到 `dotenvx` 形态，恢复后切回，不做部分 key 混读。
- wrapper 遇到 trailing 命令含 `dotenvx` 直接拒绝（退出 1），从机制上杜绝双源合并；
  `deploy-stack.sh` 识别 wrapper 的 `IDEABOARD_SECRETS_SOURCE=infisical` 标记，业务值只消费 Cloud 注入；
  本地文件解密后只读取 owner 口令，不作为业务 key 的缺省来源。直接运行时业务值才来自 offline 文件。
  image refs 始终读写同一 `docker/images.env`，不会写入临时解密文件。

冒烟（至少一个 service 取值正确）：

```bash
# 任选一个非敏感拓扑值做明文对照（secret 值只对 hash，不直接 echo 比对）
scripts/infisical-run.sh --env dev -- printenv PG_DB
# 期望输出 ideaboard（与 docker/.env 解密值一致）
```

## 4. 流程

### 轮换（改一处）

1. 在 Cloud dashboard（或 CLI）改对应 env 下的 key（共享 key 只改一处）。
2. 重启消费者（`docker compose up -d <service>` 或重跑宿主进程）；无需 PR、无需重加密。
3. 跑 `infisical-drift-check.sh` + `config` 渲染 + health（见 §9 验证矩阵）。
4. 故障回退：用 fallback `docker/.env` 的 frozen 值起服务（§3），修好 Cloud 通路后再同步。

特例（与普通 key 不同）：

- `REWARDS_CODE_HMAC_KEYS`：two-stage（新 current + 旧 previous 先并存部署，
  确认无旧 version 依赖后再移除 previous），见 `deployment.md` Rewards 节。
- `CREDITS_WELCOME_IDENTITY_KEY`：数据库 fingerprint 绑定，换值即 fail closed，
  非泄露事故**禁止轮换**；泄露则按数据事故处理（backup + 受控重加密，不只换 env）。
- `APP_SECRETS_ENCRYPTION_KEY`：信封加密主密钥，轮换必须走
  `data-protection.md` 的离线逐行重加密流程，不能只换 Cloud 里的值。

### CI / VPS 只读 service token

- 在 Cloud dashboard 为 CI 与 VPS 各建独立 service token：**read-only**，
  scope 到 `project=ideaboard` + 单 env（prod token 只读 prod），能读 `/`，
  不能写、不能管成员。
- token 经 CI secret / VPS root-only 文件（0600，如 `/secure/infisical/prod.token`）注入
  `INFISICAL_TOKEN`，永不进 git、永不打进镜像、永不出现在日志（`applog` 脱敏规则同样适用）。
- 用法：`INFISICAL_TOKEN="$(cat /secure/infisical/prod.token)" scripts/infisical-run.sh --env prod -- …`。
- 泄露/可疑：直接在 Cloud 侧 revoke 该 token 并重发，服务本身无需改配置；
  prod token 泄露按 secret 事故同时评估是否需要轮换其可见范围内的高敏 key。

### 新人 onboarding

1. 装 CLI：`npm i -g @infisical/cli`，`infisical login`（默认即 Cloud）。
2. 进 `ideaboard` project（admin 在 Cloud dashboard 授权对应 env 的 read 即可，prod 默认不给新人）。
3. 日常 dev：`scripts/infisical-run.sh --env dev -- …`（§3 例子），不需要本地私钥文件。
4. 只有值班/发布人才需要 dotenvx 私钥（fallback 同步用），新人默认不发。

### 离职 revoke

1. Cloud dashboard 侧移除 org/project 成员（或降为无 access），revoke 其个人 session 与其名下 token。
2. 若其持有过 prod 读权限：其个人 token 自动失效；service token 不变（非个人绑定）。
3. 若离职非正常/疑似带走 secret：按 §4 轮换顺序轮换高敏 key
  （`AUTH_INTERNAL_KEY`、`BETTER_AUTH_SECRET`、`APP_SECRETS_ENCRYPTION_KEY` 除外——主密钥走重加密流程），
   并记录到审计位。

### 操作审计位（Cloud dashboard）

- Cloud 侧：dashboard **Audit Logs**（谁、何时、改/读了哪个 env+key，token 身份）。
  值班每月抽查一次非预期变更（尤其是 prod）。
- 本仓库侧：`docker/releases/history/*.json`（发布记录）、backup 目录、
  `data-protection.md` 要求的 restore 记录（操作人/时间/来源 backup/目标环境/后续 rotation）。
- 两边 key 名以 code schema 为准对账（§5），审计结论只认 key 名与时间，不贴 secret 值。

## 5. 漂移门兼容（选边：仍校验文件，Cloud 为镜像）

**选边结论：`scripts/env-schema-check.sh` 继续是唯一的 required gate，
校验对象仍是 code（Go/Node schema）vs 文件（`*.env.example` + compose + 加密 `docker/.env` key 名），
不改、不降级。Cloud 是只读镜像，由 `scripts/infisical-drift-check.sh` 做同源检查。**

理由：code tags 是编译期事实源（改名即改代码），`.env.example` 是其派生文档；
Cloud 是运行时分发，存错 key 名只会在启动时爆炸。让 gate 去读远端
会把 CI 变成对外部服务的可用性依赖，且 secret 值会流经 CI 日志边界。
因此：

- Gate 不变：PR 照常跑 `env-schema-check.sh`（A–F 全门）。
- 新增镜像检查（不同步失败就 loud fail，不进 required gate，值班/发布前手动跑；
  export 一步走 Cloud API，需联网 + 登录/token，检查本身只读导出文件）：

```bash
infisical export --domain https://app.infisical.com --projectId <id> \
  --env=prod --format=dotenv > /tmp/infisical-prod.env
scripts/infisical-drift-check.sh /tmp/infisical-prod.env
```

它断言两条（只看 key 名，不看值）：required key 全在 Cloud 里；
Cloud 里无未知 key，且部署类 key（`IDEABOARD_*_IMAGE` 等）永不进 Cloud。

## 6. 备份与可用性（归官方，RPO 信任边界）

事实：

- **备份归官方**：Cloud 侧 secret 存储、加密、备份、恢复全部由 Infisical 官方负责；
  本仓库不再做任何 dump / 加密落盘 / restore drill，也不再持有平台主密钥。
- **RPO 信任边界（如实写）**：RPO/RTO 由官方备份策略与 SLA 决定，不在本仓库控制内；
  选 Cloud 即接受“secret 持久性信任官方”这一边界。出事看官方 status + dashboard，
  本仓库能做的只有 §3 fallback 切流与 §8 导出重建，不承诺自愈时间。
- **最后防线仍在**：冻结的 dotenvx `docker/.env`（加密进 git）是全量 secret 的
  “最后已知好值”脱机副本；Cloud 长时间不可用时，用它起服务（§3 fallback），
  恢复后逐 key 回填 Cloud。这是保留 dotenvx fallback 的核心原因，
  不要以“上了 Cloud”为由删除它。
- **单点认知**：Cloud 不可达 = 新注入失败（已起的业务容器不受影响——
  env 已注入进程内存，不会当场掉线）；恢复后重跑注入即可，无数据修复动作。

## 7. 费用（按 seat，以官网为准）

- Cloud 按 **seat**（成员席位）计费；service token / machine identity 不占 seat，
  CI 与 VPS 的只读 token 不产生费用，只有人成员算钱。
- 小团队（个位数成员）大致是每月 tens `$` 量级——**具体数字以官网为准**，
  本 runbook 不写死价格，调价看官网 pricing 页。
- 成本动作：离职及时移除 seat（见 §4 revoke）；长期只读的外部协作者用
  最小 env scope，来去都走 seat 增减，不要为省事开大范围常驻席位。

## 8. 未来迁回自托管的路（数据导出）

保留此段作为逃生设计，不执行、不预建：

- Cloud 存的是 key 名 + 值 + 注释的平直集合（§2 表即全量清单），
  `infisical export --format=dotenv` 可按 env 全量导出，无厂商锁定字段。
- 迁回动作 = 自建 Infisical（官方 compose）+ 按 §2 namespace
  （同 project、同 env、同 folder path）重建 + 逐 key 回填导出值 +
  把 `scripts/infisical-run.sh` 的默认 `--domain` 指回自建站。
- 脏点只有一处：注入脚本的默认 site URL 常量；namespace 与漂移检查逻辑零改动。
- 触发条件（任一）：费用超过自运成本、合规要求 secret 不出内网、
  Cloud 连续可用性不达标。触发前先做一次全 env 导出演练，确认导出完整再动手。

## 9. 验证矩阵（本次落地用）

| 检查 | 命令 | 通过标准 |
| ---- | ---- | -------- |
| Cloud 连通 | `infisical login` 后 `scripts/infisical-run.sh --env dev -- printenv PG_DB` | 输出与 dotenvx 解密值一致（至少一个 service 取值正确） |
| 注入渲染 | `scripts/infisical-run.sh --env prod -- scripts/deploy-stack.sh config` | 退出 0 |
| 漂移 | `scripts/infisical-drift-check.sh <Cloud export>` + 旧 `scripts/env-schema-check.sh` | 双绿 |
| 前端漂移 | `bash scripts/web-frontend-env-check.sh`（CI 在 `web.yml` 自动跑） | 声明/使用点/文档三方一致、无 secret、新 binding 先改 §11 |
| 无自托管残留 | 全仓 grep 自托管 compose / 自托管 db-redis / 本地自举 key 项 | 零命中（仅 Cloud 通道变量保留，见上） |
| 文档 | 本文件 | 覆盖组织建法、token、login、上下线、审计、备份边界、费用、迁回 |

## 10. 残留风险（诚实清单）

- **厂商与网络依赖**：Cloud 不可达或断网 = 新注入失败，只能切 dotenvx fallback
  （存量容器不受影响）；fallback 要求“轮换成功后单向同步一次”，靠值班自觉，
  无自动化 diff 提醒，长期不同步 = 逃生门过期。
- **RPO 不在手里**：见 §6 信任边界；Cloud 侧极端丢数据时，恢复上限 = 官方备份 +
  本仓库 frozen `docker/.env` 的新鲜度。
- **union 注入的可见性放宽**：一次 `infisical run` 把 `/` 全集给
  `docker compose up` 进程，各 service 容器能看到超出自己子集的 env
  （如 api 能看到 `AUTH_PG_PASSWORD`）；真正的隔离仍靠 DB role（已存在），
  env 可见性按“同宿主发布单元”接受放宽，不接受再拆多次注入（会引入时序 skew）。
- **seat 费用随人数涨**：成员增加即加钱（以官网为准）；离职不及时清 seat 等于白付费。
- **Cloud 侧 RBAC 误配**：prod 写权限给错人 = 高敏 key 被改无 PR 留痕；
  缓解只有 §4 的最小授权 + 每月 Audit Logs 抽查，无代码门禁可拦。

## 11. 前端 env（Cloudflare Pages 托管 + 构建烘焙）

前端 env 与后端 secret 是两条通道：Vite 把 `VITE_*` 在构建时烘焙进公开
bundle（一经发到浏览器即公开），Cloudflare Pages Functions（`apps/web/functions/`）
是跑在边缘的服务端代码，读 Cloudflare Secret binding。两边都不走
`docker compose` 注入，本节是前端唯一的接线说明。

### 11.1 构建时烘焙清单（全仓 grep 结论）

生产 bundle 只烘焙以下两个公开键（`apps/web/src/env.d.ts` 为 schema 事实源，
`scripts/web-frontend-env-check.sh` 门禁三方对齐：声明/使用点/文档）：

| Key | 使用点 | 生产值 | 说明 |
| --- | ------ | ------ | ---- |
| `VITE_API_BASE_URL` | `src/core/config/runtimeUrls.ts`（生产强制收敛到 `/api`） | 空 | 同域 Gateway；生产 dashboard 必须保持空，防止陈旧绝对地址复活 CORS |
| `VITE_CANVAS_PERF_PROBE` | `src/features/canvas/session/browser/canvasPerfProbe.ts`（仅 `DEV` 生效） | 未设置 | 本地 profiling 临时开关；生产 bundle 即使误烘焙也因 `DEV=false` 失效 |

无 secret（红线）：全仓 `VITE_*` 无 `KEY/SECRET/TOKEN/PASSWORD/PRIVATE/CREDENTIAL`
片段；`apps/web/src` 无 `import.meta.env.PUBLIC_*` 读取（Vite 只暴露 `VITE_`
命名空间，后端 `PUBLIC_ORIGIN` 前端经同域网关到达，永不经 env 传入）；
`PUBLIC_SITE_ORIGIN` / `PUBLIC_LEGAL_LINKS` 是前端 TS 常量而非 env。

命名约定（已决策）：前端公开键统一 `VITE_` 前缀——这是 Vite 暴露给浏览器的
硬要求。不引入 `VITE_PUBLIC_` 双前缀：所有 `VITE_*` 本就公开，双前缀不增加
安全性，只增加拼写负担。新键必须进 `env.d.ts` + 使用点 + `.env.example`
三处（门禁强制），键名不得含 secret 片段（门禁红线）。

### 11.2 Pages 构建注入链（仿现有形态，不自创）

注入形态与 §3 同形，唯一入口仍是 `scripts/infisical-run.sh`（union 注入对
Vite 无害：Vite 只读 `VITE_*`，其余 key 直接忽略）：

```bash
# 本地/验证构建（cwd=apps/web；VITE_* 取空即同域形态，与生产一致）
scripts/infisical-run.sh --env dev -- npm run build          # apps/web 下执行
# 未来某 VITE_* 需要非空生产值：先加进 Infisical 对应 env，
# 再用同一形态构建，无需改 workflow：
scripts/infisical-run.sh --env prod -- npm run build         # apps/web 下执行
```

- Cloudflare Pages（dashboard git 构建）：`VITE_API_BASE_URL` 留空，
  `VITE_CANVAS_PERF_PROBE` 不设置；`deployment.md` 的 Pages 环境隔离表为准。
- CI 的 Web lane（`web.yml`）**不接 Infisical**：与 §5 同一理由，gate 读远端
  会把 CI 变成对外部服务的可用性依赖。CI 构建本就无 `VITE_*`（空值同域），
  与生产一致；门禁只跑离线的 `web-frontend-env-check.sh`。
- `functions/` 服务端键（选边：**Cloudflare Secret 为准**，Infisical 只做值源）：
  唯一 binding 是 `IDEABOARD_API_ORIGIN`（`functions/_proxy.ts`，
  要求 HTTPS、无 retired-host fallback）。Functions 跑在 Cloudflare 边缘，
  `infisical run` 在请求时无法注入，运行时读的只能是 Cloudflare Pages
  Function Secret（dashboard）。轮换时改 dashboard 并重 deploy Pages；
  不要两边各改各的——运行时永远以 dashboard 为准。
  已知冲突（待 parent 定夺）：`infisical-drift-check.sh`（后端 schema 宇宙）
  把 dev 里已有的 `IDEABOARD_API_ORIGIN` 报为 unknown key。若保留 Infisical
  值源副本，需在该 checker 加前端已知名单；若不加，则删掉 dev 里的该 key
  回归 dashboard-only。无论选哪边，运行时通道不变（dashboard），
  本节运行时结论不受影响。

### 11.3 Cloud 对账（key 名，2026-10-19 `infisical export` 实测）

| Key | dev（25 keys） | staging（0 keys） | prod（24 keys） | 结论 |
| --- | --- | --- | --- | ---- |
| `VITE_API_BASE_URL` | 缺 | 缺 | 缺 | 符合预期（§2.1 不进表：生产值为空，不存） |
| `VITE_CANVAS_PERF_PROBE` | 缺 | 缺 | 缺 | 符合预期（dev-only 开关，不进 Cloud） |
| `IDEABOARD_API_ORIGIN` | 有 | 缺 | **缺** | 上报：prod Pages Functions 运行时需要该 binding；先在 Cloudflare dashboard 配好 prod 值（当前 VPS API HTTPS origin），再补入 Infisical prod 作值源。另见 §11.2：dev 现有副本被 `infisical-drift-check.sh` 报 unknown，值源去留待定夺 |

staging 为空是后端迁入事项（本节无前端动作：`VITE_*` 本就不进 Cloud；
preview 按 `deployment.md` 不连生产 API/Auth/DB）。

### 11.4 漂移门

`scripts/web-frontend-env-check.sh`（自测 `web-frontend-env-check-test.sh`，
经 `run-all-self-tests.sh` 自动拾取；CI 在 `web.yml` build job 的
`Frontend env allowlist` step 执行；`ops.yml` 覆盖其 `bash -n`/ShellCheck）：

1. 使用点 == 声明（`import.meta.env.VITE_*` + `vi.stubEnv` vs `env.d.ts`，双向）；
2. 声明 == 文档（`env.d.ts` vs `.env.example`，双向）；
3. 键名无 secret 片段（红线，见 11.1）；
4. web 源码禁读 `import.meta.env.PUBLIC_*`；
5. `.env.example` 的 `VITE_*` 行必须无提交值；
6. `functions/` 只允许 `IDEABOARD_API_ORIGIN` binding（加新 binding 先改本 runbook）。
