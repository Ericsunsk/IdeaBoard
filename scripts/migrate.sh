#!/usr/bin/env bash
# migrate.sh
#
# 本地一键迁移：API → Auth 强制顺序，init-admin 同形态。
#
#   scripts/migrate.sh --env dev
#   scripts/migrate.sh --env dev --only api
#   scripts/migrate.sh --env dev --only auth
#   scripts/migrate.sh --env dev --only init-admin \
#     --admin-email 'admin@example.com' --admin-password '...' [--display-name Admin]
#
# Secret 双通道（合并、无 scrub、不走 dotenvx）：
#   Cloud 通道 = `infisical export --format=dotenv` 落 0600 临时文件再
#     `set -a; source`（`infisical run` 会净空父 env，owner 口令过不去，故不用）。
#   owner 通道 = docker/migration.env（本机唯一 owner 凭据，不进 Cloud）。
# 先 source Cloud，再 unset Cloud 残留的 owner key，最后 source migration.env，
# 因此 PG_MIGRATION_* 只可能来自本地文件。
#
# 开跑前预检（fail fast，不进 60 次重试）：
#   owner 与运行时 role 直连 SELECT 1；运行时口令对不上报
#   “Cloud 口令与 volume 指纹不一致”；Welcome key 指纹对 guard 表比对，
#   不一致提前报，guard 表不存在（fresh DB）或算不出指纹时跳过并注明。
# 预检前先等 postgres accept 连接（pg_isready 探活，只等启动不校验口令，
# 口令错配仍由预检 fail fast 报出）：compose `up -d` 返回时库仍在启动是常态。
#
# Portable to bash 3.2 (macOS /usr/bin/bash): no assoc arrays, no mapfile.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/lib/common.sh"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATION_ENV_FILE="${REPO_ROOT}/docker/migration.env"

ENV_NAME=""
ONLY=""
ADMIN_EMAIL=""
ADMIN_PASSWORD=""
ADMIN_DISPLAY_NAME=""

usage() {
  cat <<'EOF'
usage: scripts/migrate.sh --env <dev|staging|prod> [--only <api|auth|init-admin>]
       [--admin-email <email> --admin-password <password> [--display-name <name>]]
  --env dev                  # api→auth 一次跑完（强制顺序）
  --env dev --only api       # 只跑 API migration
  --env dev --only auth      # 只跑 Auth migration
  --env dev --only init-admin --admin-email a@b.c --admin-password '...' [--display-name Admin]
                             # 初始化管理员（跑完自动 unset INIT_ADMIN_*；需 Auth Service 已启动）
EOF
}

while (($# > 0)); do
  case "$1" in
    --env)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      ENV_NAME="$2"
      shift 2
      ;;
    --env=*)
      ENV_NAME="${1#--env=}"
      shift
      ;;
    --only)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      ONLY="$2"
      shift 2
      ;;
    --only=*)
      ONLY="${1#--only=}"
      shift
      ;;
    --admin-email)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      ADMIN_EMAIL="$2"
      shift 2
      ;;
    --admin-email=*)
      ADMIN_EMAIL="${1#--admin-email=}"
      shift
      ;;
    --admin-password)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      ADMIN_PASSWORD="$2"
      shift 2
      ;;
    --admin-password=*)
      ADMIN_PASSWORD="${1#--admin-password=}"
      shift
      ;;
    --display-name)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      ADMIN_DISPLAY_NAME="$2"
      shift 2
      ;;
    --display-name=*)
      ADMIN_DISPLAY_NAME="${1#--display-name=}"
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
done

[[ -n "$ENV_NAME" ]] || { usage >&2; exit 2; }
case "$ENV_NAME" in
  dev | staging | prod) ;;
  *) echo "error: --env must be one of dev|staging|prod (got '$ENV_NAME')" >&2; exit 2 ;;
esac

RUN_API=1
RUN_AUTH=1
RUN_INIT_ADMIN=0
if [[ -n "$ONLY" ]]; then
  case "$ONLY" in
    api) RUN_AUTH=0 ;;
    auth) RUN_API=0 ;;
    init-admin) RUN_API=0; RUN_AUTH=0; RUN_INIT_ADMIN=1 ;;
    *) echo "error: --only must be one of api|auth|init-admin (got '$ONLY')" >&2; exit 2 ;;
  esac
