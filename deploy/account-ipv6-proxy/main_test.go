package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
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

func TestAutomaticBindings(t *testing.T) {
	p := proxy{cfg: configuration{AutoSecret: strings.Repeat("a", 64), AutoBase: "2001:db8:1:2:937b::"}}
	for _, id := range []string{"1", "65175", "281474976710655"} {
		user := "b-account-" + id
		sum := sha256.Sum256([]byte("b-ipv6-v1:" + id + ":" + p.cfg.AutoSecret))
		b, ok := p.authenticate("Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+hex.EncodeToString(sum[:]))))
		if !ok {
			t.Fatal("new account not authenticated", id)
		}
		again, ok := p.automaticBinding(user)
		if !ok || b != again {
			t.Fatal("unstable binding")
		}
		if id == "65175" && b.IPv6 != "2001:db8:1:2:937b::fe97" {
			t.Fatal(b.IPv6)
		}
	}
	for _, u := range []string{"b-account-0", "b-account-01", "b-account--1", "b-account-281474976710656", "b-account-1:more", "other-1"} {
		if _, ok := p.automaticBinding(u); ok {
			t.Fatal("invalid ID accepted", u)
		}
	}
	b, _ := p.automaticBinding("b-account-1")
	if _, ok := p.authenticate("Basic " + base64.StdEncoding.EncodeToString([]byte("b-account-2:"+b.Password))); ok {
		t.Fatal("cross-account credential accepted")
	}
	p.cfg.Accounts = map[string]binding{"b-account-1": {Password: "legacy", IPv6: "2001:db8::1"}}
	if _, ok := p.authenticate("Basic " + base64.StdEncoding.EncodeToString([]byte("b-account-1:"+b.Password))); ok {
		t.Fatal("legacy credential overridden")
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
