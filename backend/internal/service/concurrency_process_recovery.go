package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type processSlotRecoveryCache interface {
	CleanupProcessSlots(context.Context, string) error
}

// A marker belongs to one listener, not the Redis DB. flock proves its previous
// owner exited, even after OOM. Rolling listeners must use distinct paths.
func (s *ConcurrencyService) recoverProcessSlots(path string) error {
	cache, ok := s.cache.(processSlotRecoveryCache)
	if !ok {
		return fmt.Errorf("cache lacks process recovery support")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = lock.Close()
		}
	}()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("concurrency owner is still running: %w", err)
	}
	previous, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if prefix := strings.TrimSpace(string(previous)); prefix != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		err = cache.CleanupProcessSlots(ctx, prefix)
		cancel()
		if err != nil {
			return fmt.Errorf("recover previous owner: %w", err)
		}
	}
	// No new request slots exist yet. Preserve the old marker on failure.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".concurrency-owner-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.WriteString(RequestIDPrefix() + "\n")
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	err = dir.Sync()
	_ = dir.Close()
	if err != nil {
		return err
	}
	s.processOwnerLock = lock
	keep = true
	return nil
}
