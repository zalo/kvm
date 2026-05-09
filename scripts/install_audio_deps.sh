#!/usr/bin/env bash
#
# install_audio_deps.sh — cross-compile ALSA + Opus static libs for the
# JetKVM ARM target so the Rockchip audio CGO bridge in internal/audio
# can link against them.
#
# What this script does (~5–10 minutes on first run):
#   1. Sources ARM toolchain from /opt/jetkvm-native-buildkit (or
#      $BUILDKIT_PATH if you've installed it elsewhere)
#   2. Downloads ALSA 1.2.14 and Opus 1.5.2 tarballs
#   3. Cross-compiles both as static .a libs into /opt/jetkvm-audio-libs
#      (or $AUDIO_LIBS_DIR)
#   4. The CGO build (`make build_dev`) then links those libs in
#
# Why an admin (sudo) is needed: /opt/jetkvm-audio-libs is the path
# baked into internal/audio/cgo_source.go's #cgo flags. We could let
# you install elsewhere via $AUDIO_LIBS_DIR, but you'd then have to
# patch the .go file. Easier to just write to /opt.
#
# Run as a user with sudo (the script wraps relevant steps with `sudo`),
# OR as root.
#
# Usage:
#   ./scripts/install_audio_deps.sh           # default versions
#   ALSA_VERSION=1.2.14 OPUS_VERSION=1.5.2 \
#     ./scripts/install_audio_deps.sh         # pin versions
#   AUDIO_LIBS_DIR=$HOME/jetkvm-audio-libs \
#     ./scripts/install_audio_deps.sh         # non-/opt install (also
#                                                requires patching
#                                                cgo_source.go's paths)
#
set -euo pipefail

ALSA_VERSION="${ALSA_VERSION:-1.2.14}"
OPUS_VERSION="${OPUS_VERSION:-1.5.2}"
AUDIO_LIBS_DIR="${AUDIO_LIBS_DIR:-/opt/jetkvm-audio-libs}"
BUILDKIT_PATH="${BUILDKIT_PATH:-/opt/jetkvm-native-buildkit}"
BUILDKIT_FLAVOR="${BUILDKIT_FLAVOR:-arm-rockchip830-linux-uclibcgnueabihf}"
JOBS="${JOBS:-$(nproc 2>/dev/null || echo 2)}"

CROSS_PREFIX="${BUILDKIT_PATH}/bin/${BUILDKIT_FLAVOR}"
CC="${CROSS_PREFIX}-gcc"
AR="${CROSS_PREFIX}-ar"
RANLIB="${CROSS_PREFIX}-ranlib"
STRIP="${CROSS_PREFIX}-strip"

# Fail loudly if the toolchain isn't where we expect.
if [ ! -x "$CC" ]; then
    cat <<EOF >&2
ERROR: ARM cross-compiler not found at $CC

The JetKVM native buildkit is required first. Install it via the
devcontainer setup script:

    sudo .devcontainer/install-deps.sh

or set BUILDKIT_PATH to where you installed it (currently:
$BUILDKIT_PATH).
EOF
    exit 1
fi

# sudo wrapper — only needed when the install path requires elevation.
# We probe by trying to mkdir; if that succeeds without sudo, we're done.
SUDO=""
if [ "$(id -u)" -ne 0 ]; then
    if ! mkdir -p "$AUDIO_LIBS_DIR" 2>/dev/null; then
        # Can't write directly — fall back to sudo if available.
        if command -v sudo >/dev/null 2>&1; then
            SUDO="sudo"
        else
            cat <<EOF >&2
ERROR: cannot create $AUDIO_LIBS_DIR as $(whoami) and sudo isn't
available. Either run this script as root, install sudo, or override
AUDIO_LIBS_DIR to a path your user owns (e.g.
\$HOME/jetkvm-audio-libs) and patch the #cgo paths in
internal/audio/cgo_source.go to match.
EOF
            exit 1
        fi
    fi
fi

echo "▶ ALSA      : ${ALSA_VERSION}"
echo "▶ Opus      : ${OPUS_VERSION}"
echo "▶ Toolchain : ${BUILDKIT_PATH}/bin/${BUILDKIT_FLAVOR}-*"
echo "▶ Output    : ${AUDIO_LIBS_DIR}"
echo "▶ Jobs      : ${JOBS}"
echo

