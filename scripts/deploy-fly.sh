#!/usr/bin/env bash
set -euo pipefail

version=${1:?usage: deploy-fly.sh VERSION IMAGE_DIGEST}
digest=${2:?usage: deploy-fly.sh VERSION IMAGE_DIGEST}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ && "$version" != v0.* && "$version" != v1.* ]]
[[ "$digest" =~ ^sha256:[a-f0-9]{64}$ ]]
: "${FLY_API_TOKEN:?Set an app-scoped Fly deployment token}"

config_dir=$(mktemp -d)
trap 'rm -rf "$config_dir"' EXIT
# Keep the live ports, volume, budgets and VM settings when updating the image.
flyctl config show --app echo-observatory --toml > "$config_dir/fly.toml"
python3 - "$config_dir/fly.toml" <<'PY'
import sys
import tomllib

with open(sys.argv[1], "rb") as source:
    machines = tomllib.load(source).get("vm", [])
setting = "sysctl.net.ipv4.ip_unprivileged_port_start=443"
if not machines or any(setting not in vm.get("kernel_args", []) for vm in machines):
    sys.exit(f"Refusing deployment: every Fly VM must retain kernel_args = ['{setting}'] for non-root HTTP/3.")
PY
flyctl deploy --app echo-observatory --config "$config_dir/fly.toml" \
  --image "ghcr.io/philipschmid/echo-observatory@$digest" \
  --ha=false --update-only --strategy rolling --yes

flyctl status --app echo-observatory --json | jq -e '
  .Machines | length > 0 and all(.[];
    .state == "started" and
    (.checks | length > 0 and all(.[]; .status == "passing")))'
curl -fsS --retry 10 --retry-all-errors --retry-delay 3 --connect-timeout 5 --max-time 10 \
  https://echo.container.education/ui/ | grep -F "Version </span>$version</span>" >/dev/null
