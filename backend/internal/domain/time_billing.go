package domain

const (
	TimeBillingRepeatDaily  = "daily"
	TimeBillingRepeatWeekly = "weekly"
)

// TimeBillingRule applies a multiplier during a recurring half-open time window.
// An empty RepeatType is treated as daily for compatibility with existing rules.
// Weekly weekdays use ISO numbering: Monday=1 through Sunday=7.
type TimeBillingRule struct {
	ID             string  `json:"id"`
	Enabled        bool    `json:"enabled"`
	RepeatType     string  `json:"repeat_type,omitempty"`
	StartWeekday   int     `json:"start_weekday,omitempty"`
	EndWeekday     int     `json:"end_weekday,omitempty"`
	Start          string  `json:"start"`
	End            string  `json:"end"`
	RateMultiplier float64 `json:"rate_multiplier"`
}