fi

if ((RUN_INIT_ADMIN == 1)); then
  [[ -n "$ADMIN_EMAIL" ]] || die "--only init-admin requires --admin-email"
  [[ -n "$ADMIN_PASSWORD" ]] || die "--only init-admin requires --admin-password"
fi
if [[ -n "$ADMIN_EMAIL$ADMIN_PASSWORD$ADMIN_DISPLAY_NAME" ]] && ((RUN_INIT_ADMIN == 0)); then
  die "--admin-email/--admin-password/--display-name only apply with --only init-admin"
fi

command -v infisical >/dev/null 2>&1 || die "infisical CLI 未安装（npm i -g @infisical/cli；再 infisical login）"
command -v psql >/dev/null 2>&1 || die "psql 未安装（macOS：brew install postgresql@17）"
if ((RUN_API == 1 || RUN_INIT_ADMIN == 1)); then
  command -v go >/dev/null 2>&1 || die "go 未安装"
  command -v atlas >/dev/null 2>&1 || die "atlas 未安装（版本以 DEVELOPMENT.md 为准）"
fi
if ((RUN_AUTH == 1)); then
  command -v npm >/dev/null 2>&1 || die "npm 未安装"
  [[ -d "${REPO_ROOT}/apps/auth/node_modules" ]] || die "apps/auth/node_modules 缺失，先 cd apps/auth && npm install"
fi

[[ -f "$MIGRATION_ENV_FILE" ]] || die "docker/migration.env 缺失（cd docker && cp migration.env.example migration.env，再填 owner 口令）"

# Cloud 全集落临时文件（0600），再 source 合并；不走 `infisical run`（净空父 env），不走 dotenvx。
cloud_tmp="$(mktemp "${TMPDIR:-/tmp}/ideaboard-infisical.XXXXXX")"
cleanup_path "$cloud_tmp"
export_args=(export "--env=${ENV_NAME}" --format=dotenv --path /)
if [[ -n "${INFISICAL_PROJECT_ID:-}" ]]; then
  export_args+=(--projectId "${INFISICAL_PROJECT_ID}")
fi
if [[ -n "${INFISICAL_DOMAIN:-}" ]]; then
  export_args+=(--domain "${INFISICAL_DOMAIN}")
fi
if ! infisical "${export_args[@]}" >"$cloud_tmp" 2>"${cloud_tmp}.err"; then
  cat "${cloud_tmp}.err" >&2
  rm -f "${cloud_tmp}.err"
  die "infisical export 失败（先 infisical login；CI/VPS 用 INFISICAL_TOKEN，见 secrets-infisical.md §4）"
fi
rm -f "${cloud_tmp}.err"

set -a
# shellcheck disable=SC1090 # 运行时落盘的 Cloud export 临时文件，路径动态。
source "$cloud_tmp"
set +a
# owner key 只认本地文件：Cloud 残留（若有）先清掉。
unset PG_MIGRATION_USER PG_MIGRATION_PASSWORD POSTGRES_PASSWORD
set -a
# shellcheck disable=SC1090 # 本机 owner 文件 docker/migration.env，路径动态。
source "$MIGRATION_ENV_FILE"
set +a
rm -f "$cloud_tmp"

PG_HOST="${PG_HOST:-127.0.0.1}"
PG_PORT="${PG_PORT:-5432}"
PG_DB="${PG_DB:-ideaboard}"
PG_SSLMODE="${PG_SSLMODE:-disable}"
API_PG_USER="${API_PG_USER:-ideaboard_api}"
AUTH_PG_USER="${AUTH_PG_USER:-ideaboard_auth}"
PG_MIGRATION_USER="${PG_MIGRATION_USER:-ideaboard}"

require_env PG_MIGRATION_PASSWORD
if ((RUN_API == 1 || RUN_INIT_ADMIN == 1)); then
  require_env API_PG_PASSWORD
fi
if ((RUN_AUTH == 1)); then
  require_env AUTH_PG_PASSWORD
fi

