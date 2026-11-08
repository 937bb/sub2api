package kiro

// The provider reports a final snapshot, not deltas. Pointers distinguish an
// absent tokenUsage object from an explicitly reported all-zero snapshot.
// int32 matches the reference wire bounds; oversized values fail decoding.
type providerTokenUsage struct {
	Input  *int32 `json:"uncachedInputTokens"`
	Output *int32 `json:"outputTokens"`
	Read   *int32 `json:"cacheReadInputTokens"`
	Write  *int32 `json:"cacheWriteInputTokens"`
}

func (u *providerTokenUsage) snapshot() (Usage, bool) {
	if u == nil || (u.Input == nil && u.Output == nil && u.Read == nil && u.Write == nil) {
		return Usage{}, false
	}
	count := func(n *int32) int {
		if n == nil {
			return 0
		}
		return max(int(*n), 0)
	}
	usage := Usage{InputTokens: count(u.Input), OutputTokens: count(u.Output), CacheReadInputTokens: count(u.Read), CacheCreationInputTokens: count(u.Write)}
	if usage.CacheReadInputTokens > 0 || usage.CacheCreationInputTokens > 0 {
		usage.CacheSource = "upstream"
	}
	return usage, true
}
