package service

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

type openAICodexInfrastructureCookie struct {
	value     string
	expiresAt time.Time
}

// openAICodexInfrastructureCookieStore mirrors the official Codex client's
// process-wide infrastructure cookie continuity. It deliberately accepts only
// Cloudflare/OpenAI routing cookies and can never retain an auth or user cookie.
type openAICodexInfrastructureCookieStore struct {
	mu      sync.Mutex
	cookies map[string]openAICodexInfrastructureCookie
}

func newOpenAICodexInfrastructureCookieStore() *openAICodexInfrastructureCookieStore {
	return &openAICodexInfrastructureCookieStore{
		cookies: make(map[string]openAICodexInfrastructureCookie),
	}
}

func (s *openAICodexInfrastructureCookieStore) capture(headers http.Header, now time.Time) {
	if s == nil || headers == nil {
		return
	}
	cookies := (&http.Response{Header: headers}).Cookies()
	if len(cookies) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cookies == nil {
		s.cookies = make(map[string]openAICodexInfrastructureCookie)
	}
	for _, cookie := range cookies {
		if cookie == nil || !isOpenAICodexAffinityCookieName(cookie.Name) {
			continue
		}
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && !cookie.Expires.After(now)) {
			delete(s.cookies, cookie.Name)
			continue
		}
		if (&http.Cookie{Name: cookie.Name, Value: cookie.Value}).Valid() != nil {
			continue
		}
		expiresAt := cookie.Expires
		if cookie.MaxAge > 0 {
			expiresAt = now.Add(time.Duration(cookie.MaxAge) * time.Second)
		}
		s.cookies[cookie.Name] = openAICodexInfrastructureCookie{
			value:     cookie.Value,
			expiresAt: expiresAt,
		}
	}
}

func (s *openAICodexInfrastructureCookieStore) header(now time.Time) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(s.cookies))
	for name, stored := range s.cookies {
		if !stored.expiresAt.IsZero() && !stored.expiresAt.After(now) {
			delete(s.cookies, name)
			continue
		}
		parts = append(parts, (&http.Cookie{Name: name, Value: stored.value}).String())
	}
	return normalizeOpenAICodexAffinityCookieHeader(strings.Join(parts, "; "))
}

func (s *OpenAIGatewayService) getOpenAICodexInfrastructureCookieStore() *openAICodexInfrastructureCookieStore {
	if s == nil {
		return nil
	}
	s.openaiCodexInfrastructureCookiesOnce.Do(func() {
		s.openaiCodexInfrastructureCookies = newOpenAICodexInfrastructureCookieStore()
	})
	return s.openaiCodexInfrastructureCookies
}

func (s *OpenAIGatewayService) captureOpenAICodexInfrastructureCookies(headers http.Header) {
	if s == nil || headers == nil {
		return
	}
	s.getOpenAICodexInfrastructureCookieStore().capture(headers, time.Now())
}

func (s *OpenAIGatewayService) captureOpenAICodexInfrastructureCookiesFromWSError(err error) {
	if s == nil || err == nil {
		return
	}
	var dialErr *openAIWSDialError
	if errors.As(err, &dialErr) && dialErr != nil {
		s.captureOpenAICodexInfrastructureCookies(dialErr.ResponseHeaders)
	}
}

// applyOpenAICodexInfrastructureCookies strips every inbound/private Cookie,
// adds the latest shared infrastructure cookies, then overlays a route ticket's
// explicit infrastructure cookies by name.
func (s *OpenAIGatewayService) applyOpenAICodexInfrastructureCookies(headers http.Header) {
	if s == nil || headers == nil {
		return
	}
	explicit := normalizeOpenAICodexAffinityCookieHeader(headers.Get("Cookie"))
	headers.Del("Cookie")
	shared := s.getOpenAICodexInfrastructureCookieStore().header(time.Now())
	merged := mergeOpenAICodexAffinityCookies(shared, parseOpenAICodexAffinityCookieHeader(explicit))
	if merged != "" {
		headers.Set("Cookie", merged)
	}
}

func parseOpenAICodexAffinityCookieHeader(value string) []*http.Cookie {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return (&http.Request{Header: http.Header{"Cookie": []string{value}}}).Cookies()
}
