package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type codexDynamicProxyRoundTripper func(*http.Request) (*http.Response, error)

func (f codexDynamicProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func codexDynamicProxyResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestCodexDynamicProxyRejectsNonPublicOrMalformedEndpoints(t *testing.T) {
	for _, input := range []string{
		"127.0.0.1:8000", "10.0.0.1:8080", "169.254.169.254:80", "100.64.0.1:80",
		"192.168.0.1:80", "198.18.0.1:80", "192.0.2.1:80", "240.0.0.1:80",
		"[::1]:8080", "[::ffff:127.0.0.1]:8080", "[fc00::1]:8080", "[64:ff9b::7f00:1]:80",
		"[2001:db8::1]:80", "[2002:7f00:1::]:80", "https://8.8.8.8:0", "8.8.8.8:65536",
		"ftp://8.8.8.8:80", "http://example.com:80", "http://8.8.8.8:80/path", "http://8.8.8.8:80/?token=secret",
		"http://8.8.8.8:80/#fragment", "http://8.8.8.8:not-a-port", "<html>error</html>",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := parseOpenAICodexDynamicProxyURL(input)
			require.Error(t, err)
		})
	}
	for input, normalized := range map[string]string{
		"8.8.8.8:8080":                      "http://8.8.8.8:8080",
		"https://1.1.1.1:443/":              "https://1.1.1.1:443",
		"socks5://user:p%40ss@1.1.1.1:1080": "socks5://user:p%40ss@1.1.1.1:1080",
		"[2606:4700:4700::1111]:8080":       "http://[2606:4700:4700::1111]:8080",
	} {
		u, err := parseOpenAICodexDynamicProxyURL(input)
		require.NoError(t, err)
		require.Equal(t, normalized, u.String())
	}
}

func TestCodexDynamicProxyPublicDialPinsDNSAndRejectsMixedAnswers(t *testing.T) {
	var lookupCalls, dialCalls int
	answers := []netip.Addr{netip.MustParseAddr("8.8.8.8")}
	lookup := func(context.Context, string, string) ([]netip.Addr, error) {
		lookupCalls++
		return answers, nil
	}
	dial := func(_ context.Context, _ string, address string) (net.Conn, error) {
		dialCalls++
		require.Equal(t, "8.8.8.8:443", address, "the verified literal must be dialed, never the original DNS name")
		return nil, errors.New("connection refused")
	}
	guarded := openAICodexDynamicProxyPublicDialer(lookup, dial)
	_, err := guarded(context.Background(), "tcp", "provider.example:443")
	require.Error(t, err)
	require.Equal(t, 1, lookupCalls)
	require.Equal(t, 1, dialCalls)

	answers = append(answers, netip.MustParseAddr("127.0.0.1"))
	_, err = guarded(context.Background(), "tcp", "provider.example:443")
	require.ErrorContains(t, err, "not public")
	require.Equal(t, 1, dialCalls, "mixed answers must be rejected before any connection")
	_, err = guarded(context.Background(), "tcp", "[::ffff:127.0.0.1]:443")
	require.ErrorContains(t, err, "not public")
	require.Equal(t, 2, lookupCalls, "IP literals do not require DNS")
}

