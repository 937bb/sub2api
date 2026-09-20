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
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	openAICodexDynamicProxyTraceURL = "https://www.cloudflare.com/cdn-cgi/trace"
	openAICodexDynamicProxyMaxBody  = 32 * 1024
)

// This summary contains no provider URL, proxy credentials, or account data.
type openAICodexTurnStateDynamicProxySummary struct {
	Requested  int      `json:"requested"`
	Candidates int      `json:"candidates"`
	Verified   int      `json:"verified"`
	Selected   int      `json:"selected"`
	Countries  []string `json:"countries"`
	Failures   int      `json:"failures"`
}

type openAICodexTurnStateDynamicProxyFetcher struct {
	providerClient *http.Client
	traceClient    func(*url.URL) *http.Client
	totalTimeout   time.Duration
	traceTimeout   time.Duration
}

func fetchOpenAICodexTurnStateDynamicProxies(ctx context.Context, providerURL string, count int) ([]*OpenAICodexTurnStateProxy, openAICodexTurnStateDynamicProxySummary, error) {
	fetcher := openAICodexTurnStateDynamicProxyFetcher{
		providerClient: newOpenAICodexDynamicProxyHTTPClient(nil),
		traceClient:    newOpenAICodexDynamicProxyHTTPClient,
		totalTimeout:   25 * time.Second,
		traceTimeout:   10 * time.Second,
	}
	defer fetcher.providerClient.CloseIdleConnections()
	return fetcher.fetch(ctx, providerURL, count)
}

func (f *openAICodexTurnStateDynamicProxyFetcher) fetch(ctx context.Context, rawURL string, count int) ([]*OpenAICodexTurnStateProxy, openAICodexTurnStateDynamicProxySummary, error) {
	summary := openAICodexTurnStateDynamicProxySummary{Requested: count, Countries: []string{}}
	if count < 1 || count > 5 {
		return nil, summary, errors.New("dynamic proxy count must be between 1 and 5")
	}
	providerURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || providerURL.Scheme != "https" || providerURL.Hostname() == "" || providerURL.User != nil || providerURL.Fragment != "" {
		return nil, summary, errors.New("dynamic proxy provider requires an HTTPS URL without user information or fragment")
	}
	if ip, parseErr := netip.ParseAddr(providerURL.Hostname()); parseErr == nil && !isOpenAICodexDynamicProxyPublicIP(ip) {
		return nil, summary, errors.New("dynamic proxy provider must use a public address")
	}
	query := providerURL.Query()
	query.Set("num", strconv.Itoa(count))
	providerURL.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(ctx, f.totalTimeout)
	defer cancel()
	var verified []*OpenAICodexTurnStateProxy
	var lastErr error
	for batch := 0; batch < 2 && ctx.Err() == nil; batch++ {
		body, fetchErr := readOpenAICodexDynamicProxyURL(ctx, f.providerClient, providerURL.String())
		if fetchErr != nil {
			summary.Failures++
			lastErr = fetchErr
			continue
		}
		var candidates []*url.URL
		for _, line := range strings.Split(string(body), "\n") {
			if len(candidates) >= count || summary.Candidates+len(candidates) >= 10 {
				break
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			candidate, parseErr := parseOpenAICodexDynamicProxyURL(line)
			if parseErr != nil {
				summary.Failures++
				continue
			}
			// A rotating gateway can return the same endpoint repeatedly while
			// assigning a different public exit to each new connection. Retain
			// these bounded samples and deduplicate only verified exit addresses.
			candidates = append(candidates, candidate)
		}
		summary.Candidates += len(candidates)
		results := f.verifyBatch(ctx, candidates)
		for _, result := range results {
			if result == nil {
				summary.Failures++
				continue
			}
			verified = append(verified, result)
			summary.Verified++
		}
		selected, countries := selectOpenAICodexDynamicProxies(verified, count)
		if len(selected) >= count && len(countries) >= count {
			break
		}
	}
	selected, countries := selectOpenAICodexDynamicProxies(verified, count)
	summary.Selected, summary.Countries = len(selected), countries
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, summary, context.Canceled
	}
	if len(selected) == 0 {
		if ctx.Err() != nil {
			return nil, summary, errors.New("dynamic proxy acquisition timed out")
		}
		if lastErr != nil {
			return nil, summary, lastErr
		}
		return nil, summary, errors.New("dynamic proxy provider produced no verified public exits")
	}
	return selected, summary, nil
}

