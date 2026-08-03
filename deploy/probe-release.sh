#!/bin/sh

# Strict pre-cutover probe for an already configured Sub2API release.
# It deliberately checks both process/dependency health and the embedded SPA so
# a non-embed backend cannot pass deployment validation with /health alone.

set -eu

usage() {
    echo "Usage: $0 <base-url> [--skip-ready|--setup]" >&2
    exit 2
}

[ "$#" -ge 1 ] || usage

BASE_URL=${1%/}
shift
CHECK_READY=true
SETUP_MODE=false
while [ "$#" -gt 0 ]; do
    case "$1" in
        --skip-ready)
            CHECK_READY=false
            ;;
        --setup)
            CHECK_READY=false
            SETUP_MODE=true
            ;;
        *)
            usage
            ;;
    esac
    shift
done

case "$BASE_URL" in
    http://*|https://*) ;;
    *)
        echo "Probe base URL must start with http:// or https://" >&2
        exit 2
        ;;
esac

CONNECT_TIMEOUT=${SUB2API_PROBE_CONNECT_TIMEOUT:-3}
MAX_TIME=${SUB2API_PROBE_MAX_TIME:-10}
ADMIN_PATH=${SUB2API_PROBE_ADMIN_PATH:-/admin}
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-release-probe.XXXXXX")
trap 'rm -rf "$TMP_DIR"' EXIT HUP INT TERM

fail() {
    echo "release probe failed: $*" >&2
    exit 1
}

fetch() {
    probe_name=$1
    probe_path=$2
    body_file="$TMP_DIR/${probe_name}.body"
    header_file="$TMP_DIR/${probe_name}.headers"

    http_code=$(curl --silent --show-error --location --compressed \
        --connect-timeout "$CONNECT_TIMEOUT" --max-time "$MAX_TIME" \
        --dump-header "$header_file" --output "$body_file" \
        --write-out '%{http_code}' "${BASE_URL}${probe_path}") ||
        fail "request ${probe_path} could not be completed"

    [ "$http_code" = "200" ] || fail "${probe_path} returned HTTP ${http_code}"
    [ -s "$body_file" ] || fail "${probe_path} returned an empty body"
}

assert_html() {
    probe_name=$1
    probe_path=$2
    if ! tr -d '\r' < "$TMP_DIR/${probe_name}.headers" | grep -Eiq '^content-type:[[:space:]]*text/html'; then
        fail "${probe_path} did not return text/html"
    fi
    grep -Eq '<div[[:space:]][^>]*id="app"' "$TMP_DIR/${probe_name}.body" ||
        fail "${probe_path} is not the embedded Sub2API SPA"
}

if [ "$SETUP_MODE" = true ]; then
    fetch setup /setup/status
    grep -Eq '"needs_setup"[[:space:]]*:[[:space:]]*true' "$TMP_DIR/setup.body" ||
        fail "/setup/status did not confirm setup mode"
else
    fetch health /health
    grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' "$TMP_DIR/health.body" ||
        fail "/health response is not healthy"
fi

if [ "$CHECK_READY" = true ]; then
    fetch ready /ready
    grep -Eq '"status"[[:space:]]*:[[:space:]]*"ready"' "$TMP_DIR/ready.body" ||
        fail "/ready response is not ready"
    grep -Eq '"database"[[:space:]]*:[[:space:]]*"ok"' "$TMP_DIR/ready.body" ||
        fail "/ready did not confirm database connectivity"
fi

fetch root /
assert_html root /

fetch admin "$ADMIN_PATH"
assert_html admin "$ADMIN_PATH"

grep -Eo '(src|href)="[^"]+\.(js|css)(\?[^"]*)?"' "$TMP_DIR/root.body" |
    sed -E 's/^(src|href)="([^"]+)"$/\2/' |
    grep '^/assets/' |
    sort -u > "$TMP_DIR/assets" || true

[ -s "$TMP_DIR/assets" ] || fail "SPA HTML did not reference hashed /assets JavaScript or CSS"
grep -Eq '\.js(\?|$)' "$TMP_DIR/assets" || fail "SPA HTML did not reference a JavaScript asset"
grep -Eq '\.css(\?|$)' "$TMP_DIR/assets" || fail "SPA HTML did not reference a CSS asset"

asset_index=0
while IFS= read -r asset_path; do
    asset_index=$((asset_index + 1))
    fetch "asset-${asset_index}" "$asset_path"
    normalized_headers=$(tr -d '\r' < "$TMP_DIR/asset-${asset_index}.headers")
    case "$asset_path" in
        *.js|*.js\?*)
            printf '%s\n' "$normalized_headers" | grep -Eiq '^content-type:.*(javascript|ecmascript)' ||
                fail "${asset_path} did not return a JavaScript content type"
            ;;
        *.css|*.css\?*)
            printf '%s\n' "$normalized_headers" | grep -Eiq '^content-type:[[:space:]]*text/css' ||
                fail "${asset_path} did not return text/css"
            ;;
    esac
done < "$TMP_DIR/assets"

if [ "$SETUP_MODE" = true ]; then
    echo "setup probe passed: setup status, SPA/admin, and ${asset_index} static assets"
else
    echo "release probe passed: liveness, readiness, SPA/admin, and ${asset_index} static assets"
fi
