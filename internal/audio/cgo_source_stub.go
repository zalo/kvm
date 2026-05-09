//go:build !(linux && (arm || arm64))

package audio

import "errors"

// stubSource is a no-op AudioSource for non-ARM platforms (host dev builds only).
// The real CGO implementation lives in cgo_source.go and is built only on linux/arm{,64}.
type stubSource struct{}

func (s *stubSource) ReadMessage() (uint8, []byte, error) {
	return 0, nil, errors.New("audio not supported on this platform")
}

func (s *stubSource) WriteMessage(uint8, []byte) error {
	return errors.New("audio not supported on this platform")
}

func (s *stubSource) IsConnected() bool { return false }
func (s *stubSource) Connect() error    { return errors.New("audio not supported on this platform") }
func (s *stubSource) Disconnect()       {}

// NewCgoOutputSource returns a stub on non-ARM platforms.
func NewCgoOutputSource(string) AudioSource { return &stubSource{} }

// NewCgoInputSource returns a stub on non-ARM platforms.
func NewCgoInputSource(string) AudioSource { return &stubSource{} }
