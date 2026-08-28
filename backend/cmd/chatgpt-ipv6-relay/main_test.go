package main

import (
	"bytes"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRandomIPv6StaysInsidePrefix(t *testing.T) {
	prefix := netip.MustParsePrefix("2a02:ae02:1a:2c00::/64")
	addr, err := randomIPv6(prefix, bytes.NewReader([]byte{0x93, 0x7b, 0xfb, 0xee, 0x0b, 0x1d, 0x00, 0x01}))
	require.NoError(t, err)
	require.True(t, prefix.Contains(addr))
	require.Equal(t, "2a02:ae02:1a:2c00:937b:fbee:b1d:1", addr.String())
}

func TestRandomIPv6RejectsNon64Prefix(t *testing.T) {
	_, err := randomIPv6(netip.MustParsePrefix("2a02:ae02:1a:2c00::/56"), bytes.NewReader(make([]byte, 8)))
	require.Error(t, err)
}
