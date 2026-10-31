// Linux-only CONNECT adapter for persisted per-account IPv6 proxy bindings.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type binding struct {
	Password string `json:"password"`
	IPv6     string `json:"ipv6"`
}
type configuration struct {
	Listen     string             `json:"listen"`
	Relay      string             `json:"relay"`
	Prefix     string             `json:"prefix"`
	Accounts   map[string]binding `json:"accounts"`
	AutoSecret string             `json:"auto_secret,omitempty"`
	AutoBase   string             `json:"auto_base,omitempty"`
}
type proxy struct {
	cfg    configuration
	prefix netip.Prefix
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: account-ipv6-proxy config.json")
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	var cfg configuration
	if err = json.Unmarshal(data, &cfg); err != nil {
		log.Fatal(err)
	}
	prefix, err := netip.ParsePrefix(cfg.Prefix)
	if err != nil || prefix.Bits() != 64 || !prefix.Addr().Is6() {
		log.Fatal("IPv6 /64 required")
	}
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil || host != "127.0.0.1" {
		log.Fatal("listen must use IPv4 loopback")
	}
	seen := map[string]bool{}
	if cfg.AutoSecret != "" {
		base, e := netip.ParseAddr(cfg.AutoBase)
		if len(cfg.AutoSecret) != 64 || e != nil || !prefix.Contains(base) || !base.Is6() {
			log.Fatal("invalid automatic binding configuration")
		}
		v := base.As16()
		for _, b := range v[10:] {
			if b != 0 {
				log.Fatal("automatic base must reserve 48 bits for account IDs")
			}
		}
	}
	for user, b := range cfg.Accounts {
		a, e := netip.ParseAddr(b.IPv6)
		if e != nil || !prefix.Contains(a) || a == prefix.Addr() || len(b.Password) < 32 || user == "" || seen[a.String()] {
			log.Fatal("invalid or duplicate binding")
		}
		seen[a.String()] = true
	}
	p := &proxy{cfg: cfg, prefix: prefix}
	log.Printf("ready accounts=%d listen=%s", len(cfg.Accounts), cfg.Listen)
	log.Fatal((&http.Server{Addr: cfg.Listen, Handler: p, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 16384}).ListenAndServe())
}

func (p *proxy) authenticate(h string) (binding, bool) {
	if !strings.HasPrefix(h, "Basic ") {
		return binding{}, false
	}
	raw, e := base64.StdEncoding.DecodeString(strings.TrimPrefix(h, "Basic "))
	if e != nil {
		return binding{}, false
	}
	user, password, ok := strings.Cut(string(raw), ":")
	b, exists := p.cfg.Accounts[user]
	if !exists {
		b, exists = p.automaticBinding(user)
	}
	return b, ok && exists && subtle.ConstantTimeCompare([]byte(password), []byte(b.Password)) == 1
}

// Signed canonical IDs allow new accounts immediately, without changing the
// running process or its immutable legacy credential map.
func (p *proxy) automaticBinding(user string) (binding, bool) {
	if p.cfg.AutoSecret == "" || !strings.HasPrefix(user, "b-account-") {
		return binding{}, false
	}
	raw := strings.TrimPrefix(user, "b-account-")
	id, err := strconv.ParseUint(raw, 10, 48)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != raw {
		return binding{}, false
	}
	base, err := netip.ParseAddr(p.cfg.AutoBase)
	if err != nil || !base.Is6() {
		return binding{}, false
	}
	a := base.As16()
	binary.BigEndian.PutUint64(a[8:], binary.BigEndian.Uint64(a[8:])|id)
	sum := sha256.Sum256([]byte("b-ipv6-v1:" + raw + ":" + p.cfg.AutoSecret))
	return binding{Password: hex.EncodeToString(sum[:]), IPv6: netip.AddrFrom16(a).String()}, true
}

func allowedTarget(address string) (string, bool) {
	h, port, e := net.SplitHostPort(address)
	h = strings.ToLower(h)
	return h, e == nil && port == "443" && (h == "chatgpt.com" || h == "auth.openai.com" || h == "api.openai.com")
}

func (p *proxy) dial(ctx context.Context, host string, b binding) (net.Conn, error) {
	d := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if host == "chatgpt.com" {
		c, e := d.DialContext(ctx, "tcp", p.cfg.Relay)
		if e != nil {
			return nil, e
		}
		a := netip.MustParseAddr(b.IPv6).As16()
		preface := append([]byte("S2RLYIP6\x01"), a[:]...)
		c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_, e = c.Write(preface)
		c.SetWriteDeadline(time.Time{})
		if e != nil {
			c.Close()
			return nil, e
		}
		return c, nil
	}
	d.LocalAddr = &net.TCPAddr{IP: net.ParseIP(b.IPv6)}
	d.Control = func(_, _ string, c syscall.RawConn) error {
		var e error
		if err := c.Control(func(fd uintptr) { e = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, 78, 1) }); err != nil {
			return err
		}
		return e
	}
	// No IPv4/direct fallback: refresh and API traffic use the same source address.
	return d.DialContext(ctx, "tcp6", net.JoinHostPort(host, "443"))
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","bindings":%d}`, len(p.cfg.Accounts))
		return
	}
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT required", 405)
		return
	}
	b, ok := p.authenticate(r.Header.Get("Proxy-Authorization"))
	if !ok {
		w.Header().Set("Proxy-Authenticate", `Basic realm="account-ipv6"`)
		http.Error(w, "authentication required", 407)
		return
	}
	host, ok := allowedTarget(r.Host)
	if !ok {
		http.Error(w, "destination not allowed", 403)
		return
	}
	upstream, err := p.dial(r.Context(), host, b)
	if err != nil {
		log.Printf("dial failed host=%s source=%s error=%v", host, b.IPv6, err)
		http.Error(w, "upstream unavailable", 502)
		return
	}
	defer upstream.Close()
	client, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	if _, err = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if rw.Flush() != nil {
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	copyHalf := func(dst net.Conn, src io.Reader) {
		defer wg.Done()
		io.Copy(dst, src)
		if c, ok := dst.(*net.TCPConn); ok {
			c.CloseWrite()
		}
	}
	go copyHalf(upstream, rw)
	go copyHalf(client, upstream)
	wg.Wait()
}
