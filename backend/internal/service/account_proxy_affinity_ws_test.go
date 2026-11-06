//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAccountProxyAffinityWebSocketReuseAndSharedCapacity(t *testing.T) {
	a := affinityTestAccount()
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 3
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 3
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	dialer := &rotationTestDialer{}
	pool.setClientDialerForTest(dialer)
	contexts := map[string]context.Context{}
	for i := 0; i < 100 && len(contexts) < 3; i++ {
		ctx := affinityTestContext(fmt.Sprintf("thread-%d", i), 7)
		routes, err := accountProxyRoutesForContext(ctx, a, "")
		require.NoError(t, err)
		contexts[routes[0]] = ctx
	}
	require.Len(t, contexts, 3)
	ids := map[string]string{}
	req := openAIWSAcquireRequest{Account: a, WSURL: "wss://example.com/v1/responses"}
	var held []*openAIWSConnLease
	defer func() {
		for _, lease := range held {
			lease.Release()
		}
	}()
	for round := 0; round < 2; round++ {
		for route, ctx := range contexts {
			lease, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			require.Equal(t, route, lease.conn.handshakeCompatibility.selectedProxy)
			if round == 0 {
				ids[route] = lease.ConnID()
				lease.Release()
			} else {
				require.Equal(t, ids[route], lease.ConnID())
				require.True(t, lease.reused)
				held = append(held, lease)
			}
		}
	}
	require.Len(t, dialer.routes, 3, "three proxies use three independent sockets")
	ctx, cancel := context.WithTimeout(affinityTestContext("capacity-probe", 7), 50*time.Millisecond)
	defer cancel()
	_, err := pool.Acquire(ctx, req)
	require.Error(t, err)
	require.Len(t, dialer.routes, 3, "adding proxies must not multiply the account budget")
	for _, lease := range held {
		lease.Release()
	}
	held = nil
	// A continuation already pinned before the rollout retains its exact socket,
	// even if the new hash would select a different route.
	for route, pinnedID := range ids {
		for otherRoute, ctx := range contexts {
			if route == otherRoute {
				continue
			}
			pinnedReq := req
			pinnedReq.ForcePreferredConn, pinnedReq.PreferredConnID = true, pinnedID
			lease, err := pool.Acquire(ctx, pinnedReq)
			require.NoError(t, err)
			require.Equal(t, pinnedID, lease.ConnID())
			require.Equal(t, route, lease.conn.handshakeCompatibility.selectedProxy)
			lease.Release()
		}
	}
}

func TestAccountProxyAffinityWebSocketNoFailureRotation(t *testing.T) {
	a := affinityTestAccount()
	ctx := affinityTestContext("failure", 7)
	for _, status := range []int{0, 429, 503} {
		d := &poolTestDialer{status: status}
		_, gotStatus, _, err := dialWithAccountProxyFallback(ctx, d, "wss://example.com", nil, "", a)
		require.Error(t, err)
		require.Equal(t, status, gotStatus)
		require.Len(t, d.routes, 1)
	}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 3
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	d := &rotationTestDialer{failFirst: true}
	pool.setClientDialerForTest(d)
	req := openAIWSAcquireRequest{Account: a, WSURL: "wss://example.com/v1/responses"}
	_, err := pool.Acquire(ctx, req)
	require.Error(t, err)
	require.Len(t, d.routes, 1)
	lease, err := pool.Acquire(ctx, req)
	require.NoError(t, err)
	defer lease.Release()
	require.Len(t, d.routes, 2)
	require.Equal(t, d.routes[0], d.routes[1], "explicit retry stays on the same route")
}
