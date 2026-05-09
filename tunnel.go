package kvm

import (
	"github.com/jetkvm/kvm/internal/tunnel"
)

// tunnelManager owns the cloudflared quick-tunnel subprocess. Initialized
// in main.go with the JetKVM's local HTTP port (80).
var tunnelManager *tunnel.Manager

func initTunnel() {
	tunnelManager = tunnel.NewManager(80)
}

// rpcStartTunnel starts a cloudflared quick tunnel that exposes
// http://localhost:80 to a *.trycloudflare.com URL. Idempotent — returns
// the existing state if a tunnel is already running.
func rpcStartTunnel() (tunnel.State, error) {
	if tunnelManager == nil {
		return tunnel.State{}, errTunnelUnavailable
	}
	return tunnelManager.Start()
}

// rpcStopTunnel stops the running tunnel. No-op if not running.
func rpcStopTunnel() error {
	if tunnelManager == nil {
		return errTunnelUnavailable
	}
	return tunnelManager.Stop()
}

// rpcGetTunnelStatus returns whether the tunnel is active, its URL,
// when it started, and any recent error.
func rpcGetTunnelStatus() (tunnel.State, error) {
	if tunnelManager == nil {
		return tunnel.State{}, nil // pre-init = inactive, no error
	}
	return tunnelManager.Status(), nil
}

var errTunnelUnavailable = simpleError("tunnel manager not initialized")

type simpleError string

func (e simpleError) Error() string { return string(e) }
