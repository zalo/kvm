// Package tunnel manages an optional `cloudflared tunnel --url` quick
// tunnel that exposes the JetKVM's local HTTP server (port 80) to the
// public internet via a *.trycloudflare.com URL.
//
// Why quick tunnels: they're throwaway, don't require a Cloudflare
// account, and give us an HTTPS URL with no router NAT/port-forward
// setup. Perfect for "share this with a friend" cloud gaming.
//
// Limits: trycloudflare URLs change every time the tunnel restarts,
// they're rate-limited, and Cloudflare may take them down at any time.
// For anything serious, configure a real named Cloudflare Tunnel.
package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/jetkvm/kvm/internal/logging"
	"github.com/rs/zerolog"
)

// extractedBinaryPath is where Manager writes the embedded cloudflared
// bytes on first start. /tmp survives long enough for the JetKVM's
// boot-to-shutdown lifetime; we re-extract on each boot.
var extractedBinaryPath = "/tmp/jetkvm-cloudflared"

// fallbackBinaryPath is consulted when the embedded binary is empty
// (e.g. dev builds where build_cloudflared.sh wasn't run). Lets you
// drop a manually-built cloudflared on the device for testing without
// rebuilding the whole firmware.
var fallbackBinaryPath = "/userdata/jetkvm/bin/cloudflared"

// trycloudflareURLPattern matches the URL cloudflared prints to stderr
// when a quick tunnel is provisioned. Format examples:
//
//	|  https://something-foo-bar.trycloudflare.com  |
//	INF Connection registered ... | hostname=fast-anvil-xyz.trycloudflare.com
var trycloudflareURLPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// State is the public status of the tunnel. Returned via RPC.
type State struct {
	// Active is true when the cloudflared process is alive and a URL has
	// been observed. Note: cloudflared can be running but URL not yet
	// printed (~1–2s after start); UI should show "starting…" then.
	Active bool `json:"active"`

	// Starting indicates the process is up but no URL captured yet.
	Starting bool `json:"starting"`

	// URL is the *.trycloudflare.com URL once captured. Empty otherwise.
	URL string `json:"url,omitempty"`

	// StartedAt is when the cloudflared process was last spawned.
	StartedAt time.Time `json:"started_at,omitempty"`

	// LastError is the most recent failure message (e.g. binary missing,
	// crash, port in use). Cleared when a new tunnel starts successfully.
	LastError string `json:"last_error,omitempty"`
}

// Manager owns the cloudflared subprocess.
type Manager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	state   State
	logger  zerolog.Logger
	logPort int // port to forward (typically 80)
}

// NewManager constructs a new tunnel manager that forwards to
// http://localhost:<port>.
func NewManager(localPort int) *Manager {
	return &Manager{
		logPort: localPort,
		logger:  logging.GetDefaultLogger().With().Str("component", "cloudflared").Logger(),
	}
}

// resolveBinary materializes a cloudflared binary on disk, either from
// the embedded bytes (preferred) or from the fallback disk path.
// Returns the absolute path or an error. Idempotent — repeated calls
// after success skip re-writing.
func resolveBinary() (string, error) {
	if len(embeddedBinary) > 0 {
		// Already extracted? Verify size matches as a cheap freshness check.
		if fi, err := os.Stat(extractedBinaryPath); err == nil && fi.Size() == int64(len(embeddedBinary)) {
			return extractedBinaryPath, nil
		}
		if err := os.MkdirAll(filepath.Dir(extractedBinaryPath), 0o755); err != nil {
			return "", fmt.Errorf("mkdir extract dir: %w", err)
		}
		if err := os.WriteFile(extractedBinaryPath, embeddedBinary, 0o755); err != nil {
			return "", fmt.Errorf("write embedded cloudflared: %w", err)
		}
		return extractedBinaryPath, nil
	}
	if _, err := os.Stat(fallbackBinaryPath); err == nil {
		return fallbackBinaryPath, nil
	}
	return "", fmt.Errorf("cloudflared not embedded and not present at %s — rebuild with scripts/build_cloudflared.sh, or drop a binary on the device manually", fallbackBinaryPath)
}

// Status returns a snapshot of the tunnel's current state.
func (m *Manager) Status() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Start launches a new cloudflared quick tunnel. If a tunnel is already
// running, returns the existing state without restarting. Returns an
// error if the cloudflared binary is missing or the process fails to
// launch.
func (m *Manager) Start() (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil && m.cmd.Process != nil {
		// Already running — idempotent return of current state.
		return m.state, nil
	}

	binPath, err := resolveBinary()
	if err != nil {
		m.state.LastError = err.Error()
		return m.state, err
	}

	url := fmt.Sprintf("http://localhost:%d", m.logPort)
	cmd := exec.Command(binPath, //nolint:gosec // path is from a trusted source (embedded build artifact)
		"tunnel",
		"--no-autoupdate",
		"--url", url,
		// HTTP/2 transport — simpler than QUIC, fewer surprises on the
		// JetKVM's network stack.
		"--protocol", "http2",
	)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.state.LastError = fmt.Sprintf("stderr pipe: %v", err)
		return m.state, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.state.LastError = fmt.Sprintf("stdout pipe: %v", err)
		return m.state, err
	}

	if err := cmd.Start(); err != nil {
		m.state.LastError = fmt.Sprintf("start: %v", err)
		if errors.Is(err, exec.ErrNotFound) || errFileNotExist(err) {
			m.state.LastError = fmt.Sprintf("cloudflared not runnable at %s — try `scripts/build_cloudflared.sh` and re-deploy the firmware", binPath)
		}
		return m.state, err
	}

	m.cmd = cmd
	m.state = State{
		Active:    false,
		Starting:  true,
		StartedAt: time.Now(),
	}

	// Scan both pipes for the URL. cloudflared writes startup banners
	// to stderr; the URL appears there in the first ~2s of life.
	go m.scanForURL(stderr, "stderr")
	go m.scanForURL(stdout, "stdout")

	// Reap on exit so we don't leak zombies and so Status reflects
	// crashes promptly.
	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cmd = nil
		m.state.Active = false
		m.state.Starting = false
		if err != nil {
			m.state.LastError = fmt.Sprintf("cloudflared exited: %v", err)
			m.state.URL = ""
			m.logger.Warn().Err(err).Msg("cloudflared exited")
		} else {
			m.logger.Info().Msg("cloudflared exited cleanly")
		}
	}()

	m.logger.Info().Str("forward", url).Int32("pid", int32(cmd.Process.Pid)).Msg("cloudflared started")
	return m.state, nil
}

// Stop terminates the running tunnel. No-op if not running.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}
	// SIGTERM — cloudflared handles it cleanly and unregisters from CF.
	if err := m.cmd.Process.Signal(syscallSIGTERM()); err != nil {
		m.logger.Warn().Err(err).Msg("failed to signal cloudflared")
		return err
	}
	// The Wait() goroutine in Start clears m.cmd and updates state.
	return nil
}

// scanForURL reads cloudflared's output line-by-line, hunting for the
// trycloudflare URL. Once found, sets state.URL and flips Active=true.
func (m *Manager) scanForURL(r io.ReadCloser, source string) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		m.logger.Trace().Str("source", source).Str("line", line).Msg("cloudflared output")

		match := trycloudflareURLPattern.FindString(line)
		if match == "" {
			continue
		}
		m.mu.Lock()
		if m.state.URL == "" {
			m.state.URL = match
			m.state.Active = true
			m.state.Starting = false
			m.state.LastError = ""
			m.logger.Info().Str("url", match).Msg("quick tunnel URL captured")
		}
		m.mu.Unlock()
	}
}