func TestCodexDynamicProxyPrefersCountriesAndDeduplicatesActualExit(t *testing.T) {
	var providerCalls int
	exits := map[string]string{
		"8.8.8.1": "ip=9.9.9.1\nloc=DE\n", "8.8.8.2": "ip=9.9.9.2\nloc=DE\n",
		"8.8.8.3": "ip=9.9.9.1\nloc=DE\n", "8.8.8.4": "ip=9.9.9.4\nloc=US\n",
		"8.8.8.5": "ip=9.9.9.5\nloc=JP\n",
	}
	fetcher := openAICodexTurnStateDynamicProxyFetcher{
		providerClient: &http.Client{Transport: codexDynamicProxyRoundTripper(func(req *http.Request) (*http.Response, error) {
			providerCalls++
			require.Equal(t, "3", req.URL.Query().Get("num"))
			require.Equal(t, "Rand", req.URL.Query().Get("region"))
			require.Equal(t, "curl/8.0", req.Header.Get("User-Agent"))
			require.Equal(t, "text/plain", req.Header.Get("Accept"))
			require.Empty(t, req.Header.Get("Authorization"))
			require.Empty(t, req.Header.Get("Cookie"))
			if providerCalls == 1 {
				return codexDynamicProxyResponse(200, "127.0.0.1:80\n8.8.8.1:8080\n8.8.8.2:8080\n8.8.8.3:8080\n"), nil
			}
			return codexDynamicProxyResponse(200, "8.8.8.4:8080\n8.8.8.5:8080\n"), nil
		})},
		traceClient: func(proxyURL *url.URL) *http.Client {
			return &http.Client{Transport: codexDynamicProxyRoundTripper(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() != openAICodexDynamicProxyTraceURL || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
					return nil, errors.New("unexpected trace request or account header")
				}
				return codexDynamicProxyResponse(200, exits[proxyURL.Hostname()]), nil
			})}
		},
		totalTimeout: time.Second, traceTimeout: time.Second,
	}
	proxies, summary, err := fetcher.fetch(context.Background(), "https://provider.example/white/api?region=Rand&num=1", 3)
	require.NoError(t, err)
	require.Equal(t, 2, providerCalls)
	require.Len(t, proxies, 3)
	require.Equal(t, []string{"DE", "US", "JP"}, summary.Countries)
	require.Equal(t, 5, summary.Candidates)
	require.Equal(t, 5, summary.Verified)
	require.Equal(t, 1, summary.Failures)
	require.Equal(t, []string{"9.9.9.1", "9.9.9.4", "9.9.9.5"}, []string{proxies[0].ExitIP, proxies[1].ExitIP, proxies[2].ExitIP})
	for _, proxy := range proxies {
		require.Equal(t, "dynamic", proxy.Source)
		require.Zero(t, proxy.ID)
	}
}

func TestCodexDynamicProxyNeverClaimsMissingCountries(t *testing.T) {
	proxies, countries := selectOpenAICodexDynamicProxies([]*OpenAICodexTurnStateProxy{
		{Country: "DE", ExitIP: "8.8.8.1"}, {Country: "DE", ExitIP: "8.8.8.2"},
		{Country: "DE", ExitIP: "8.8.8.1"}, {Country: "US", ExitIP: "8.8.8.3"},
	}, 5)
	require.Len(t, proxies, 3)
	require.Equal(t, []string{"DE", "US"}, countries)
}

func TestCodexDynamicProxyRetainsRotatingGatewaySamplesAndCapsCandidates(t *testing.T) {
	providerCalls := 0
	var traceCalls atomic.Int32
	fetcher := openAICodexTurnStateDynamicProxyFetcher{
		providerClient: &http.Client{Transport: codexDynamicProxyRoundTripper(func(*http.Request) (*http.Response, error) {
			providerCalls++
			return codexDynamicProxyResponse(200, strings.Repeat("8.8.8.8:8080\n", 50)), nil
		})},
		traceClient: func(proxyURL *url.URL) *http.Client {
			return &http.Client{Transport: codexDynamicProxyRoundTripper(func(*http.Request) (*http.Response, error) {
				if proxyURL.String() != "http://8.8.8.8:8080" {
					return nil, errors.New("unexpected proxy URL")
				}
				exit := traceCalls.Add(1)
				return codexDynamicProxyResponse(200, fmt.Sprintf("ip=9.9.9.%d\nloc=DE\n", exit)), nil
			})}
		},
		totalTimeout: time.Second, traceTimeout: time.Second,
	}
	proxies, summary, err := fetcher.fetch(context.Background(), "https://provider.example/", 5)
	require.NoError(t, err)
	require.Equal(t, 2, providerCalls)
	require.Equal(t, int32(10), traceCalls.Load())
	require.Equal(t, 10, summary.Candidates)
	require.Equal(t, 10, summary.Verified)
	require.Equal(t, []string{"DE"}, summary.Countries)
	require.Len(t, proxies, 5)
	seenExits := map[string]bool{}
	for _, proxy := range proxies {
		require.Equal(t, "http://8.8.8.8:8080", proxy.ProxyURL)
		require.False(t, seenExits[proxy.ExitIP])
		seenExits[proxy.ExitIP] = true
	}
}

