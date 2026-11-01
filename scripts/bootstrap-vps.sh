#!/usr/bin/env bash

set +x
set -euo pipefail
export LC_ALL=C

RELEASE_REF="${IDEABOARD_RELEASE_REF:-}"
MANIFEST="${IDEABOARD_RELEASE_MANIFEST:-}"
ARCHIVE="${IDEABOARD_DEPLOYMENT_BUNDLE_ARCHIVE:-}"
SIGNATURE="${IDEABOARD_DEPLOYMENT_BUNDLE_SIGNATURE:-}"
ENTRYPOINT="${IDEABOARD_PRODUCTION_ENTRYPOINT:-/usr/local/libexec/ideaboard-production-deploy}"

usage() {
  cat <<'EOF'
Usage: bootstrap-vps.sh --ref <vX.Y.Z> --manifest <path> --bundle <path> --signature <path>

Deploy local signed release artifacts through the independently installed trusted
Production entrypoint. INFISICAL_TOKEN is required. This command never clones a
repository or installs executable code. Provision Docker, Infisical CLI, Cosign,
Python 3, jq, flock, the root-owned verifier and entrypoint before running it.
EOF
}
while (( $# > 0 )); do
  case "$1" in
    --ref|--manifest|--bundle|--signature)
      (( $# >= 2 )) || { printf 'Missing value for %s\n' "$1" >&2; exit 2; }
      case "$1" in
        --ref) RELEASE_REF="$2" ;;
        --manifest) MANIFEST="$2" ;;
        --bundle) ARCHIVE="$2" ;;
        --signature) SIGNATURE="$2" ;;
      esac
      shift 2
      ;;
    --help|-h) usage; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done
[[ -n "$RELEASE_REF" ]] || { echo '--ref <vX.Y.Z> is required' >&2; exit 2; }
[[ "$RELEASE_REF" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
  echo '--ref must be a strict SemVer tag' >&2; exit 2;
}
for artifact in "$MANIFEST" "$ARCHIVE" "$SIGNATURE"; do
  [[ -f "$artifact" && -s "$artifact" ]] || { echo '--manifest, --bundle and --signature require nonempty local files' >&2; exit 2; }
done
[[ -x "$ENTRYPOINT" ]] || { echo 'Independently installed Production entrypoint is unavailable' >&2; exit 2; }
python3 - "$ENTRYPOINT" <<'PYTHON' || { echo 'Production entrypoint must be a trusted local file' >&2; exit 2; }
import os, pathlib, stat, sys
try:
    info=pathlib.Path(sys.argv[1]).lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o022:
        raise ValueError()
except (OSError,ValueError):
    sys.exit(1)
PYTHON
if [[ -z "${INFISICAL_TOKEN:-}" || ${#INFISICAL_TOKEN} -gt 8192 || "$INFISICAL_TOKEN" == *[![:graph:]]* ]]; then
  echo 'A nonempty, single-line Production Infisical token is required' >&2; exit 2
fi
header="$(python3 - "$MANIFEST" "$ARCHIVE" "$SIGNATURE" "$RELEASE_REF" <<'PYTHON'
import json, pathlib, re, sys
try:
    manifest=json.loads(pathlib.Path(sys.argv[1]).read_text())
    commit=manifest['commit']
    if manifest['tag'] != sys.argv[4] or not isinstance(commit,str) or not re.fullmatch(r'[0-9a-f]{40}',commit):
        raise ValueError()
    a,s=(pathlib.Path(p).stat().st_size for p in sys.argv[2:4])
    if not 0<a<=67108864 or not 0<s<=1048576:
        raise ValueError()
    print(json.dumps(dict(tag=sys.argv[4],commit=commit,archive_bytes=a,signature_bytes=s),separators=(',',':')))
except (OSError,ValueError,KeyError,TypeError):
    sys.exit(1)
PYTHON
)" || { echo 'Invalid local signed release identity or size' >&2; exit 2; }
{
  printf '%s\n' IDEABOARD_DEPLOY_V2 "$INFISICAL_TOKEN" "$header"
  cat < "$ARCHIVE"
  cat < "$SIGNATURE"
} | env -u INFISICAL_TOKEN "$ENTRYPOINT"
