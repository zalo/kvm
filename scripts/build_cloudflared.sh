#!/usr/bin/env bash
#
# build_cloudflared.sh — cross-compiles cloudflared from the vendored
# git submodule (vendor/cloudflared) into internal/tunnel/dist/cloudflared.
#
# That output gets embedded into jetkvm_app via //go:embed at the next
# `go build`, so the firmware ships with cloudflared bytes inside it —
# no separate file to deploy, no runtime download.
#
# Why we vendor: the trust boundary on a private gaming-station fork is
# "everything we ship was built from source we control or have audited".
# The submodule is pinned to a tag (commit), reproducible, and lets us
# bump versions deliberately.
#
# Run after a fresh clone: `git submodule update --init --recursive`
# then this script. The Makefile invokes it as part of build_dev.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VENDOR_DIR="${REPO_ROOT}/third_party/cloudflared"
OUT_DIR="${REPO_ROOT}/internal/tunnel/dist"
OUT="${OUT_DIR}/cloudflared"

if [ ! -d "$VENDOR_DIR/.git" ] && [ ! -f "$VENDOR_DIR/.git" ]; then
    cat <<EOF >&2
ERROR: third_party/cloudflared submodule not initialized.
Run: git submodule update --init --recursive
EOF
    exit 1
fi

if [ ! -f "${VENDOR_DIR}/cmd/cloudflared/main.go" ]; then
    echo "ERROR: third_party/cloudflared looks empty (no cmd/cloudflared/main.go)" >&2
    exit 1
fi

mkdir -p "$OUT_DIR"

# Skip work if the binary is already up to date relative to the
# submodule's HEAD commit. Crude but cheap.
if [ -f "$OUT" ] && [ -f "${OUT}.commit" ]; then
    have=$(cat "${OUT}.commit")
    want=$(cd "$VENDOR_DIR" && git rev-parse HEAD)
    if [ "$have" = "$want" ]; then
        echo "✓ ${OUT} already built from cloudflared@$want"
        exit 0
    fi
fi

cd "$VENDOR_DIR"
COMMIT=$(git rev-parse HEAD)
SHORT=$(git rev-parse --short HEAD)
echo "▶ Cross-compiling cloudflared @ ${SHORT} for linux/arm/v7"

# Static build, stripped. CGO off — keeps us off the JetKVM's libc/glibc
# split and gives us a binary that works on uClibc out of the box.
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
    go build -ldflags="-s -w" -trimpath -o "$OUT" ./cmd/cloudflared

echo "$COMMIT" > "${OUT}.commit"
size=$(stat -c %s "$OUT" 2>/dev/null || stat -f %z "$OUT")
echo "✓ ${OUT} (${size} bytes) built from cloudflared@${SHORT}"
