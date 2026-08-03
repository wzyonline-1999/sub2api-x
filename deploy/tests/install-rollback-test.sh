#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

# Load only the probe/activation functions. This keeps the test portable to the
# macOS system Bash while install.sh itself intentionally requires Bash 4+ for
# its localization tables.
sed -n '/^resolve_service_probe_url()/,/^# Enable service auto-start/p' "$ROOT_DIR/deploy/install.sh" |
    sed '$d' > "$TEMP_DIR/functions.sh"
source "$TEMP_DIR/functions.sh"

print_info() { :; }
print_success() { :; }
print_warning() { :; }
print_error() { printf '%s\n' "$*" >> "$TEMP_DIR/errors.log"; }
msg() { printf '%s' "$1"; }
chown() { :; }
systemctl() {
    printf '%s\n' "$*" >> "$TEMP_DIR/systemctl.log"
    if [ "${1:-}" = "show" ]; then
        printf '%s\n' 'GIN_MODE=release SERVER_HOST=0.0.0.0 SERVER_PORT=19090'
    fi
    return 0
}

INSTALL_DIR="$TEMP_DIR/install"
SERVICE_NAME="sub2api"
SERVICE_USER="sub2api"
SERVER_PORT=18080
PROBE_ATTEMPTS=1
PROBE_INTERVAL_SECONDS=0
mkdir -p "$INSTALL_DIR"

cat > "$INSTALL_DIR/probe-release.sh" <<'EOF'
#!/bin/sh
test "${1:-}" = 'http://127.0.0.1:19090' || exit 2
case " $* " in
    *' --skip-ready '*) exit 0 ;;
    *) exit 1 ;;
esac
EOF
chmod +x "$INSTALL_DIR/probe-release.sh"

printf 'new-release' > "$INSTALL_DIR/sub2api"
printf 'previous-release' > "$INSTALL_DIR/sub2api.backup.test"

if activate_release_or_rollback "$INSTALL_DIR/sub2api.backup.test"; then
    echo "failed release unexpectedly reported success" >&2
    exit 1
fi

test "$(cat "$INSTALL_DIR/sub2api")" = 'previous-release'
grep -Fxq 'stop sub2api' "$TEMP_DIR/systemctl.log"
test "$(grep -Fxc 'start sub2api' "$TEMP_DIR/systemctl.log")" -eq 2

echo "installer automatic rollback checks passed"
