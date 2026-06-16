package service

import (
	"context"
	"net/http"
	"time"
)

type OpenAICodexUserAgentProvider interface {
	GetOpenAICodexUserAgent(ctx context.Context) string
}

type OpenAICodexFingerprintAccountRepository interface {
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
}

type atomicOpenAICodexFingerprintRepository interface {
	EnsureOpenAICodexFingerprint(ctx context.Context, id int64, fingerprint OpenAICodexFingerprint, replaceExisting bool) (OpenAICodexFingerprint, error)
}

type openAICodexFingerprintContextKey struct{}

func openAICodexFingerprintContext(ctx context.Context, fp OpenAICodexFingerprint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAICodexFingerprintContextKey{}, fp)
}

func openAICodexFingerprintFromContext(ctx context.Context) (OpenAICodexFingerprint, bool) {
	if ctx == nil {
		return OpenAICodexFingerprint{}, false
	}
	fp, ok := ctx.Value(openAICodexFingerprintContextKey{}).(OpenAICodexFingerprint)
	return fp, ok
}

type OpenAICodexFingerprintService struct {
	accountRepo       OpenAICodexFingerprintAccountRepository
	userAgentProvider OpenAICodexUserAgentProvider
	now               func() time.Time
}

func NewOpenAICodexFingerprintService(accountRepo OpenAICodexFingerprintAccountRepository, userAgentProvider OpenAICodexUserAgentProvider) *OpenAICodexFingerprintService {
	return &OpenAICodexFingerprintService{
		accountRepo:       accountRepo,
		userAgentProvider: userAgentProvider,
		now:               time.Now,
	}
}

func (s *OpenAICodexFingerprintService) Ensure(ctx context.Context, account *Account) (OpenAICodexFingerprint, error) {
	if account == nil {
		return OpenAICodexFingerprint{}, ErrAccountNilInput
	}
	if !account.IsOpenAIOAuthLike() {
		return OpenAICodexFingerprint{}, nil
	}

	defaultProfile := ParseOpenAICodexUAProfile(s.defaultUserAgent(ctx))
	return ensureOpenAICodexFingerprintWithProfile(ctx, account, s.accountRepo, defaultProfile, s.currentTime())
}

func ensureOpenAICodexFingerprintWithProfile(ctx context.Context, account *Account, accountRepo OpenAICodexFingerprintAccountRepository, defaultProfile OpenAICodexUAProfile, now time.Time) (OpenAICodexFingerprint, error) {
	if account == nil {
		return OpenAICodexFingerprint{}, ErrAccountNilInput
	}
	if !account.IsOpenAIOAuthLike() {
		return OpenAICodexFingerprint{}, nil
	}

	fp, changed := NormalizeOpenAICodexFingerprint(account.Extra[OpenAICodexFingerprintExtraKey], defaultProfile, now)
	if !changed {
		return fp, nil
	}
	if accountRepo == nil {
		// No persistence path: use the fingerprint for this request, but do not mark
		// the account cache as converged when nothing durable was written.
		return fp, nil
	}
	if repo, ok := accountRepo.(atomicOpenAICodexFingerprintRepository); ok {
		var err error
		fp, err = repo.EnsureOpenAICodexFingerprint(ctx, account.ID, fp, changedExistingOpenAICodexFingerprint(account.Extra[OpenAICodexFingerprintExtraKey]))
		if err != nil {
			return OpenAICodexFingerprint{}, err
		}
	} else {
		if err := accountRepo.UpdateExtra(ctx, account.ID, map[string]any{OpenAICodexFingerprintExtraKey: fp}); err != nil {
			return OpenAICodexFingerprint{}, err
		}
	}
	if account.Extra == nil {
		account.Extra = map[string]any{}
	}
	account.Extra[OpenAICodexFingerprintExtraKey] = fp
	return fp, nil
}

func ensureOpenAICodexFingerprintForRequest(ctx context.Context, accountRepo OpenAICodexFingerprintAccountRepository, account *Account, req *http.Request) (OpenAICodexFingerprint, error) {
	if account == nil {
		return OpenAICodexFingerprint{}, ErrAccountNilInput
	}
	if !account.IsOpenAIOAuthLike() {
		return OpenAICodexFingerprint{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	profile := ParseOpenAICodexUAProfile(DefaultOpenAICodexUserAgent)
	fp, err := ensureOpenAICodexFingerprintWithProfile(ctx, account, accountRepo, profile, time.Now())
	if err != nil {
		return OpenAICodexFingerprint{}, err
	}
	applyOpenAICodexFingerprintHeaders(req, fp)
	return fp, nil
}

type OpenAICodexUAHeaders struct {
	UserAgent  string
	Originator string
	Version    string
}

func OpenAICodexHeadersFromUAProfile(profile OpenAICodexUAProfile) OpenAICodexUAHeaders {
	normalized := normalizeOpenAICodexUAProfile(profile)
	return OpenAICodexUAHeaders{
		UserAgent:  normalized.UserAgent(),
		Originator: normalized.Originator,
		Version:    normalized.CodexVersion,
	}
}

func applyOpenAICodexFingerprintHeaders(req *http.Request, fp OpenAICodexFingerprint) {
	if req == nil {
		return
	}
	headers := OpenAICodexHeadersFromUAProfile(fp.UAProfile)
	req.Header.Set("version", headers.Version)
	req.Header.Set("originator", headers.Originator)
	if headers.UserAgent != "" {
		req.Header.Set("user-agent", headers.UserAgent)
	}
}

func changedExistingOpenAICodexFingerprint(existing any) bool {
	return existing != nil
}

func (s *OpenAICodexFingerprintService) defaultUserAgent(ctx context.Context) string {
	if s == nil || s.userAgentProvider == nil {
		return DefaultOpenAICodexUserAgent
	}
	ua := s.userAgentProvider.GetOpenAICodexUserAgent(ctx)
	if ua == "" {
		return DefaultOpenAICodexUserAgent
	}
	return ua
}

func (s *OpenAICodexFingerprintService) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now()
	}
	return s.now()
}
