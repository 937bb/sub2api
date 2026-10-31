package main

import (
	"encoding/base64"
	"testing"
)

func TestTargetRestrictions(t *testing.T) {
	for target, want := range map[string]bool{"chatgpt.com:443": true, "auth.openai.com:443": true, "api.openai.com:443": true, "chatgpt.com:80": false, "chatgpt.com.evil.test:443": false, "127.0.0.1:5432": false, "[::1]:443": false} {
		_, got := allowedTarget(target)
		if got != want {
			t.Fatalf("%s: %v", target, got)
		}
	}
}
func TestAccountAuthentication(t *testing.T) {
	p := proxy{cfg: configuration{Accounts: map[string]binding{"b-42": {Password: "secret", IPv6: "2001:db8::42"}, "b-43": {Password: "different", IPv6: "2001:db8::43"}}}}
	for _, tt := range []struct {
		raw string
		ok  bool
	}{{"b-42:secret", true}, {"b-43:secret", false}, {"b-44:secret", false}, {"b-42:wrong", false}} {
		b, ok := p.authenticate("Basic " + base64.StdEncoding.EncodeToString([]byte(tt.raw)))
		if ok != tt.ok {
			t.Fatal(tt.raw)
		}
		if ok && b.IPv6 != "2001:db8::42" {
			t.Fatal("cross-account source")
		}
	}
}
