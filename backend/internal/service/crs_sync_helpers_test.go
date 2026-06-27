package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildSelectedSet(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		wantNil  bool
		wantSize int
	}{
		{
			name:    "nil input returns nil (backward compatible: create all)",
			ids:     nil,
			wantNil: true,
		},
		{
			name:     "empty slice returns empty map (create none)",
			ids:      []string{},
			wantNil:  false,
			wantSize: 0,
		},
		{
			name:     "single ID",
			ids:      []string{"abc-123"},
			wantNil:  false,
			wantSize: 1,
		},
		{
			name:     "multiple IDs",
			ids:      []string{"a", "b", "c"},
			wantNil:  false,
			wantSize: 3,
		},
		{
			name:     "duplicate IDs are deduplicated",
			ids:      []string{"a", "a", "b"},
			wantNil:  false,
			wantSize: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSelectedSet(tt.ids)
			if tt.wantNil {
				if got != nil {
					t.Errorf("buildSelectedSet(%v) = %v, want nil", tt.ids, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("buildSelectedSet(%v) = nil, want non-nil map", tt.ids)
			}
			if len(got) != tt.wantSize {
				t.Errorf("buildSelectedSet(%v) has %d entries, want %d", tt.ids, len(got), tt.wantSize)
			}
			// Verify all unique IDs are present
			for _, id := range tt.ids {
				if _, ok := got[id]; !ok {
					t.Errorf("buildSelectedSet(%v) missing key %q", tt.ids, id)
				}
			}
		})
	}
}

func TestShouldCreateAccount(t *testing.T) {
	tests := []struct {
		name        string
		crsID       string
		selectedSet map[string]struct{}
		want        bool
	}{
		{
			name:        "nil set allows all (backward compatible)",
			crsID:       "any-id",
			selectedSet: nil,
			want:        true,
		},
		{
			name:        "empty set blocks all",
			crsID:       "any-id",
			selectedSet: map[string]struct{}{},
			want:        false,
		},
		{
			name:        "ID in set is allowed",
			crsID:       "abc-123",
			selectedSet: map[string]struct{}{"abc-123": {}, "def-456": {}},
			want:        true,
		},
		{
			name:        "ID not in set is blocked",
			crsID:       "xyz-789",
			selectedSet: map[string]struct{}{"abc-123": {}, "def-456": {}},
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCreateAccount(tt.crsID, tt.selectedSet)
			if got != tt.want {
				t.Errorf("shouldCreateAccount(%q, %v) = %v, want %v",
					tt.crsID, tt.selectedSet, got, tt.want)
			}
		})
	}
}

func TestCRSLoginHTTPErrorDoesNotExposeRawBody(t *testing.T) {
	secretBody := `{"error":"bad token","access_token":"sk-live-secret","api_key":"sk-api-secret"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/web/auth/login" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		http.Error(w, secretBody, http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := crsLogin(context.Background(), server.Client(), server.URL, "admin", "password")
	if err == nil {
		t.Fatal("expected login error")
	}
	message := err.Error()
	if !strings.Contains(message, "crs login failed") || !strings.Contains(message, "status=401") {
		t.Fatalf("error %q missing operation/status", message)
	}
	for _, secret := range []string{"sk-live-secret", "sk-api-secret", "access_token", "api_key", secretBody} {
		if strings.Contains(message, secret) {
			t.Fatalf("error %q exposed raw CRS login body fragment %q", message, secret)
		}
	}
}

func TestCRSExportHTTPErrorDoesNotExposeRawBody(t *testing.T) {
	secretBody := `{"message":"export failed","refresh_token":"refresh-secret","accounts":[{"api_key":"sk-export-secret"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/admin/sync/export-accounts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("include_secrets") != "true" {
			t.Fatalf("include_secrets query missing")
		}
		http.Error(w, secretBody, http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := crsExportAccounts(context.Background(), server.Client(), server.URL, "admin-token-secret")
	if err == nil {
		t.Fatal("expected export error")
	}
	message := err.Error()
	if !strings.Contains(message, "crs export failed") || !strings.Contains(message, "status=502") {
		t.Fatalf("error %q missing operation/status", message)
	}
	for _, secret := range []string{"refresh-secret", "sk-export-secret", "refresh_token", "api_key", "admin-token-secret", secretBody} {
		if strings.Contains(message, secret) {
			t.Fatalf("error %q exposed raw CRS export body fragment %q", message, secret)
		}
	}
}