func TestCodexDynamicProxyBoundsConcurrencyAndCancellation(t *testing.T) {
	started := make(chan struct{}, 10)
	var active, peak atomic.Int32
	fetcher := openAICodexTurnStateDynamicProxyFetcher{
		traceClient: func(*url.URL) *http.Client {
			return &http.Client{Transport: codexDynamicProxyRoundTripper(func(req *http.Request) (*http.Response, error) {
				current := active.Add(1)
				defer active.Add(-1)
				for previous := peak.Load(); current > previous && !peak.CompareAndSwap(previous, current); previous = peak.Load() {
				}
				started <- struct{}{}
				<-req.Context().Done()
				return nil, req.Context().Err()
			})}
		},
		traceTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var candidates []*url.URL
	for i := 1; i <= 10; i++ {
		u, err := parseOpenAICodexDynamicProxyURL(fmt.Sprintf("8.8.8.%d:8080", i))
		require.NoError(t, err)
		candidates = append(candidates, u)
	}
	done := make(chan []*OpenAICodexTurnStateProxy, 1)
	go func() { done <- fetcher.verifyBatch(ctx, candidates) }()
	for range 5 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("five trace probes did not run concurrently")
		}
	}
	require.Equal(t, int32(5), active.Load())
	cancel()
	select {
	case results := <-done:
		require.Len(t, results, 10)
		for _, result := range results {
			require.Nil(t, result)
		}
	case <-time.After(time.Second):
		t.Fatal("trace probes did not stop after cancellation")
	}
	require.Equal(t, int32(5), peak.Load())
	require.Zero(t, active.Load())
}

func TestCodexDynamicProxyBoundedProviderFailuresAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  codexDynamicProxyRoundTripper
	}{
		{"http429", func(*http.Request) (*http.Response, error) {
			return codexDynamicProxyResponse(429, "secret-provider-body"), nil
		}},
		{"oversized", func(*http.Request) (*http.Response, error) {
			return codexDynamicProxyResponse(200, strings.Repeat("x", openAICodexDynamicProxyMaxBody+1)), nil
		}},
		{"network", func(*http.Request) (*http.Response, error) {
			return nil, errors.New("https://user:secret@proxy.example/?token=secret")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			fetcher := openAICodexTurnStateDynamicProxyFetcher{
				providerClient: &http.Client{Transport: codexDynamicProxyRoundTripper(func(req *http.Request) (*http.Response, error) {
					calls++
					return tc.run(req)
				})},
				traceClient:  func(*url.URL) *http.Client { t.Fatal("no proxy should be probed"); return nil },
				totalTimeout: time.Second, traceTimeout: time.Second,
			}
			proxies, summary, err := fetcher.fetch(context.Background(), "https://provider.example/?token=secret", 5)
			require.Error(t, err)
			require.Empty(t, proxies, "failure must never create a direct connection fallback")
			require.Equal(t, 2, calls)
			require.Equal(t, 2, summary.Failures)
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestCodexDynamicProxyTotalDeadlineStopsProvider(t *testing.T) {
	var calls atomic.Int32
	fetcher := openAICodexTurnStateDynamicProxyFetcher{
		providerClient: &http.Client{Transport: codexDynamicProxyRoundTripper(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			<-req.Context().Done()
			return nil, req.Context().Err()
		})},
		totalTimeout: 15 * time.Millisecond,
	}
	start := time.Now()
	proxies, _, err := fetcher.fetch(context.Background(), "https://provider.example/", 3)
	require.ErrorContains(t, err, "timed out")
	require.Empty(t, proxies)
	require.Equal(t, int32(1), calls.Load())
	require.Less(t, time.Since(start), time.Second)
}

func TestCodexDynamicProxyTransportUsesVerifiedTLSAndNoEnvironmentProxy(t *testing.T) {
	client := newOpenAICodexDynamicProxyHTTPClient(nil)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.Nil(t, transport.Proxy, "provider calls must not inherit business or environment proxies")
	require.True(t, transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify)
	require.Error(t, client.CheckRedirect(nil, nil))
	proxyURL, err := parseOpenAICodexDynamicProxyURL("http://8.8.8.8:8080")
	require.NoError(t, err)
	proxyClient := newOpenAICodexDynamicProxyHTTPClient(proxyURL)
	proxyTransport, ok := proxyClient.Transport.(*http.Transport)
	require.True(t, ok)
	configured, err := proxyTransport.Proxy(&http.Request{})
	require.NoError(t, err)
	require.Equal(t, proxyURL, configured)
}
