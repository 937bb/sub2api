package siwc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Registration contains no tokens, codes, state, nonce or PKCE secrets.
type Registration struct {
	ClientID  string    `json:"client_id"`
	HostID    string    `json:"host_id"`
	AccountID int64     `json:"account_id"`
	ProxyID   *int64    `json:"proxy_id"`
	CreatedAt time.Time `json:"created_at"`
}

func registrationPath(dir, sessionID string) string {
	sum := sha256.Sum256([]byte(sessionID))
	return filepath.Join(dir, "registration-"+hex.EncodeToString(sum[:])+".json")
}

func SaveRegistration(dir, sessionID string, registration Registration) error {
	if dir == "" {
		return nil
	}
	data, err := json.Marshal(registration)
	if err != nil {
		return err
	}
	return atomicStateFile(registrationPath(dir, sessionID), data, false)
}

func LoadRegistration(dir, sessionID string) (*Registration, error) {
	if dir == "" || sessionID == "" || len(sessionID) > 256 {
		return nil, errors.New("SIWC registration unavailable; start a new authorization")
	}
	data, err := os.ReadFile(registrationPath(dir, sessionID))
	if err != nil {
		return nil, errors.New("SIWC registration unavailable; start a new authorization")
	}
	var value Registration
	if json.Unmarshal(data, &value) != nil || time.Since(value.CreatedAt) > 24*time.Hour ||
		(value.ClientID != "dynamic_agent_client" && !clientIDPattern.MatchString(value.ClientID)) {
		return nil, errors.New("SIWC registration invalid or expired")
	}
	return &value, nil
}

// StableHostID adopts the previous browser ID once, then persists it with the
// runtime. Atomic link creation lets concurrent processes agree on one ID.
func StableHostID(dir, previous string) (string, error) {
	if dir == "" {
		return previous, nil
	}
	path := filepath.Join(dir, "host-id")
	if data, err := os.ReadFile(path); err == nil {
		if _, err := NewSession(string(data)); err != nil {
			return "", errors.New("invalid stored SIWC host ID")
		}
		return string(data), nil
	} else if !os.IsNotExist(err) {
		return "", errors.New("cannot read SIWC host ID")
	}
	session, err := NewSession(previous)
	if err != nil {
		return "", err
	}
	if err := atomicStateFile(path, []byte(session.HostID), true); err != nil && !os.IsExist(err) {
		return "", errors.New("cannot persist SIWC host ID")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("cannot read SIWC host ID")
	}
	if _, err := NewSession(string(data)); err != nil {
		return "", errors.New("invalid stored SIWC host ID")
	}
	return string(data), nil
}

func atomicStateFile(path string, data []byte, exclusive bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".siwc-state-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if exclusive {
		return os.Link(f.Name(), path)
	}
	return os.Rename(f.Name(), path)
}