psql_owner() {
  PGPASSWORD="$PG_MIGRATION_PASSWORD" PGCONNECT_TIMEOUT=5 psql \
    -h "$PG_HOST" -p "$PG_PORT" -U "$PG_MIGRATION_USER" -d "$PG_DB" \
    -v ON_ERROR_STOP=1 -tA "$@"
}

psql_runtime() {
  local user="$1" password="$2"
  shift 2
  PGPASSWORD="$password" PGCONNECT_TIMEOUT=5 psql \
    -h "$PG_HOST" -p "$PG_PORT" -U "$user" -d "$PG_DB" \
    -v ON_ERROR_STOP=1 -tA "$@"
}

# 等 postgres accept 连接（60s 超时）。优先 pg_isready（只探活不碰口令）；
# 无 pg_isready 才回落到 owner 直连（此时口令错配会等满 60s 再由预检报错）。
wait_for_postgres() {
  local attempts=60 i=0
  log "migrate: 等 postgres ${PG_HOST}:${PG_PORT} 就绪…"
  if command -v pg_isready >/dev/null 2>&1; then
    while ((i < attempts)); do
      if pg_isready -h "$PG_HOST" -p "$PG_PORT" >/dev/null 2>&1; then
        log "migrate: postgres 就绪"
        return 0
      fi
      i=$((i + 1))
      sleep 1
    done
  else
    while ((i < attempts)); do
      if psql_owner -c 'SELECT 1' >/dev/null 2>&1; then
        log "migrate: postgres 就绪"
        return 0
      fi
      i=$((i + 1))
      sleep 1
    done
  fi
  die "postgres ${PG_HOST}:${PG_PORT}/${PG_DB} 60s 内未就绪（docker compose -f docker/compose.local.yml ps 确认 postgres 已 running 再重跑）"
}

# owner 直连预检：migration.env 口令必须能连上本地 volume。
preflight_owner_login() {
  if psql_owner -c 'SELECT 1' >/dev/null 2>&1; then
    log "preflight: owner ${PG_MIGRATION_USER} 登录 OK"
    return 0
  fi
  printf 'error: docker/migration.env 的 owner 口令连不上 %s:%s/%s（PG_MIGRATION_USER=%s）。检查 migration.env 的 PG_MIGRATION_PASSWORD 是否与 volume 初始化口令一致。\n' \
    "$PG_HOST" "$PG_PORT" "$PG_DB" "$PG_MIGRATION_USER" >&2
  exit 1
}

# 运行时 role 直连预检：Cloud 口令必须与 volume 指纹一致，否则直接报，不进 60 次重试。
preflight_runtime_login() {
  local user="$1" password="$2" key_name="$3"
  if psql_runtime "$user" "$password" -c 'SELECT 1' >/dev/null 2>&1; then
    log "preflight: runtime ${user} 登录 OK"
    return 0
  fi
  printf 'error: Cloud 口令与 volume 指纹不一致（%s 用 Cloud 口令连不上 %s:%s/%s）。对照 Cloud %s 与 docker/.env 解密值（dotenvx get %s -f docker/.env），或用 docker/postgres/init-runtime-roles.sh 重置 role 口令后再跑。\n' \
    "$user" "$PG_HOST" "$PG_PORT" "$PG_DB" "$ENV_NAME" "$key_name" >&2
  exit 1
}

