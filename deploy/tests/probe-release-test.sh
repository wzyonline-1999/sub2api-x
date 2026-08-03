#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

cat > "$TEMP_DIR/curl" <<'EOF'
#!/bin/bash
set -euo pipefail

headers=""
body=""
url=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        --connect-timeout|--max-time|--write-out)
            shift 2
            ;;
        --dump-header)
            headers=$2
            shift 2
            ;;
        --output)
            body=$2
            shift 2
            ;;
        --silent|--show-error|--location|--compressed)
            shift
            ;;
        http://*|https://*)
            url=$1
            shift
            ;;
        *)
            echo "unexpected curl argument: $1" >&2
            exit 2
            ;;
    esac
done

path=${url#http://probe.test}
printf '%s\n' "$path" >> "$FAKE_CURL_LOG"
status=200
content_type="application/json"
response='{}'
case "$path" in
    /health)
        response='{"status":"ok"}'
        ;;
    /ready)
        if [ "${FAKE_READY_FAIL:-}" = "1" ]; then
            status=503
            response='{"status":"not_ready","checks":{"database":"unavailable"}}'
        else
            response='{"status":"ready","checks":{"database":"ok"}}'
        fi
        ;;
    /setup/status)
        response='{"code":0,"data":{"needs_setup":true}}'
        ;;
    /|/admin)
        content_type="text/html; charset=utf-8"
        response='<html><head><script type="module" src="/assets/app-123.js"></script><link rel="stylesheet" href="/assets/app-123.css"></head><body><div id="app"></div></body></html>'
        ;;
    /assets/app-123.js)
        content_type="application/javascript"
        response='console.log("ok")'
        ;;
    /assets/app-123.css)
        content_type="text/css"
        response='body{display:block}'
        ;;
    *)
        status=404
        response='not found'
        content_type="text/plain"
        ;;
esac

printf 'HTTP/1.1 %s Test\r\nContent-Type: %s\r\n\r\n' "$status" "$content_type" > "$headers"
printf '%s' "$response" > "$body"
printf '%s' "$status"
EOF
chmod +x "$TEMP_DIR/curl"

export PATH="$TEMP_DIR:$PATH"
export FAKE_CURL_LOG="$TEMP_DIR/curl.log"

"$ROOT_DIR/deploy/probe-release.sh" http://probe.test > "$TEMP_DIR/release.out"
grep -Fq 'release probe passed' "$TEMP_DIR/release.out"
grep -Fxq '/ready' "$FAKE_CURL_LOG"
grep -Fxq '/admin' "$FAKE_CURL_LOG"
grep -Fxq '/assets/app-123.js' "$FAKE_CURL_LOG"
grep -Fxq '/assets/app-123.css' "$FAKE_CURL_LOG"

if FAKE_READY_FAIL=1 "$ROOT_DIR/deploy/probe-release.sh" http://probe.test >/dev/null 2>&1; then
    echo "release probe accepted a failed database readiness check" >&2
    exit 1
fi

: > "$FAKE_CURL_LOG"
"$ROOT_DIR/deploy/probe-release.sh" http://probe.test --setup > "$TEMP_DIR/setup.out"
grep -Fq 'setup probe passed' "$TEMP_DIR/setup.out"
grep -Fxq '/setup/status' "$FAKE_CURL_LOG"
if grep -Eq '^/(health|ready)$' "$FAKE_CURL_LOG"; then
    echo "setup probe unexpectedly required normal-server health endpoints" >&2
    exit 1
fi

echo "strict release probe checks passed"