func (f *openAICodexTurnStateDynamicProxyFetcher) verifyBatch(ctx context.Context, candidates []*url.URL) []*OpenAICodexTurnStateProxy {
	results := make([]*OpenAICodexTurnStateProxy, len(candidates))
	var workers sync.WaitGroup
	semaphore := make(chan struct{}, 5)
	for i, proxyURL := range candidates {
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}
			probeCtx, cancel := context.WithTimeout(ctx, f.traceTimeout)
			defer cancel()
			client := f.traceClient(proxyURL)
			defer client.CloseIdleConnections()
			body, err := readOpenAICodexDynamicProxyURL(probeCtx, client, openAICodexDynamicProxyTraceURL)
			if err != nil {
				return
			}
			var exitIP, country string
			for _, line := range strings.Split(string(body), "\n") {
				key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
				if !ok {
					continue
				}
				switch key {
				case "ip":
					exitIP = strings.TrimSpace(value)
				case "loc":
					country = strings.TrimSpace(value)
				}
			}
			ip, err := netip.ParseAddr(exitIP)
			if err != nil || !isOpenAICodexDynamicProxyPublicIP(ip) || len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
				return
			}
			results[i] = &OpenAICodexTurnStateProxy{
				Source: "dynamic", Name: "Dynamic " + country, ProxyURL: proxyURL.String(),
				MaskedURL: maskOpenAICodexTurnStateProxyURL(proxyURL.String()),
				Enabled:   true, HealthStatus: "healthy", Country: country, ExitIP: ip.Unmap().String(),
			}
		}()
	}
	workers.Wait()
	return results
}

func selectOpenAICodexDynamicProxies(candidates []*OpenAICodexTurnStateProxy, count int) ([]*OpenAICodexTurnStateProxy, []string) {
	selected := make([]*OpenAICodexTurnStateProxy, 0, count)
	countries := make([]string, 0, count)
	seenIPs := make(map[string]struct{})
	seenCountries := make(map[string]struct{})
	var sameCountry []*OpenAICodexTurnStateProxy
	for _, candidate := range candidates {
		if _, exists := seenIPs[candidate.ExitIP]; exists {
			continue
		}
		seenIPs[candidate.ExitIP] = struct{}{}
		if _, exists := seenCountries[candidate.Country]; exists {
			sameCountry = append(sameCountry, candidate)
			continue
		}
		seenCountries[candidate.Country] = struct{}{}
		selected = append(selected, candidate)
		countries = append(countries, candidate.Country)
		if len(selected) == count {
			return selected, countries
		}
	}
	for _, candidate := range sameCountry {
		selected = append(selected, candidate)
		if len(selected) == count {
			break
		}
	}
	return selected, countries
}

func parseOpenAICodexDynamicProxyURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("unsupported dynamic proxy format")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid dynamic proxy port")
	}
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || !isOpenAICodexDynamicProxyPublicIP(ip) {
		return nil, errors.New("dynamic proxy requires a public IP literal")
	}
	u.Host = net.JoinHostPort(ip.Unmap().String(), strconv.Itoa(port))
	u.Path = ""
	return u, nil
}

func readOpenAICodexDynamicProxyURL(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("dynamic proxy request could not be created")
	}
	req.Header.Set("User-Agent", "curl/8.0")
	req.Header.Set("Accept", "text/plain")
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("dynamic proxy request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dynamic proxy request returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, openAICodexDynamicProxyMaxBody+1))
	if err != nil || len(body) > openAICodexDynamicProxyMaxBody {
		return nil, errors.New("dynamic proxy response was unreadable or too large")
	}
	return body, nil
}

func newOpenAICodexDynamicProxyHTTPClient(proxyURL *url.URL) *http.Client {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	transport := &http.Transport{
		DialContext:         openAICodexDynamicProxyPublicDialer(net.DefaultResolver.LookupNetIP, dialer.DialContext),
		TLSHandshakeTimeout: 8 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		DisableKeepAlives: true, MaxResponseHeaderBytes: 16 * 1024,
	}
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("dynamic proxy redirects are disabled")
	}}
}

// Resolve once, reject mixed public/private answers, and dial the checked literal
// so DNS rebinding cannot redirect a provider request into a private network.
func openAICodexDynamicProxyPublicDialer(lookup func(context.Context, string, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("dynamic proxy destination is invalid")
		}
		var addresses []netip.Addr
		if ip, parseErr := netip.ParseAddr(host); parseErr == nil {
			addresses = []netip.Addr{ip}
		} else {
			addresses, err = lookup(ctx, "ip", host)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("dynamic proxy destination lookup failed")
			}
		}
		for _, ip := range addresses {
			if !isOpenAICodexDynamicProxyPublicIP(ip) {
				return nil, errors.New("dynamic proxy destination is not public")
			}
		}
		for _, ip := range addresses {
			conn, dialErr := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if dialErr == nil {
				return conn, nil
			}
			if ctx.Err() != nil {
				break
			}
		}
		return nil, errors.New("dynamic proxy public destination connection failed")
	}
}

var openAICodexDynamicProxyNonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
}

func isOpenAICodexDynamicProxyPublicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range openAICodexDynamicProxyNonPublicPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
