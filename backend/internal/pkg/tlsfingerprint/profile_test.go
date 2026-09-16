package tlsfingerprint

import "testing"

func TestProfileCloneAndContentCacheKey(t *testing.T) {
	profile := BuiltinProfile("nodejs24")
	if profile == nil {
		t.Fatal("nodejs24 profile is missing")
	}
	clone := profile.Clone()
	if clone.CacheKey() != profile.CacheKey() {
		t.Fatal("cloning changed the profile cache key")
	}

	originalKey := profile.CacheKey()
	clone.CipherSuites[0], clone.CipherSuites[1] = clone.CipherSuites[1], clone.CipherSuites[0]
	if clone.CacheKey() == originalKey {
		t.Fatal("edited profile reused the old cache key")
	}
	if profile.CacheKey() != originalKey {
		t.Fatal("editing the clone modified the original profile")
	}
}
