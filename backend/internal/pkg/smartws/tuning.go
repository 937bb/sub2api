package smartws

import (
	"fmt"
	"strconv"
	"time"
)

// Tuning controls local resources, not provider quotas. Zero values retain the
// production defaults. Values are immutable after construction.
type Tuning struct {
	IdentityCapacity int
	AccountCapacity  int
	CapacityLimit    int
	IdleSeconds      int
	ColdWaitMillis   int
}

// TuningFromEnvironment accepts only explicitly named, bounded integer options.
// Errors never include raw environment values.
func TuningFromEnvironment(lookup func(string) (string, bool)) (Tuning, error) {
	var t Tuning
	fields := []struct {
		name   string
		target *int
	}{
		{"SMART_WS_IDENTITY_CAPACITY", &t.IdentityCapacity},
		{"SMART_WS_ACCOUNT_CAPACITY", &t.AccountCapacity},
		{"SMART_WS_CAPACITY_LIMIT", &t.CapacityLimit},
		{"SMART_WS_IDLE_SECONDS", &t.IdleSeconds},
		{"SMART_WS_COLD_WAIT_MS", &t.ColdWaitMillis},
	}
	for _, field := range fields {
		if raw, ok := lookup(field.name); ok {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 0 {
				return Tuning{}, fmt.Errorf("invalid %s: expected a nonnegative integer", field.name)
			}
			*field.target = value
		}
	}
	if err := t.validate(); err != nil {
		return Tuning{}, err
	}
	return t, nil
}
func (t Tuning) validate() error {
	identity, account, capacity, idle := 64, 256, 4096, 300
	if t.IdentityCapacity != 0 {
		identity = t.IdentityCapacity
	}
	if t.AccountCapacity != 0 {
		account = t.AccountCapacity
	}
	if t.CapacityLimit != 0 {
		capacity = t.CapacityLimit
	}
	if t.IdleSeconds != 0 {
		idle = t.IdleSeconds
	}
	if identity < 1 || identity > 256 || account < identity || account > 512 || capacity < account || capacity < 128 || capacity > 8192 {
		return fmt.Errorf("invalid SmartWS capacity: require identity (1..256) <= account (1..512) <= pool (128..8192)")
	}
	if idle < 30 || idle > 900 {
		return fmt.Errorf("invalid SMART_WS_IDLE_SECONDS: expected 30..900 or 0 for default")
	}
	if t.ColdWaitMillis < 0 || t.ColdWaitMillis > 500 {
		return fmt.Errorf("invalid SMART_WS_COLD_WAIT_MS: expected 0..500")
	}
	return nil
}

// NewPoolWithTuning preserves pacing, identity isolation and dispatch semantics.
// Optional cold waiting applies only to reusable identities before dispatch.
func NewPoolWithTuning(dial DialFunc, t Tuning) (*Pool, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}
	c := defaultPoolConfig()
	if t.IdentityCapacity != 0 {
		c.perKey = t.IdentityCapacity
	}
	if t.AccountCapacity != 0 {
		c.perAccount = t.AccountCapacity
	}
	if t.CapacityLimit != 0 {
		c.hardCapacity = t.CapacityLimit
	}
	if t.IdleSeconds != 0 {
		c.idle = time.Duration(t.IdleSeconds) * time.Second
	}
	c.coldWait = time.Duration(t.ColdWaitMillis) * time.Millisecond
	c.dial = dial
	return newPool(c), nil
}
