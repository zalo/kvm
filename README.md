# JetKVM-coop — gaming-optimized fork of JetKVM

This is a downstream fork of [jetkvm/kvm](https://github.com/jetkvm/kvm) that
turns a single JetKVM v1 into a small co-op cloud-gaming station: one host PC
plugged into the JetKVM streams video + audio to multiple browser viewers over
WebRTC, each of whom can plug in their own gamepad and play together.

The upstream project is the real KVM-over-IP firmware. We borrow the encoder,
HID gadget, and signaling layers and bolt on the bits a co-op gaming setup
needs. Maintained as a private fork — we don't expect to upstream most of
this.

Credit and respect to the [JetKVM team](https://jetkvm.com) for the underlying
firmware. If you want a normal KVM-over-IP, use upstream.

## What this fork adds on top of upstream

- **1280×720@120 Hz video** — extends the default EDID with a 720p120 DTD
  (base block, NVIDIA-friendly) and plumbs the source vrefresh into the
  Rockchip MPP encoder's rate-control config. Glass-to-glass drops from
  ~16.7 ms → ~8.3 ms when the source PC is at 120 Hz.

- **Browser-driven gamepad passthrough** — up to 4 emulated USB HID
  gamepads. Includes a "Console Mode" toggle that disables keyboard +
  mouse so games that auto-grab those don't steal them from the
  controller.

- **Audio output** — vendored from
  [jetkvm/kvm#1205](https://github.com/jetkvm/kvm/pull/1205). HDMI-out
  audio via Opus is enabled; **microphone input is intentionally
  disabled in this fork** (use Discord for voice chat — much lower
  latency than routing browser → JetKVM → USB → game).

- **Multi-tenant streaming** — the encoder runs once and fans out RTP
  to every connected peer instead of kicking the existing one out.
  Up to ~10 concurrent viewers on a LAN before CPU/bandwidth pinches.

- **Per-session gamepad slot mapping** — when "Co-op multi-player" is
  on (Settings → Gamepad), each viewer claims one of 4 HID gamepad
  slots. Pair with Console Mode for a 4-player couch-coop console
  setup driven from 4 different web browsers. When off, all viewers
  share slot 0 (last-input-wins, single-player).

- **Sharing password gate** — set a separate password under Settings
  → Sharing. Guests sign in at `/share-login` and get a `shareToken`
  cookie scoped to the WebRTC stream. The admin password is never
  shared. Guests can be kicked en masse with the "Kick all clients"
  button on the same page.

- **Cloudflare Tunnel quick links** — generate a
  `*.trycloudflare.com` URL from the Sharing settings page so the
  JetKVM is reachable from the internet without router NAT/port-forward
  setup. Backed by the official `cloudflared` binary running on the
  device. Stop the tunnel from the same UI.

## Hardware

JetKVM v1 (rv1106 / Toshiba TC358743 capture chip). The chip silicon
caps cleanly at 1080p@60 and 720p@120 — anything higher won't lock.
Your cloud-gaming source PC needs to output one of those modes.

## Building

```sh
# 1. Install the ARM cross-toolchain (one-time)
sudo .devcontainer/install-deps.sh

# 2. Cross-compile ALSA + Opus into /opt/jetkvm-audio-libs (one-time)
./scripts/install_audio_deps.sh

# 3. Drop a pinned cloudflared binary into the deploy artefact (one-time)
./scripts/install_cloudflared.sh

# 4. Build firmware + push to the JetKVM
./dev_deploy.sh -r 10.0.0.20
```

If you don't want audio support (e.g. building on a host without the
ALSA/Opus libs), pass `-tags no_audio` to `go build` — `audio.go`
falls back to a stub on non-ARM hosts and on `no_audio` builds.

## Threat model and trust assumptions

This fork is meant for a small group of trusted players. The sharing
password is one shared secret per device, bcrypt-hashed and gate
scoped only to WebRTC signaling. There is no per-user accounting and
no rate limit beyond the per-IP limit on `/auth/share-login`.

The cloudflared quick tunnel exposes the JetKVM's HTTP/WebSocket
surface to the public internet without your router's firewall.
**Always set a sharing password before opening a tunnel** — without
one, anyone with the URL can connect. The "Kick all clients" button
is a hammer, not a scalpel: use it when you're done playing or when
something looks wrong.

## Why not vendor cloudflared as a submodule?

Considered and rejected. Cloudflared has its own `go.mod` graph
(hundreds of deps, including some CGO-heavy QUIC pieces) that would
either force a separate-module build inside our repo or balloon our
own dependency tree. We don't modify cloudflared. The official
`cloudflared-linux-arm` release is statically linked, runs cleanly on
the JetKVM's uClibc rootfs, and we pin a specific version + SHA256 in
`scripts/install_cloudflared.sh`. That's effectively the same trust
boundary as a submodule pin without the build cost. If Cloudflare
changes the quick-tunnel CLI surface, we'll bump the pinned version.

## Upstream sync

This fork is rebased irregularly against `jetkvm/kvm@dev`. The big
features (multi-tenant registry, sharing auth, gamepad slot manager,
cloudflared wrapper) live in their own files so they survive rebases
cleanly. Anything that rewrote upstream code (WebRTC handlers,
encoder fps plumbing, EDID const) is the source of conflicts.

## Original JetKVM resources

- [Discord](https://jetkvm.com/discord)
- [Website](https://jetkvm.com)
- [Documentation](https://jetkvm.com/docs)
- Upstream issues: [jetkvm/kvm/issues](https://github.com/jetkvm/kvm/issues)
- Fork-specific issues: file a private issue on this repo

For the original project's contributing guidelines and code of conduct,
see [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md). For development setup
details, see [`DEVELOPMENT.md`](DEVELOPMENT.md).
