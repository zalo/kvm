package kvm

import (
	"github.com/jetkvm/kvm/internal/sync"
	"github.com/pion/webrtc/v4/pkg/media"
)

// sessionRegistry holds all active WebRTC sessions for multi-tenant streaming.
// One encoder pipeline → fan out to N peer connections.
type sessionRegistry struct {
	mu       sync.Mutex
	byID     map[string]*Session
	primary  *Session // most recently added; legacy single-session callers read this
	password string   // bcrypt hash; empty means no auth required
}

var sessions = &sessionRegistry{byID: map[string]*Session{}}

// Add registers a session. Idempotent on re-add.
func (r *sessionRegistry) Add(s *Session) {
	if s == nil || s.ID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[s.ID] = s
	r.primary = s
}

// Remove deregisters a session. Picks a remaining session as primary, or nil.
func (r *sessionRegistry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	if r.primary != nil && r.primary.ID == id {
		r.primary = nil
		for _, s := range r.byID {
			r.primary = s
			break
		}
	}
}

// Primary returns the most-recently-added session, or nil. Used by legacy
// single-session code paths that haven't been refactored to broadcast.
func (r *sessionRegistry) Primary() *Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.primary
}

// Count returns the number of active sessions.
func (r *sessionRegistry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byID)
}

// snapshot returns a slice copy of current sessions (caller-owned, no lock).
func (r *sessionRegistry) snapshot() []*Session {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Session, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, s)
	}
	return out
}

// ForEach calls fn with each active session. Safe to call concurrently with
// Add/Remove; iteration is over a snapshot.
func (r *sessionRegistry) ForEach(fn func(*Session)) {
	for _, s := range r.snapshot() {
		fn(s)
	}
}

// BroadcastVideoSample writes the sample to every session's VideoTrack.
// Per-peer write errors are swallowed: a failing peer must not stall the encoder.
func (r *sessionRegistry) BroadcastVideoSample(sample media.Sample) {
	for _, s := range r.snapshot() {
		if s.VideoTrack != nil {
			_ = s.VideoTrack.WriteSample(sample)
		}
	}
}

// BroadcastAudioSample writes the sample to every session's AudioTrack.
func (r *sessionRegistry) BroadcastAudioSample(sample media.Sample) error {
	for _, s := range r.snapshot() {
		if s.AudioTrack != nil {
			_ = s.AudioTrack.WriteSample(sample)
		}
	}
	return nil
}