$SUDO mkdir -p "$AUDIO_LIBS_DIR"
# Make the dir user-writable so we don't need sudo for every wget/extract.
if [ -n "$SUDO" ]; then
    $SUDO chown "$(id -u):$(id -g)" "$AUDIO_LIBS_DIR" || $SUDO chmod 0777 "$AUDIO_LIBS_DIR"
fi

cd "$AUDIO_LIBS_DIR"

# --- ALSA ---------------------------------------------------------------
ALSA_TARBALL="alsa-lib-${ALSA_VERSION}.tar.bz2"
ALSA_DIR="alsa-lib-${ALSA_VERSION}"
if [ ! -f "$ALSA_TARBALL" ]; then
    echo "▶ Fetching $ALSA_TARBALL"
    wget -q --show-progress \
        "https://www.alsa-project.org/files/pub/lib/${ALSA_TARBALL}"
fi
[ -d "$ALSA_DIR" ] || tar xf "$ALSA_TARBALL"

if [ ! -f "${ALSA_DIR}/.built" ]; then
    echo "▶ Cross-compiling ALSA (${ALSA_VERSION})"
    pushd "$ALSA_DIR" >/dev/null
    # Cortex-A7 + NEON, conservative -O2. The disable-* flags trim ALSA
    # down to only the PCM bits the JetKVM audio path uses; full ALSA
    # pulls in seq/rawmidi/ucm which the device doesn't need and which
    # had FD_SETSIZE issues in past devcontainer builds.
    CFLAGS="-O2 -mfpu=neon -mtune=cortex-a7 -mfloat-abi=hard" \
    CC="$CC" AR="$AR" RANLIB="$RANLIB" \
        ./configure \
            --host="$BUILDKIT_FLAVOR" \
            --prefix="${AUDIO_LIBS_DIR}/${ALSA_DIR}" \
            --enable-static=yes --enable-shared=no \
            --with-pcm-plugins=rate,linear \
            --disable-seq --disable-rawmidi --disable-ucm \
            --disable-python --disable-old-symbols \
            --disable-topology --disable-hwdep --disable-mixer \
            --disable-alisp --disable-aload --disable-resmgr
    make -j"$JOBS"
    touch .built
    popd >/dev/null
fi

# --- Opus ---------------------------------------------------------------
OPUS_TARBALL="opus-${OPUS_VERSION}.tar.gz"
OPUS_DIR="opus-${OPUS_VERSION}"
if [ ! -f "$OPUS_TARBALL" ]; then
    echo "▶ Fetching $OPUS_TARBALL"
    wget -q --show-progress \
        "https://downloads.xiph.org/releases/opus/${OPUS_TARBALL}"
fi
[ -d "$OPUS_DIR" ] || tar xf "$OPUS_TARBALL"

if [ ! -f "${OPUS_DIR}/.built" ]; then
    echo "▶ Cross-compiling Opus (${OPUS_VERSION})"
    pushd "$OPUS_DIR" >/dev/null
    # --enable-fixed-point: Cortex-A7 has no hardware FPU on the rv1106
    # so the fixed-point Opus path is much faster than software-float.
    CFLAGS="-O2 -mfpu=neon -mtune=cortex-a7 -mfloat-abi=hard" \
    CC="$CC" AR="$AR" RANLIB="$RANLIB" \
        ./configure \
            --host="$BUILDKIT_FLAVOR" \
            --enable-static=yes --enable-shared=no \
            --enable-fixed-point
    make -j"$JOBS"
    touch .built
    popd >/dev/null
fi

# --- Sanity check -------------------------------------------------------
EXPECTED_ALSA="${AUDIO_LIBS_DIR}/${ALSA_DIR}/src/.libs/libasound.a"
EXPECTED_OPUS="${AUDIO_LIBS_DIR}/${OPUS_DIR}/.libs/libopus.a"
if [ ! -f "$EXPECTED_ALSA" ] || [ ! -f "$EXPECTED_OPUS" ]; then
    echo "ERROR: expected static libs missing after build" >&2
    echo "  $EXPECTED_ALSA -> $([ -f "$EXPECTED_ALSA" ] && echo OK || echo MISSING)" >&2
    echo "  $EXPECTED_OPUS -> $([ -f "$EXPECTED_OPUS" ] && echo OK || echo MISSING)" >&2
    exit 1
fi

echo
echo "✓ ALSA + Opus built in ${AUDIO_LIBS_DIR}"
echo "✓ Now you can rebuild the firmware *with* audio support:"
echo
echo "    make build_dev DEVICE_IP=10.0.0.20"
echo
echo "  (drop the no_audio build tag — it's no longer needed)"
