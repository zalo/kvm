package audio

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jetkvm/kvm/internal/logging"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/rs/zerolog"
)

// SampleWriter is the per-frame fanout target. Implementations should be
// non-blocking on the hot path; per-peer write errors should not propagate up.
type SampleWriter func(media.Sample) error

// OutputRelay forwards audio from AudioSource (CGO) to one or more WebRTC peers
// via a SampleWriter callback. The callback is responsible for fanning out to
// all active sessions in the multi-tenant model.
type OutputRelay struct {
	source  AudioSource
	write   SampleWriter
	ctx     context.Context
	cancel  context.CancelFunc
	logger  zerolog.Logger
	running atomic.Bool
	sample  media.Sample
	stopped chan struct{}

	// Stats (Uint32: overflows after 2.7 years @ 50fps, faster atomics on 32-bit ARM)
	framesRelayed atomic.Uint32
	framesDropped atomic.Uint32
}

// NewOutputRelay creates a relay for output audio (device → browser).
// The write callback is invoked once per Opus frame; in the multi-tenant
// pipeline it fans out the same sample to every active session's AudioTrack.
func NewOutputRelay(source AudioSource, write SampleWriter) *OutputRelay {
	ctx, cancel := context.WithCancel(context.Background())
	logger := logging.GetDefaultLogger().With().Str("component", "audio-output-relay").Logger()

	return &OutputRelay{
		source:  source,
		write:   write,
		ctx:     ctx,
		cancel:  cancel,
		logger:  logger,
		stopped: make(chan struct{}),
		sample: media.Sample{
			Duration: 20 * time.Millisecond,
		},
	}
}

// Start begins relaying audio frames
func (r *OutputRelay) Start() error {
	if r.running.Swap(true) {
		return fmt.Errorf("output relay already running")
	}

	go r.relayLoop()
	r.logger.Debug().Msg("output relay started")
	return nil
}

// Stop stops the relay and waits for goroutine to exit
func (r *OutputRelay) Stop() {
	if !r.running.Swap(false) {
		return
	}

	r.cancel()
	<-r.stopped

	r.logger.Debug().
		Uint32("frames_relayed", r.framesRelayed.Load()).
		Uint32("frames_dropped", r.framesDropped.Load()).
		Msg("output relay stopped")
}

// relayLoop continuously reads from audio source and writes to WebRTC
func (r *OutputRelay) relayLoop() {
	defer close(r.stopped)

	const reconnectDelay = 1 * time.Second

	for r.running.Load() {
		// Ensure connected
		if !r.source.IsConnected() {
			if err := r.source.Connect(); err != nil {
				r.logger.Debug().Err(err).Msg("failed to connect, will retry")
				time.Sleep(reconnectDelay)
				continue
			}
		}

		// Read message from audio source
		msgType, payload, err := r.source.ReadMessage()
		if err != nil {
			// Connection error - reconnect
			if r.running.Load() {
				r.logger.Warn().Err(err).Msg("read error, reconnecting")
				r.source.Disconnect()
				time.Sleep(reconnectDelay)
			}
			continue
		}

		// Handle message
		if msgType == ipcMsgTypeOpus && len(payload) > 0 {
			// Reuse sample struct (zero-allocation hot path)
			r.sample.Data = payload

			if err := r.write(r.sample); err != nil {
				r.framesDropped.Add(1)
				r.logger.Warn().Err(err).Msg("failed to fan out sample to WebRTC peers")
			} else {
				r.framesRelayed.Add(1)
			}
		}
	}
}

// InputRelay forwards audio from WebRTC (browser microphone) to AudioSource (USB audio)
type InputRelay struct {
	source  AudioSource
	ctx     context.Context
	cancel  context.CancelFunc
	logger  zerolog.Logger
	running atomic.Bool
}

// NewInputRelay creates a relay for input audio (browser → device)
func NewInputRelay(source AudioSource) *InputRelay {
	ctx, cancel := context.WithCancel(context.Background())
	logger := logging.GetDefaultLogger().With().Str("component", "audio-input-relay").Logger()

	return &InputRelay{
		source: source,
		ctx:    ctx,
		cancel: cancel,
		logger: logger,
	}
}

// Start begins relaying audio frames
func (r *InputRelay) Start() error {
	if r.running.Swap(true) {
		return fmt.Errorf("input relay already running")
	}

	r.logger.Debug().Msg("input relay started")
	return nil
}

// Stop stops the relay
func (r *InputRelay) Stop() {
	if !r.running.Swap(false) {
		return
	}

	r.cancel()
	r.logger.Debug().Msg("input relay stopped")
}
