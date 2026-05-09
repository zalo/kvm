package kvm

import (
	"github.com/jetkvm/kvm/internal/sync"
	"github.com/jetkvm/kvm/internal/usbgadget"
)

// gamepadSlots manages per-session HID gamepad slot assignment for multi-tenant
// co-op gameplay. When multi-player mode is enabled in config, each connected
// session is allocated one of the up-to-MaxGamepads HID gamepad slots, and
// that session's gamepad reports are remapped to its assigned slot regardless
// of the browser-side pad index.
//
// When multi-player mode is disabled, gamepadSlot returns -1 and callers fall
// back to the original padIndex from the report (single-player / last-input
// behavior, same as keyboard and mouse).
type gamepadSlotManager struct {
	mu      sync.Mutex
	occupied [usbgadget.MaxGamepads]string // slot -> session ID; "" means free
}

var gamepadSlots = &gamepadSlotManager{}

// claim assigns the first free slot to the given session. Returns -1 if all
// slots are taken or the session already holds a slot.
func (g *gamepadSlotManager) claim(sessionID string) int {
	if sessionID == "" {
		return -1
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, holder := range g.occupied {
		if holder == sessionID {
			return i
		}
	}
	for i, holder := range g.occupied {
		if holder == "" {
			g.occupied[i] = sessionID
			return i
		}
	}
	return -1
}

// release frees any slot held by the given session.
func (g *gamepadSlotManager) release(sessionID string) {
	if sessionID == "" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, holder := range g.occupied {
		if holder == sessionID {
			g.occupied[i] = ""
		}
	}
}

// slotFor returns the slot assigned to a session, or -1 if none.
func (g *gamepadSlotManager) slotFor(sessionID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i, holder := range g.occupied {
		if holder == sessionID {
			return i
		}
	}
	return -1
}

// resolveGamepadSlot picks the HID slot for an incoming gamepad report.
// In multi-player mode, the session's claimed slot wins (lazily allocated on
// first report). Otherwise the browser-supplied padIndex is used as-is.
func resolveGamepadSlot(session *Session, padIndex int) int {
	ensureConfigLoaded()
	if !config.MultiPlayerGamepad || session == nil {
		return padIndex
	}
	slot := gamepadSlots.slotFor(session.ID)
	if slot < 0 {
		slot = gamepadSlots.claim(session.ID)
	}
	if slot < 0 {
		// All slots in use — drop additional players' gamepad input rather
		// than let them stomp on someone else's slot.
		return -1
	}
	return slot
}
