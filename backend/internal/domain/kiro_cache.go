package domain

import "fmt"

// KiroResponseCachePolicy governs real response reuse across a group's accounts.
type KiroResponseCachePolicy struct {
	Enabled       bool `json:"enabled"`
	TargetPercent int  `json:"target_percent"`
	TTLSeconds    int  `json:"ttl_seconds"`
	MaxEntries    int  `json:"max_entries"`
}

func DefaultKiroResponseCachePolicy() KiroResponseCachePolicy {
	return KiroResponseCachePolicy{TargetPercent: 100, TTLSeconds: 300, MaxEntries: 128}
}

func (p KiroResponseCachePolicy) Validate() error {
	if p.TargetPercent < 0 || p.TargetPercent > 100 || p.TTLSeconds < 1 || p.TTLSeconds > 3600 || p.MaxEntries < 1 || p.MaxEntries > 1024 {
		return fmt.Errorf("Kiro cache requires target 0..100, TTL 1..3600s and capacity 1..1024")
	}
	return nil
}