# Welcome key 指纹预检：Cloud key 与 guard 表绑定指纹比对。表不存在（fresh DB）
# 或算不出指纹时跳过并注明；不一致则提前报。
preflight_welcome_fingerprint() {
  if [[ -z "${CREDITS_WELCOME_IDENTITY_KEY:-}" ]]; then
    log "preflight: 跳过 Welcome key 指纹比对（CREDITS_WELCOME_IDENTITY_KEY 未设置）"
    return 0
  fi
  local want=""
  if command -v python3 >/dev/null 2>&1; then
    want="$(CREDITS_WELCOME_IDENTITY_KEY="$CREDITS_WELCOME_IDENTITY_KEY" python3 -c \
      'import base64,hashlib,os; print(hashlib.sha256(b"ideaboard:welcome-identity-key:v1:"+base64.b64decode(os.environ["CREDITS_WELCOME_IDENTITY_KEY"])).hexdigest())' 2>/dev/null)" || want=""
  fi
  if [[ -z "$want" ]] && command -v openssl >/dev/null 2>&1; then
    local raw_tmp
    raw_tmp="$(mktemp "${TMPDIR:-/tmp}/ideaboard-welcome-key.XXXXXX")"
    cleanup_path "$raw_tmp"
    if printf '%s' "$CREDITS_WELCOME_IDENTITY_KEY" | openssl base64 -d -out "$raw_tmp" 2>/dev/null; then
      want="$({ printf 'ideaboard:welcome-identity-key:v1:'; cat "$raw_tmp"; } | openssl dgst -sha256 -r 2>/dev/null | awk '{print $1}')"
    fi
    rm -f "$raw_tmp"
  fi
  if [[ -z "$want" ]]; then
    log "preflight: 跳过 Welcome key 指纹比对（本机无 python3/openssl，算不出指纹）"
    return 0
  fi
  local got=""
  if ! got="$(psql_owner -c 'SELECT key_fingerprint FROM credit_welcome_identity_key_guard LIMIT 1' 2>/dev/null)"; then
    log "preflight: 跳过 Welcome key 指纹比对（guard 表不可读，多为 fresh DB，migrate 会创建它）"
    return 0
  fi
  got="$(printf '%s' "$got" | tr -d '[:space:]')"
  if [[ -z "$got" ]]; then
    log "preflight: 跳过 Welcome key 指纹比对（guard 表为空，首次绑定在 migrate/init-admin 时完成）"
    return 0
  fi
  if [[ "$want" == "$got" ]]; then
    log "preflight: Welcome key 指纹 OK"
    return 0
  fi
  printf 'error: Welcome key 指纹不一致（Cloud CREDITS_WELCOME_IDENTITY_KEY 算出 %s，库内 guard 表绑定 %s）。误换 key 会 fail closed，非泄露事故禁止轮换；泄露按 data-protection.md 数据事故处理。\n' \
    "$want" "$got" >&2
  exit 1
}

if ((RUN_API == 1 || RUN_AUTH == 1)); then
  wait_for_postgres
fi
if ((RUN_API == 1)); then
  preflight_owner_login
  preflight_runtime_login "$API_PG_USER" "$API_PG_PASSWORD" "API_PG_PASSWORD"
  preflight_welcome_fingerprint
elif ((RUN_INIT_ADMIN == 1)); then
  preflight_runtime_login "$API_PG_USER" "$API_PG_PASSWORD" "API_PG_PASSWORD"
  preflight_welcome_fingerprint
fi
if ((RUN_AUTH == 1)); then
  if ((RUN_API == 0)); then
    preflight_owner_login
  fi
  preflight_runtime_login "$AUTH_PG_USER" "$AUTH_PG_PASSWORD" "AUTH_PG_PASSWORD"
fi

if ((RUN_API == 1)); then
  log "migrate: API migration…"
  (cd "${REPO_ROOT}/apps/api" && go run ./cmd migrate)
  log "migrate: API 完成"
fi

if ((RUN_AUTH == 1)); then
  log "migrate: Auth migration…"
  (cd "${REPO_ROOT}/apps/auth" && npm run migrate)
  log "migrate: Auth 完成"
fi

if ((RUN_INIT_ADMIN == 1)); then
  log "migrate: init-admin…"
  export INIT_ADMIN_EMAIL="$ADMIN_EMAIL"
  export INIT_ADMIN_PASSWORD="$ADMIN_PASSWORD"
  if [[ -n "$ADMIN_DISPLAY_NAME" ]]; then
    export INIT_ADMIN_DISPLAY_NAME="$ADMIN_DISPLAY_NAME"
  else
    unset INIT_ADMIN_DISPLAY_NAME
  fi
  status=0
  (cd "${REPO_ROOT}/apps/api" && go run ./cmd init-admin) || status=$?
  unset INIT_ADMIN_EMAIL INIT_ADMIN_PASSWORD INIT_ADMIN_DISPLAY_NAME
  ((status == 0)) || exit "$status"
  log "migrate: init-admin 完成（INIT_ADMIN_* 已 unset）"
fi

log "migrate: 全部完成"
