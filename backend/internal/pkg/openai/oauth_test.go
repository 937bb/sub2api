package openai

import (
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSessionStore_Stop_Idempotent(t *testing.T) {
	store := NewSessionStore()

	store.Stop()
	store.Stop()

	select {
	case <-store.stopCh:
		// ok
	case <-time.After(time.Second):
		t.Fatal("stopCh 未关闭")
	}
}

func TestSessionStore_Stop_Concurrent(t *testing.T) {
	store := NewSessionStore()

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Stop()
		}()
	}

	wg.Wait()

	select {
	case <-store.stopCh:
		// ok
	case <-time.After(time.Second):
		t.Fatal("stopCh 未关闭")
	}
}

func TestGenerateState_CodexShape(t *testing.T) {
	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState failed: %v", err)
	}
	if len(state) != 43 {
		t.Fatalf("state length mismatch: got=%d want=43", len(state))
	}
	if strings.Contains(state, "=") {
		t.Fatalf("state should not contain padding: %q", state)
	}
}

func TestGenerateCodeVerifier_CodexShape(t *testing.T) {
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier failed: %v", err)
	}
	if len(verifier) != 86 {
		t.Fatalf("code_verifier length mismatch: got=%d want=86", len(verifier))
	}
	if strings.Contains(verifier, "=") {
		t.Fatalf("code_verifier should not contain padding: %q", verifier)
	}
}

func TestBuildAuthorizationURLForPlatform_OpenAI(t *testing.T) {
	authURL := BuildAuthorizationURLForPlatform("state-1", "challenge-1", DefaultRedirectURI, OAuthPlatformOpenAI)
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("Parse URL failed: %v", err)
	}
	q := parsed.Query()
	if got := q.Get("client_id"); got != ClientID {
		t.Fatalf("client_id mismatch: got=%q want=%q", got, ClientID)
	}
	if got := q.Get("scope"); got != DefaultScopes {
		t.Fatalf("scope mismatch: got=%q want=%q", got, DefaultScopes)
	}
	if got := q.Get("codex_cli_simplified_flow"); got != "true" {
		t.Fatalf("codex flow mismatch: got=%q want=true", got)
	}
	if got := q.Get("id_token_add_organizations"); got != "true" {
		t.Fatalf("id_token_add_organizations mismatch: got=%q want=true", got)
	}
	if got := q.Get("originator"); got != DefaultOriginator {
		t.Fatalf("originator mismatch: got=%q want=%q", got, DefaultOriginator)
	}
	wantQuery := "response_type=code&client_id=app_EMoamEEZ73f0CkXaXp7hrann&redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback&scope=openid%20profile%20email%20offline_access%20api.connectors.read%20api.connectors.invoke&code_challenge=challenge-1&code_challenge_method=S256&id_token_add_organizations=true&codex_cli_simplified_flow=true&state=state-1&originator=codex_cli_rs"
	if got := parsed.RawQuery; got != wantQuery {
		t.Fatalf("authorize query mismatch:\ngot:  %s\nwant: %s", got, wantQuery)
	}
}
