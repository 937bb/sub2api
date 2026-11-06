package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"golang.org/x/sync/singleflight"
)

type kiroUsageEntry struct {
	info    *UsageInfo
	expires time.Time
}
type kiroUsageCache struct {
	sync.Mutex
	entries map[string]kiroUsageEntry
	flight  singleflight.Group
}

func (s *AccountUsageService) getKiroUsage(ctx context.Context, a *Account, force bool) (*UsageInfo, error) {
	if s.kiroGateway == nil {
		return nil, errors.New("Kiro usage service unavailable")
	}
	proxy, err := kiroProxy(a)
	if err != nil {
		return nil, err
	}
	// Include credentials and proxy identity so edits cannot reuse another identity's quota.
	identity, _ := json.Marshal(a.Credentials)
	hash := sha256.Sum256(append(identity, []byte(proxy)...))
	key := fmt.Sprintf("%d:%x", a.ID, hash)
	load := func() *UsageInfo {
		s.kiroUsage.Lock()
		defer s.kiroUsage.Unlock()
		e := s.kiroUsage.entries[key]
		if time.Now().Before(e.expires) {
			return e.info
		}
		return nil
	}
	if !force {
		if info := load(); info != nil {
			return info, nil
		}
	}
	result := s.kiroUsage.flight.DoChan(key, func() (any, error) {
		if !force {
			if info := load(); info != nil {
				return info, nil
			}
		}
		info, err := s.fetchKiroUsage(ctx, a)
		ttl := time.Minute
		if err != nil {
			info = &UsageInfo{Error: err.Error(), ErrorCode: "kiro_usage_unavailable"}
			ttl = 15 * time.Second
		}
		s.kiroUsage.Lock()
		defer s.kiroUsage.Unlock()
		if s.kiroUsage.entries == nil {
			s.kiroUsage.entries = make(map[string]kiroUsageEntry)
		}
		for k, v := range s.kiroUsage.entries {
			if time.Now().After(v.expires) {
				delete(s.kiroUsage.entries, k)
			}
		}
		if len(s.kiroUsage.entries) >= 1024 {
			for k := range s.kiroUsage.entries {
				delete(s.kiroUsage.entries, k)
				break
			}
		}
		s.kiroUsage.entries[key] = kiroUsageEntry{info, time.Now().Add(ttl)}
		return info, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-result:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.(*UsageInfo), nil
	}
}

func (s *AccountUsageService) fetchKiroUsage(ctx context.Context, a *Account) (*UsageInfo, error) {
	a, token, err := s.kiroGateway.kiroAccessToken(ctx, a)
	if err != nil {
		return nil, errors.New("Kiro quota authentication failed")
	}
	proxy, err := kiroProxy(a)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kiro.UsageURL(a.Credentials), nil)
	if err != nil {
		return nil, err
	}
	req = prepareKiroHTTPRequest(req, a.Credentials, token, kiro.OperationUsage)
	resp, err := s.kiroGateway.httpUpstream.Do(req, proxy, a.ID, a.Concurrency)
	if err != nil {
		return nil, errors.New("Kiro quota transport failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Kiro quota HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return nil, errors.New("invalid Kiro quota response")
	}
	now := time.Now().UTC()
	quota, err := kiro.ParseQuota(body, now)
	if err != nil {
		return nil, err
	}
	return &UsageInfo{KiroQuota: quota, Source: "active", UpdatedAt: &now, SubscriptionTier: quota.Plan}, nil
}
