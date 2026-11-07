package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

type kiroModelCapabilities struct {
	ID     string `json:"modelId"`
	Name   string `json:"modelName"`
	Schema struct {
		Properties map[string]any `json:"properties"`
	} `json:"additionalModelRequestFieldsSchema"`
}
type kiroCatalogEntry struct {
	Models  []kiroModelCapabilities
	Expires time.Time
}

var kiroCatalog = struct {
	sync.Mutex
	Entries map[string]kiroCatalogEntry
}{Entries: map[string]kiroCatalogEntry{}}

// Discovery uses the same account token and fixed proxy as generation.
func (s *GatewayService) kiroModels(ctx context.Context, a *Account, token string) ([]kiroModelCapabilities, error) {
	proxy, err := kiroProxy(a)
	if err != nil {
		return nil, err
	}
	identity := sha256.Sum256([]byte(token + "\x00" + proxy + "\x00" + a.GetCredential("profile_arn") + "\x00" + kiro.EnvironmentKey(a.Credentials)))
	key := fmt.Sprintf("%d:%x", a.ID, identity)
	now := time.Now()
	kiroCatalog.Lock()
	entry, ok := kiroCatalog.Entries[key]
	kiroCatalog.Unlock()
	if ok && entry.Expires.After(now) {
		return entry.Models, nil
	}
	payload := map[string]any{"origin": "KIRO_CLI"}
	if arn := a.GetCredential("profile_arn"); arn != "" && a.GetCredential("auth_method") != "api_key" {
		payload["profileArn"] = arn
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiro.ModelsURL(a.Credentials), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = prepareKiroHTTPRequest(req, a.Credentials, token, kiro.OperationModels)
	resp, err := s.httpUpstream.Do(req, proxy, a.ID, a.Concurrency)
	if err != nil {
		return nil, kiro.TransportError(kiro.OperationModels, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, kiro.ResponseError(kiro.OperationModels, resp)
	}
	var catalog struct {
		Models []kiroModelCapabilities `json:"models"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&catalog) != nil || len(catalog.Models) == 0 {
		return nil, &kiro.UpstreamError{Operation: kiro.OperationModels, Reason: "invalid_response"}
	}
	kiroCatalog.Lock()
	for k, v := range kiroCatalog.Entries {
		if !v.Expires.After(now) {
			delete(kiroCatalog.Entries, k)
		}
	}
	if len(kiroCatalog.Entries) >= 1024 {
		for k := range kiroCatalog.Entries {
			delete(kiroCatalog.Entries, k)
			break
		}
	}
	kiroCatalog.Entries[key] = kiroCatalogEntry{Models: catalog.Models, Expires: now.Add(5 * time.Minute)}
	kiroCatalog.Unlock()
	return catalog.Models, nil
}
func (s *AccountTestService) FetchKiroAccountModels(ctx context.Context, a *Account) ([]openai.Model, error) {
	gateway := &GatewayService{httpUpstream: s.httpUpstream, claudeTokenProvider: s.claudeTokenProvider}
	a, token, err := gateway.kiroAccessToken(ctx, a)
	if err != nil {
		return nil, err
	}
	catalog, err := gateway.kiroModels(ctx, a, token)
	if err != nil {
		return nil, err
	}
	out := make([]openai.Model, 0, len(catalog))
	for _, m := range catalog {
		out = append(out, openai.Model{ID: m.ID, Object: "model", Type: "model", DisplayName: m.Name})
	}
	return out, nil
}

func (s *OpenAIGatewayService) kiroModelsResponse(ctx context.Context, a *Account, codex bool) (*OpenAIModelsResponse, error) {
	if s.kiroGateway == nil {
		return nil, errors.New("Kiro gateway unavailable")
	}
	a, token, err := s.kiroGateway.kiroAccessToken(ctx, a)
	if err != nil {
		return nil, err
	}
	catalog, err := s.kiroGateway.kiroModels(ctx, a, token)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0, len(catalog))
	for _, m := range catalog {
		if codex {
			entries = append(entries, map[string]any{"slug": m.ID, "display_name": m.Name, "description": m.Name})
		} else {
			entries = append(entries, map[string]any{"id": m.ID, "object": "model", "owned_by": "kiro"})
		}
	}
	var body []byte
	if codex {
		body, err = json.Marshal(map[string]any{"models": entries})
	} else {
		body, err = json.Marshal(map[string]any{"object": "list", "data": entries})
	}
	return &OpenAIModelsResponse{Body: body, ETag: codexModelsManifestBodyETag(body)}, err
}
