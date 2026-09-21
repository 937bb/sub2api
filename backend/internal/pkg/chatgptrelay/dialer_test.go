package chatgptrelay

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceIPv6PrefaceRoundTrip(t *testing.T) {
	var payload bytes.Buffer
	require.NoError(t, WriteSourceIPv6Preface(&payload, "2a02:ae02:1a:2c00::1234"))
	payload.WriteString("tls-client-hello")

	reader := bufio.NewReader(&payload)
	addr, found, err := ReadSourceIPv6Preface(reader)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "2a02:ae02:1a:2c00::1234", addr.String())
	remainder, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "tls-client-hello", string(remainder))
}

func TestSourceIPv6PrefaceLeavesLegacyTLSBuffered(t *testing.T) {
	legacy := []byte{0x16, 0x03, 0x01, 0x00, 0x20, 0x01, 0x02, 0x03}
	reader := bufio.NewReader(bytes.NewReader(legacy))
	_, found, err := ReadSourceIPv6Preface(reader)
	require.NoError(t, err)
	require.False(t, found)
	remainder, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, legacy, remainder)
}

func TestWrapRedirectsOnlyChatGPTTLS(t *testing.T) {
	var gotNetwork string
	var gotAddress string
	wantErr := errors.New("dial stopped")
	base := func(_ context.Context, network, address string) (net.Conn, error) {
		gotNetwork = network
		gotAddress = address
		return nil, wantErr
	}
	dial := Wrap(base, Settings{Enabled: true, RelayAddr: "127.0.0.1:24443"})

	_, err := dial(context.Background(), "tcp", "chatgpt.com:443")
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "tcp", gotNetwork)
	require.Equal(t, "127.0.0.1:24443", gotAddress)

	_, err = dial(context.Background(), "tcp", "api.openai.com:443")
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "api.openai.com:443", gotAddress)

	_, err = dial(context.Background(), "udp", "chatgpt.com:443")
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "chatgpt.com:443", gotAddress)
}

func TestWrapDisabledKeepsOriginalAddress(t *testing.T) {
	var gotAddress string
	base := func(_ context.Context, _ string, address string) (net.Conn, error) {
		gotAddress = address
		return nil, errors.New("dial stopped")
	}
	dial := Wrap(base, Settings{RelayAddr: "127.0.0.1:24443"})

	_, _ = dial(context.Background(), "tcp", "chatgpt.com:443")
	require.Equal(t, "chatgpt.com:443", gotAddress)
}
