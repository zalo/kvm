package tunnel

import (
	_ "embed"
)

// embeddedBinary is the cross-compiled cloudflared binary, produced by
// scripts/build_cloudflared.sh from the vendor/cloudflared submodule.
// It's bundled into jetkvm_app at build time via go:embed so the firmware
// is a single artifact — no separate cloudflared file to push or update.
//
// At runtime, Manager.Start writes these bytes to a temp file under
// /tmp and execs from there.
//
//go:embed dist/cloudflared
var embeddedBinary []byte

// EmbeddedBinary returns the bundled cloudflared bytes (statically
// linked linux/arm/v7 ELF). Empty if build_cloudflared.sh hasn't run.
func EmbeddedBinary() []byte { return embeddedBinary }
