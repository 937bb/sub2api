package service

import "errors"

// ErrOpenAIChannelModelRestricted identifies a local channel configuration
// rejection. No upstream request has run, so retrying an upstream attempt cannot
// resolve it. Selection still unwraps to ErrNoAvailableAccounts for its callers.
var ErrOpenAIChannelModelRestricted = errors.New("requested model is restricted by the channel configuration")

type openAIChannelModelRestrictedSelectionError struct {
	err error
}

func (e openAIChannelModelRestrictedSelectionError) Error() string { return e.err.Error() }

func (e openAIChannelModelRestrictedSelectionError) Unwrap() error { return e.err }

func (e openAIChannelModelRestrictedSelectionError) Is(target error) bool {
	return target == ErrOpenAIChannelModelRestricted
}

// noAvailableError preserves the selection diagnostics and distinguishes a
// wholly channel-restricted pool from temporary or mixed exclusions. A single
// restricted account is insufficient evidence when other candidates failed for
// another reason. The legacy scheduler uses channel_restricted for the same
// upstream model restriction.
func (s openAISelectionFilterStats) noAvailableError(requestedModel string, compactBlocked bool, extra string) error {
	err := noAvailableOpenAISelectionError(requestedModel, compactBlocked, s.summary(extra))
	if compactBlocked || s.pool <= 0 || len(s.reasons) != 1 {
		return err
	}
	if s.reasons["channel_upstream_restricted"] != s.pool && s.reasons["channel_restricted"] != s.pool {
		return err
	}
	return openAIChannelModelRestrictedSelectionError{err: err}
}
