package chatgptrelay

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/netip"
	"strings"
)

const (
	sourceIPv6PrefaceMagic   = "S2RLYIP6"
	sourceIPv6PrefaceVersion = byte(1)
	sourceIPv6PrefaceSize    = len(sourceIPv6PrefaceMagic) + 1 + 16
)

type sourceIPv6ContextKey struct{}

// WithSourceIPv6 binds a direct ChatGPT dial to one IPv6 source address.
// Invalid and non-IPv6 values are deliberately cleared instead of reaching
// the relay wire protocol.
func WithSourceIPv6(ctx context.Context, value string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !addr.Is6() || addr.Is4In6() || addr.IsUnspecified() {
		return context.WithValue(ctx, sourceIPv6ContextKey{}, "")
	}
	return context.WithValue(ctx, sourceIPv6ContextKey{}, addr.String())
}

func SourceIPv6FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(sourceIPv6ContextKey{}).(string)
	return strings.TrimSpace(value)
}

// WriteSourceIPv6Preface sends the requested source address before TLS starts.
// The preface exists only on the trusted Sub2API-to-relay hop.
func WriteSourceIPv6Preface(w io.Writer, value string) error {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !addr.Is6() || addr.Is4In6() || addr.IsUnspecified() {
		return fmt.Errorf("invalid relay source IPv6 %q", value)
	}
	preface := make([]byte, 0, sourceIPv6PrefaceSize)
	preface = append(preface, sourceIPv6PrefaceMagic...)
	preface = append(preface, sourceIPv6PrefaceVersion)
	bytes := addr.As16()
	preface = append(preface, bytes[:]...)
	for len(preface) > 0 {
		written, writeErr := w.Write(preface)
		if writeErr != nil {
			return writeErr
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		preface = preface[written:]
	}
	return nil
}

// ReadSourceIPv6Preface consumes a source-address preface when present. A
// legacy TLS ClientHello is left fully buffered so older Sub2API instances can
// continue using the relay during a rolling deployment.
func ReadSourceIPv6Preface(r *bufio.Reader) (netip.Addr, bool, error) {
	if r == nil {
		return netip.Addr{}, false, fmt.Errorf("nil relay preface reader")
	}
	magic, err := r.Peek(len(sourceIPv6PrefaceMagic))
	if err != nil {
		return netip.Addr{}, false, fmt.Errorf("peek relay preface: %w", err)
	}
	if string(magic) != sourceIPv6PrefaceMagic {
		return netip.Addr{}, false, nil
	}
	preface := make([]byte, sourceIPv6PrefaceSize)
	if _, err := io.ReadFull(r, preface); err != nil {
		return netip.Addr{}, false, fmt.Errorf("read relay preface: %w", err)
	}
	if preface[len(sourceIPv6PrefaceMagic)] != sourceIPv6PrefaceVersion {
		return netip.Addr{}, false, fmt.Errorf("unsupported relay preface version %d", preface[len(sourceIPv6PrefaceMagic)])
	}
	var raw [16]byte
	copy(raw[:], preface[len(sourceIPv6PrefaceMagic)+1:])
	addr := netip.AddrFrom16(raw)
	if !addr.Is6() || addr.Is4In6() || addr.IsUnspecified() {
		return netip.Addr{}, false, fmt.Errorf("invalid relay preface source IPv6")
	}
	return addr, true, nil
}
