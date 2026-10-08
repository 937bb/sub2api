package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type openAISiwcModelsUpdater interface {
	UpdateOpenAISiwcModels(ctx context.Context, id int64, subject, clientID string, models []string) (bool, error)
}

// fetchSIWCModels uses only the public SIWC catalog and the selected account's
// proxy. A missing configured proxy must not silently become a direct request.
func (s *OpenAIGatewayService) fetchSIWCModels(ctx context.Context, account *Account) ([]string, error) {
	if s == nil || !account.IsOpenAISiwc() {
		return nil, fmt.Errorf("SIWC account is required")
	}
	proxyURL := ""
	if account.ProxyID != nil {
		proxy := account.Proxy
		if proxy == nil {
			if s.proxyRepo == nil {
				return nil, fmt.Errorf("SIWC configured proxy is unavailable")
			}
			var err error
			proxy, err = s.proxyRepo.GetByID(ctx, *account.ProxyID)
			if err != nil || proxy == nil {
				return nil, fmt.Errorf("SIWC configured proxy is unavailable")
			}
		}
		proxyURL = strings.TrimSpace(proxy.URL())
		if proxyURL == "" {
			return nil, fmt.Errorf("SIWC configured proxy is unavailable")
		}
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("resolve SIWC model catalog credentials: %w", err)
	}
	client, err := newSIWCClient(proxyURL)
	if err != nil {
		return nil, err
	}
	models, err := client.Models(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetch SIWC model catalog: %w", err)
	}
	if s.accountRepo != nil && account.ID > 0 {
		updater, ok := s.accountRepo.(openAISiwcModelsUpdater)
		if !ok {
			return nil, fmt.Errorf("SIWC model catalog persistence is unavailable")
		}
		updated, err := updater.UpdateOpenAISiwcModels(ctx, account.ID, account.GetCredential("subject"), account.GetCredential("client_id"), models)
		if err != nil {
			return nil, fmt.Errorf("persist SIWC model catalog: %w", err)
		}
		if !updated {
			return nil, fmt.Errorf("SIWC account identity changed during model discovery")
		}
	}
	account.Credentials = shallowCopyMap(account.Credentials)
	account.Credentials["siwc_models"] = append([]string(nil), models...)
	return models, nil
}

// siwcModelsBody projects SIWC's listed model IDs into the public OpenAI list
// envelope. Account aliases and group visibility remain the caller's policy.
func siwcModelsBody(models []string) ([]byte, error) {
	type modelEntry struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
		Created int64  `json:"created"`
	}
	entries := make([]modelEntry, 0, len(models))
	for _, model := range dedupeAndSortModelIDs(models) {
		entries = append(entries, modelEntry{ID: model, Object: "model", OwnedBy: "openai"})
	}
	return json.Marshal(struct {
		Object string       `json:"object"`
		Data   []modelEntry `json:"data"`
	}{Object: "list", Data: entries})
}
