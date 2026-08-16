package service

import (
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	openAIWSPayloadSizeMinFailureSample = 64 * 1024
	openAIWSPayloadSizeSafetyPercent    = 95
	openAIWSPayloadSizeLearningTTL      = 6 * time.Hour
)

type openAIWSPayloadSizeRoute struct {
	failureBytes int
	expiresAt    time.Time
}

// openAIWSPayloadSizeRouter learns a remote close-1009 request boundary and
// sends similarly large payloads over HTTP without disabling WS for small turns.
type openAIWSPayloadSizeRouter struct {
	mu      sync.Mutex
	routes  map[string]openAIWSPayloadSizeRoute
	nowFunc func() time.Time
}

func (r *openAIWSPayloadSizeRouter) shouldBypass(wsURL string, payloadBytes int) (int, bool) {
	if r == nil || payloadBytes <= 0 {
		return 0, false
	}
	key := openAIWSPayloadSizeRouteKey(wsURL)
	if key == "" {
		return 0, false
	}

	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	route, ok := r.routes[key]
	if !ok {
		return 0, false
	}
	if !route.expiresAt.After(now) {
		delete(r.routes, key)
		return 0, false
	}
	threshold := openAIWSPayloadSizeBypassThreshold(route.failureBytes)
	return threshold, threshold > 0 && payloadBytes >= threshold
}

func (r *openAIWSPayloadSizeRouter) observeRemoteMessageTooBig(wsURL string, payloadBytes int) (int, bool) {
	if r == nil || payloadBytes < openAIWSPayloadSizeMinFailureSample {
		return 0, false
	}
	key := openAIWSPayloadSizeRouteKey(wsURL)
	if key == "" {
		return 0, false
	}

	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routes == nil {
		r.routes = make(map[string]openAIWSPayloadSizeRoute)
	}
	route, ok := r.routes[key]
	updated := !ok || !route.expiresAt.After(now) || payloadBytes < route.failureBytes
	if updated {
		route.failureBytes = payloadBytes
	}
	route.expiresAt = now.Add(openAIWSPayloadSizeLearningTTL)
	r.routes[key] = route
	return openAIWSPayloadSizeBypassThreshold(route.failureBytes), updated
}

func (r *openAIWSPayloadSizeRouter) now() time.Time {
	if r != nil && r.nowFunc != nil {
		return r.nowFunc()
	}
	return time.Now()
}

func openAIWSPayloadSizeBypassThreshold(failureBytes int) int {
	if failureBytes < openAIWSPayloadSizeMinFailureSample {
		return 0
	}
	return failureBytes * openAIWSPayloadSizeSafetyPercent / 100
}

func openAIWSPayloadSizeRouteKey(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Host) == "" {
		return strings.ToLower(trimmed)
	}
	return strings.ToLower(strings.TrimSpace(parsed.Host)) + strings.TrimRight(parsed.EscapedPath(), "/")
}
