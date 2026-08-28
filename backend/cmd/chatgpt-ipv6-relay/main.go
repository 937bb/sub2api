package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	defaultListenAddr  = "127.0.0.1:24443"
	defaultHealthAddr  = "127.0.0.1:24444"
	defaultTargetAddr  = "chatgpt.com:443"
	defaultDialTimeout = 10 * time.Second
)

type relayConfig struct {
	ListenAddr  string
	HealthAddr  string
	TargetAddr  string
	Prefix      netip.Prefix
	DialTimeout time.Duration
}

type relayStats struct {
	startedAt time.Time
	accepted  atomic.Uint64
	active    atomic.Int64
	failed    atomic.Uint64
}

type relayServer struct {
	cfg   relayConfig
	stats relayStats
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid relay configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	server := &relayServer{cfg: cfg}
	server.stats.startedAt = time.Now()
	if err := server.run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("relay stopped", "error", err)
		os.Exit(1)
	}
}

func loadConfig() (relayConfig, error) {
	prefixRaw := strings.TrimSpace(os.Getenv("RELAY_IPV6_PREFIX"))
	if prefixRaw == "" {
		return relayConfig{}, errors.New("RELAY_IPV6_PREFIX is required")
	}
	prefix, err := netip.ParsePrefix(prefixRaw)
	if err != nil {
		return relayConfig{}, fmt.Errorf("parse RELAY_IPV6_PREFIX: %w", err)
	}
	prefix = prefix.Masked()
	if !prefix.Addr().Is6() || prefix.Addr().Is4In6() || prefix.Bits() != 64 {
		return relayConfig{}, errors.New("RELAY_IPV6_PREFIX must be an IPv6 /64")
	}

	cfg := relayConfig{
		ListenAddr:  envOrDefault("RELAY_LISTEN_ADDR", defaultListenAddr),
		HealthAddr:  envOrDefault("RELAY_HEALTH_ADDR", defaultHealthAddr),
		TargetAddr:  defaultTargetAddr,
		Prefix:      prefix,
		DialTimeout: defaultDialTimeout,
	}
	if _, _, err := net.SplitHostPort(cfg.ListenAddr); err != nil {
		return relayConfig{}, fmt.Errorf("invalid RELAY_LISTEN_ADDR: %w", err)
	}
	if _, _, err := net.SplitHostPort(cfg.HealthAddr); err != nil {
		return relayConfig{}, fmt.Errorf("invalid RELAY_HEALTH_ADDR: %w", err)
	}
	return cfg, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func (s *relayServer) run(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen relay: %w", err)
	}
	defer func() {
		if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			slog.Warn("close relay listener", "error", closeErr)
		}
	}()

	healthServer := &http.Server{
		Addr:              s.cfg.HealthAddr,
		Handler:           http.HandlerFunc(s.handleHealth),
		ReadHeaderTimeout: 5 * time.Second,
	}
	healthErr := make(chan error, 1)
	go func() {
		if serveErr := healthServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			healthErr <- serveErr
		}
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = healthServer.Shutdown(shutdownCtx)
	}()

	slog.Info("relay ready",
		"listen", s.cfg.ListenAddr,
		"health", s.cfg.HealthAddr,
		"target", s.cfg.TargetAddr,
		"ipv6_prefix", s.cfg.Prefix.String(),
	)

	for {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil || errors.Is(acceptErr, net.ErrClosed) {
				return ctx.Err()
			}
			select {
			case serveErr := <-healthErr:
				return fmt.Errorf("health server: %w", serveErr)
			default:
			}
			return fmt.Errorf("accept downstream: %w", acceptErr)
		}
		s.stats.accepted.Add(1)
		s.stats.active.Add(1)
		go s.handleConn(conn)
	}
}

func (s *relayServer) handleConn(downstream net.Conn) {
	defer func() {
		if closeErr := downstream.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			slog.Debug("close downstream", "error", closeErr)
		}
	}()
	defer s.stats.active.Add(-1)

	sourceAddr, err := randomIPv6(s.cfg.Prefix, rand.Reader)
	if err != nil {
		s.stats.failed.Add(1)
		slog.Error("generate source address", "error", err)
		return
	}

	dialCtx, cancel := context.WithTimeout(context.Background(), s.cfg.DialTimeout)
	dialer := net.Dialer{
		Timeout:   s.cfg.DialTimeout,
		KeepAlive: 30 * time.Second,
		LocalAddr: &net.TCPAddr{IP: sourceAddr.AsSlice()},
		Control:   freeBindControl(),
	}
	upstream, err := dialer.DialContext(dialCtx, "tcp6", s.cfg.TargetAddr)
	cancel()
	if err != nil {
		s.stats.failed.Add(1)
		slog.Warn("upstream dial failed", "source_ipv6", sourceAddr.String(), "error", err)
		return
	}
	defer func() {
		if closeErr := upstream.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			slog.Debug("close upstream", "error", closeErr)
		}
	}()

	proxyBidirectional(downstream, upstream)
}

func proxyBidirectional(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyConn := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if tcp, ok := dst.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}
	go copyConn(left, right)
	go copyConn(right, left)
	wg.Wait()
}

func randomIPv6(prefix netip.Prefix, reader io.Reader) (netip.Addr, error) {
	if !prefix.Addr().Is6() || prefix.Addr().Is4In6() || prefix.Bits() != 64 {
		return netip.Addr{}, errors.New("prefix must be an IPv6 /64")
	}
	base := prefix.Masked().Addr().As16()
	var host [8]byte
	if _, err := io.ReadFull(reader, host[:]); err != nil {
		return netip.Addr{}, fmt.Errorf("read random host bits: %w", err)
	}
	copy(base[8:], host[:])
	return netip.AddrFrom16(base), nil
}

func (s *relayServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/health" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":         "ok",
		"target":         s.cfg.TargetAddr,
		"ipv6_prefix":    s.cfg.Prefix.String(),
		"accepted_total": s.stats.accepted.Load(),
		"active":         s.stats.active.Load(),
		"failed_total":   s.stats.failed.Load(),
		"uptime_seconds": int64(time.Since(s.stats.startedAt).Seconds()),
	})
}
