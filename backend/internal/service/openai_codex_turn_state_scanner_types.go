package service

import (
	"context"
	"time"
)

const defaultOpenAICodexTurnStateScanModel = "gpt-5.5"

type OpenAICodexTurnStateProxy struct {
	ID                  int64      `json:"id"`
	Source              string     `json:"source,omitempty"`
	SourceID            int64      `json:"source_id,omitempty"`
	Name                string     `json:"name"`
	ProxyURL            string     `json:"-"`
	MaskedURL           string     `json:"masked_url"`
	Country             string     `json:"country,omitempty"`
	ExitIP              string     `json:"exit_ip,omitempty"`
	Enabled             bool       `json:"enabled"`
	RouteBindingEnabled bool       `json:"route_binding_enabled"`
	HealthStatus        string     `json:"health_status"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type OpenAICodexTurnStateScan struct {
	LeaseID         string     `json:"-"`
	AccountID       int64      `json:"account_id"`
	Model           string     `json:"model"`
	Status          string     `json:"status"`
	AttemptCount    int        `json:"attempt_count"`
	LastProxyID     *int64     `json:"last_proxy_id,omitempty"`
	LastProxyURL    string     `json:"-"`
	LastStateLength int        `json:"last_state_length"`
	LastError       string     `json:"last_error,omitempty"`
	LastAttemptAt   *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt   *time.Time `json:"last_success_at,omitempty"`
	NextAttemptAt   *time.Time `json:"next_attempt_at,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type OpenAICodexTurnStateAccountStatus struct {
	AccountID       int64      `json:"account_id"`
	AccountName     string     `json:"account_name"`
	AccountType     string     `json:"account_type"`
	PlanType        string     `json:"plan_type,omitempty"`
	Model           string     `json:"model"`
	Status          string     `json:"status"`
	StateLength     int        `json:"state_length"`
	TargetLengths   []int      `json:"target_lengths"`
	IssuedAt        *time.Time `json:"issued_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	LastAttemptAt   *time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt   *time.Time `json:"last_success_at,omitempty"`
	AttemptCount    int        `json:"attempt_count"`
	LastProxyID     *int64     `json:"last_proxy_id,omitempty"`
	LastProxyMasked string     `json:"last_proxy_masked,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
}

type OpenAICodexTurnStateAccountStatusList struct {
	Items    []*OpenAICodexTurnStateAccountStatus `json:"items"`
	Total    int64                                `json:"total"`
	Page     int                                  `json:"page"`
	PageSize int                                  `json:"page_size"`
}

type OpenAICodexTurnStateOperationsSummary struct {
	OAuthAccounts   int64      `json:"oauth_accounts"`
	ReadyAccounts   int64      `json:"ready_accounts"`
	MissingAccounts int64      `json:"missing_accounts"`
	TargetModels    int64      `json:"target_models"`
	ReadyModelSlots int64      `json:"ready_model_slots"`
	TotalModelSlots int64      `json:"total_model_slots"`
	RunningJobs     int64      `json:"running_jobs"`
	EnabledProxies  int64      `json:"enabled_proxies"`
	HealthyProxies  int64      `json:"healthy_proxies"`
	SharedProxies   int64      `json:"shared_proxies"`
	LastScanAt      *time.Time `json:"last_scan_at,omitempty"`
}

type OpenAICodexTurnStateScannerRepository interface {
	ListOpenAICodexTurnStateProxies(ctx context.Context, enabledOnly bool) ([]*OpenAICodexTurnStateProxy, error)
	ListReusableOpenAICodexTurnStateProxies(ctx context.Context) ([]*OpenAICodexTurnStateProxy, error)
	CreateOpenAICodexTurnStateProxies(ctx context.Context, proxies []*OpenAICodexTurnStateProxy) (int, error)
	SetOpenAICodexTurnStateProxyEnabled(ctx context.Context, id int64, enabled bool) error
	SetOpenAICodexTurnStateProxyRouteBinding(ctx context.Context, id int64, enabled bool) error
	DeleteOpenAICodexTurnStateProxy(ctx context.Context, id int64) error
	UpdateOpenAICodexTurnStateProxyHealth(ctx context.Context, proxy *OpenAICodexTurnStateProxy) error
	UpsertOpenAICodexTurnStateScan(ctx context.Context, scan *OpenAICodexTurnStateScan) error
	GetOpenAICodexTurnStateScan(ctx context.Context, accountID int64, model string) (*OpenAICodexTurnStateScan, error)
	ListRecentlyUsedOpenAICodexAccountIDs(ctx context.Context, usedSince time.Time) ([]int64, error)
	ListOpenAICodexTurnStateAccountStatuses(ctx context.Context, accountIDs []int64, targetModels []string, settings *OpenAICodexTurnStateScanSettings, page, pageSize int) (*OpenAICodexTurnStateAccountStatusList, error)
	GetOpenAICodexTurnStateOperationsSummary(ctx context.Context, targetModels []string, settings *OpenAICodexTurnStateScanSettings) (*OpenAICodexTurnStateOperationsSummary, error)
	ListObservedOpenAICodexTurnStateModels(ctx context.Context, accountID int64, usedSince time.Time) ([]string, error)
}

// OpenAICodexTurnStateScanLeaseRepository coordinates scan jobs across
// multiple service instances sharing the same database. It is optional so
// in-memory and test repositories can continue to implement the scanner
// repository without persistence-specific lease methods.
type OpenAICodexTurnStateScanLeaseRepository interface {
	ClaimOpenAICodexTurnStateScan(ctx context.Context, accountID int64, model, leaseID string, leaseUntil time.Time) (bool, error)
	ReleaseOpenAICodexTurnStateScan(ctx context.Context, accountID int64, model, leaseID string) error
}
