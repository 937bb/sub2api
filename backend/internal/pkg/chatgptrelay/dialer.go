package chatgptrelay

import (
	"context"
	"net"
	"strings"
)

const targetHost = "chatgpt.com"

// DialContext matches net.Dialer.DialContext and http.Transport.DialContext.
type DialContext func(ctx context.Context, network, address string) (net.Conn, error)

// Settings controls the optional local TCP relay used for direct ChatGPT traffic.
type Settings struct {
	Enabled   bool
	RelayAddr string
}

// Wrap redirects only direct chatgpt.com:443 TCP dials to the configured relay.
func Wrap(base DialContext, settings Settings) DialContext {
	if base == nil || !settings.Enabled || strings.TrimSpace(settings.RelayAddr) == "" {
		return base
	}
	relayAddr := strings.TrimSpace(settings.RelayAddr)
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if IsTarget(network, address) {
			return base(ctx, network, relayAddr)
		}
		return base(ctx, network, address)
	}
}

// IsTarget reports whether a dial is a TCP connection to chatgpt.com:443.
func IsTarget(network, address string) bool {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(network)), "tcp") {
		return false
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil || port != "443" {
		return false
	}
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return host == targetHost
}
