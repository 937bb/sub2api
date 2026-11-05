package kiro

import (
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"time"
)

type Quota struct {
	Plan        string     `json:"plan"`
	Used        float64    `json:"used"`
	Limit       float64    `json:"limit"`
	Utilization *float64   `json:"utilization"`
	ResetsAt    *time.Time `json:"resets_at,omitempty"`
}
type usageAmounts struct {
	Used         float64  `json:"currentUsage"`
	Limit        float64  `json:"usageLimit"`
	PreciseUsed  *float64 `json:"currentUsageWithPrecision"`
	PreciseLimit *float64 `json:"usageLimitWithPrecision"`
}

func (a usageAmounts) values() (float64, float64, error) {
	u, l := a.Used, a.Limit
	if a.PreciseUsed != nil {
		u = *a.PreciseUsed
	}
	if a.PreciseLimit != nil {
		l = *a.PreciseLimit
	}
	if u < 0 || l < 0 || math.IsNaN(u) || math.IsNaN(l) || math.IsInf(u, 0) || math.IsInf(l, 0) {
		return 0, 0, errors.New("invalid Kiro quota amounts")
	}
	return u, l, nil
}

func UsageURL(c map[string]any) string {
	q := url.Values{"origin": {"AI_EDITOR"}, "resourceType": {"AGENTIC_REQUEST"}}
	if arn := String(c, "profile_arn"); arn != "" {
		q.Set("profileArn", arn)
	}
	return "https://q." + Region(c, "api") + ".amazonaws.com/getUsageLimits?" + q.Encode()
}

// ParseQuota includes only currently valid trial and bonus balances.
func ParseQuota(data []byte, now time.Time) (*Quota, error) {
	var raw struct {
		NextReset    float64 `json:"nextDateReset"`
		Subscription struct {
			Title string `json:"subscriptionTitle"`
		} `json:"subscriptionInfo"`
		Breakdown []struct {
			usageAmounts
			Resource  string  `json:"resourceType"`
			NextReset float64 `json:"nextDateReset"`
			Trial     *struct {
				usageAmounts
				Status string  `json:"freeTrialStatus"`
				Expiry float64 `json:"freeTrialExpiry"`
			} `json:"freeTrialInfo"`
			Bonuses []struct {
				usageAmounts
				Status string  `json:"status"`
				Expiry float64 `json:"expiresAt"`
			} `json:"bonuses"`
		} `json:"usageBreakdownList"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return nil, errors.New("invalid Kiro quota response")
	}
	quota := &Quota{Plan: raw.Subscription.Title}
	selected := -1
	for i, b := range raw.Breakdown {
		if b.Resource == "CREDIT" || b.Resource == "AGENTIC_REQUEST" {
			selected = i
			break
		}
	}
	if selected < 0 && len(raw.Breakdown) == 1 {
		selected = 0
	}
	if selected < 0 {
		return quota, nil
	}
	b := raw.Breakdown[selected]
	add := func(a usageAmounts) error {
		u, l, err := a.values()
		if err == nil {
			quota.Used += u
			quota.Limit += l
		}
		return err
	}
	if err := add(b.usageAmounts); err != nil {
		return nil, err
	}
	active := func(status string, expiry float64) bool {
		return status == "ACTIVE" && (expiry == 0 || expiry > float64(now.Unix()))
	}
	if b.Trial != nil && active(b.Trial.Status, b.Trial.Expiry) {
		if err := add(b.Trial.usageAmounts); err != nil {
			return nil, err
		}
	}
	for _, bonus := range b.Bonuses {
		if active(bonus.Status, bonus.Expiry) {
			if err := add(bonus.usageAmounts); err != nil {
				return nil, err
			}
		}
	}
	reset := b.NextReset
	if reset == 0 {
		reset = raw.NextReset
	}
	if reset > 0 && reset < 32503680000 {
		t := time.Unix(int64(reset), 0).UTC()
		quota.ResetsAt = &t
	}
	if math.IsInf(quota.Used, 0) || math.IsInf(quota.Limit, 0) {
		return nil, errors.New("invalid Kiro quota total")
	}
	if quota.Limit > 0 {
		percent := quota.Used / quota.Limit * 100
		if math.IsInf(percent, 0) {
			return nil, errors.New("invalid Kiro quota ratio")
		}
		quota.Utilization = &percent
	}
	return quota, nil
}
