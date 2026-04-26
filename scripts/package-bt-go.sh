#!/usr/bin/env bash

set -euo pipefail

cat >&2 <<'EOF'
package-bt-go.sh has been disabled.

Better Auth now runs as an independent Node service under apps/auth, so the old
single Go binary Baota bundle would produce a broken login/register flow.

Use the standard deployment bundle instead:

  ./scripts/package-deploy.sh

See DEPLOYMENT.md for the two-service topology and Nginx routing.
EOF

exit 1
