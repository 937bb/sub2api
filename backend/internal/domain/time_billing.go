package domain

// TimeBillingRule applies a multiplier during a daily half-open time window.
// Start > End represents a window that crosses midnight.
type TimeBillingRule struct {
	ID             string  `json:"id"`
	Enabled        bool    `json:"enabled"`
	Start          string  `json:"start"`
	End            string  `json:"end"`
	RateMultiplier float64 `json:"rate_multiplier"`
}
