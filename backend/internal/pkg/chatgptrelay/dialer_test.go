package chatgptrelay

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

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
