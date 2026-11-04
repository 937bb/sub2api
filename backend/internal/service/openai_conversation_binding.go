package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// OpenAIConversationBinding survives cache expiry and account deletion. It stores
// no credentials, client identifiers or conversation content.
type OpenAIConversationBinding struct {
	AccountID           int64
	CredentialAccountID int64
	IdentityHash        string
}

type OpenAIConversationBindingRepository interface {
	GetOpenAIConversationBinding(context.Context, string) (*OpenAIConversationBinding, error)
	ClaimOpenAIConversationBinding(context.Context, string, *OpenAIConversationBinding) (*OpenAIConversationBinding, error)
}

type OpenAIConversationError struct {
	Status        int
	Code, Message string
}

func (e *OpenAIConversationError) Error() string { return e.Code + ": " + e.Message }
func conversationUnavailable() error {
	return &OpenAIConversationError{http.StatusServiceUnavailable, "conversation_account_unavailable", "This conversation's account is unavailable. Retry later or start a new conversation; automatic account switching is disabled."}
}
func conversationStoreUnavailable() error {
	return &OpenAIConversationError{http.StatusServiceUnavailable, "conversation_binding_unavailable", "Conversation binding could not be verified. Retry later."}
}
func conversationIdentityRequired() error {
	return &OpenAIConversationError{http.StatusConflict, "conversation_binding_unknown", "The original conversation account cannot be verified. Reuse the original session-id or start a new conversation."}
}

type openAIConversationContextKey struct{}
type openAIConversationState struct {
	mu                 sync.Mutex
	key                string
	continuation       bool
	previousResponseID string
	apiKeyID           int64
	binding            *OpenAIConversationBinding
}

// WithOpenAIConversationBinding namespaces client signals by authenticated key
// and group. Content-derived fallback is accepted only while its binding remains
// identifiable; a continuation with unknown ownership never silently migrates.
func (s *OpenAIGatewayService) WithOpenAIConversationBinding(ctx context.Context, c *gin.Context, groupID *int64, sessionHash, previousResponseID string, body []byte) context.Context {
	if s == nil || s.cfg == nil || !s.cfg.Gateway.OpenAIConversationBindingEnabled {
		return ctx
	}
	keyID := getAPIKeyIDFromContext(c)
	seed := strings.TrimSpace(sessionHash)
	if seed == "" && previousResponseID != "" {
		seed = "response:" + previousResponseID
	}
	key := ""
	if keyID > 0 && seed != "" {
		key = openAIConversationBindingKey(keyID, derefGroupID(groupID), seed)
	}
	state := &openAIConversationState{key: key, apiKeyID: keyID, previousResponseID: previousResponseID, continuation: previousResponseID != "" || openAIConversationHasHistory(body)}
	return context.WithValue(ctx, openAIConversationContextKey{}, state)
}

func openAIConversationHasHistory(body []byte) bool {
	root := openAIRequestPayloadView(body)
	for _, path := range []string{"input", "messages"} {
		for _, item := range root.Get(path).Array() {
			if role := item.Get("role").String(); role == "assistant" || role == "tool" {
				return true
			}
			switch item.Get("type").String() {
			case "reasoning", "compaction", "compaction_summary", "function_call", "function_call_output", "custom_tool_call", "custom_tool_call_output", "item_reference":
				return true
			}
			for _, part := range item.Get("content").Array() {
				if part.Get("type").String() == "tool_result" || part.Get("type").String() == "tool_use" {
					return true
				}
			}
		}
	}
	return gjson.GetBytes(body, "previous_response_id").String() != ""
}

func openAIConversationStateFromContext(ctx context.Context) *openAIConversationState {
	state, _ := ctx.Value(openAIConversationContextKey{}).(*openAIConversationState)
	return state
}

func conversationIdentityHash(owner *Account) string {
	namespace := codexAccountIdentityNamespace(owner)
	device := codexAccountInstallationID(owner)
	if namespace == "" || device == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(namespace + "\x00" + device))
	return fmt.Sprintf("%x", sum)
}

func (s *OpenAIGatewayService) conversationBindingForAccount(ctx context.Context, account *Account) (*OpenAIConversationBinding, error) {
	owner, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil || owner == nil || !owner.IsOpenAIOAuthLike() {
		return nil, conversationUnavailable()
	}
	hash := conversationIdentityHash(owner)
	if hash == "" {
		return nil, conversationIdentityRequired()
	}
	return &OpenAIConversationBinding{AccountID: account.ID, CredentialAccountID: owner.ID, IdentityHash: hash}, nil
}

// selectBoundOpenAIConversation runs before both legacy and advanced schedulers.
// Sticky escape, weighted selection and excluded-account failover cannot override
// the durable owner. Returning handled=true forbids a fallback to the account pool.
func (s *OpenAIGatewayService) selectBoundOpenAIConversation(ctx context.Context, req OpenAIAccountScheduleRequest) (*AccountSelectionResult, OpenAIAccountScheduleDecision, bool, error) {
	state := openAIConversationStateFromContext(ctx)
	if state == nil || NormalizeOpenAICompatiblePlatform(req.Platform) != PlatformOpenAI || req.RequiredImageCapability != "" {
		return nil, OpenAIAccountScheduleDecision{}, false, nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	repo, ok := s.accountRepo.(OpenAIConversationBindingRepository)
	if !ok {
		return nil, OpenAIAccountScheduleDecision{}, true, conversationStoreUnavailable()
	}
	if state.binding == nil && state.key != "" {
		b, err := repo.GetOpenAIConversationBinding(ctx, state.key)
		if err != nil {
			return nil, OpenAIAccountScheduleDecision{}, true, conversationStoreUnavailable()
		}
		state.binding = b
	}
	if state.previousResponseID != "" {
		previous, readErr := repo.GetOpenAIConversationBinding(ctx, openAIConversationBindingKey(state.apiKeyID, derefGroupID(req.GroupID), "response:"+state.previousResponseID))
		if readErr != nil {
			return nil, OpenAIAccountScheduleDecision{}, true, conversationStoreUnavailable()
		}
		if previous != nil {
			if state.binding != nil && state.binding.IdentityHash != previous.IdentityHash {
				return nil, OpenAIAccountScheduleDecision{}, true, conversationIdentityRequired()
			}
			if state.binding == nil {
				if state.key == "" {
					state.key = openAIConversationBindingKey(state.apiKeyID, derefGroupID(req.GroupID), "response:"+state.previousResponseID)
				}
				state.binding, readErr = repo.ClaimOpenAIConversationBinding(ctx, state.key, previous)
				if readErr != nil || state.binding == nil {
					return nil, OpenAIAccountScheduleDecision{}, true, conversationStoreUnavailable()
				}
				if state.binding.IdentityHash != previous.IdentityHash {
					return nil, OpenAIAccountScheduleDecision{}, true, conversationIdentityRequired()
				}
			}
		}
	}
	// Import only a response owner verified against the authenticated API key.
	// A shared legacy sticky cache does not prove tenant-specific ownership.
	if state.binding == nil && state.continuation {
		var id int64
		var err error
		if state.previousResponseID != "" {
			id, err = s.getOpenAIWSStateStore().GetResponseAccount(ctx, derefGroupID(req.GroupID), state.previousResponseID)
		}
		if err != nil {
			return nil, OpenAIAccountScheduleDecision{}, true, conversationStoreUnavailable()
		}

		if id > 0 {
			_, ownerKey, found, ownerErr := s.getOpenAIWSStateStore().GetHTTPResponseOwner(ctx, derefGroupID(req.GroupID), state.previousResponseID)
			if ownerErr != nil {
				return nil, OpenAIAccountScheduleDecision{}, true, conversationStoreUnavailable()
			}
			if !found || ownerKey != state.apiKeyID {
				return nil, OpenAIAccountScheduleDecision{}, true, conversationIdentityRequired()
			}
			a, lookupErr := s.accountRepo.GetByID(ctx, id)
			if lookupErr != nil {
				return nil, OpenAIAccountScheduleDecision{}, true, conversationUnavailable()
			}
			if a != nil && a.IsOpenAIOAuthLike() {
				b, bindErr := s.conversationBindingForAccount(ctx, a)
				if bindErr != nil {
					return nil, OpenAIAccountScheduleDecision{}, true, bindErr
				}
				if state.key == "" {
					return nil, OpenAIAccountScheduleDecision{}, true, conversationIdentityRequired()
				}
				state.binding, err = repo.ClaimOpenAIConversationBinding(ctx, state.key, b)
				if err != nil || state.binding == nil {
					return nil, OpenAIAccountScheduleDecision{}, true, conversationStoreUnavailable()
				}
			}
		}
	}
	if state.binding == nil {
		return nil, OpenAIAccountScheduleDecision{}, false, nil
	}
	b := state.binding
	decision := OpenAIAccountScheduleDecision{Layer: "conversation_binding", SelectedAccountID: b.AccountID, StickySessionHit: true}
	a, err := s.accountRepo.GetByID(ctx, b.AccountID)
	if err != nil || a == nil || !a.IsOpenAIOAuthLike() {
		return nil, decision, true, conversationUnavailable()
	}
	current, err := s.conversationBindingForAccount(ctx, a)
	if err != nil || current.IdentityHash != b.IdentityHash {
		return nil, decision, true, conversationUnavailable()
	}
	if _, excluded := req.ExcludedIDs[a.ID]; excluded {
		return nil, decision, true, conversationUnavailable()
	}
	req.StickyAccountID = a.ID
	req.DisableStickyEscape = true
	req.PreserveStickyBinding = true
	req.StickyWeighted = false
	req.RequirePrivacySet = s.openAIGroupRequiresPrivacySet(ctx, req.GroupID)
	// A response-only continuation still needs a nonempty selection seed.
	if req.SessionHash == "" {
		req.SessionHash = state.key
	}
	scheduler := &defaultOpenAIAccountScheduler{service: s, stats: newOpenAIAccountRuntimeStats()}
	selected, _, err := scheduler.selectBySessionHash(ctx, req)
	if err != nil || selected == nil || selected.Account == nil {
		return nil, decision, true, conversationUnavailable()
	}
	decision.SelectedAccountType = selected.Account.Type
	if req.PreviousResponseID != "" {
		previous, readErr := repo.GetOpenAIConversationBinding(ctx, openAIConversationBindingKey(state.apiKeyID, derefGroupID(req.GroupID), "response:"+req.PreviousResponseID))
		matches := readErr == nil && previous != nil && previous.IdentityHash == b.IdentityHash
		if previous == nil && readErr == nil {
			responseOwner, ownerErr := s.getOpenAIWSStateStore().GetResponseAccount(ctx, derefGroupID(req.GroupID), req.PreviousResponseID)
			_, ownerKey, found, tenantErr := s.getOpenAIWSStateStore().GetHTTPResponseOwner(ctx, derefGroupID(req.GroupID), req.PreviousResponseID)
			matches = ownerErr == nil && tenantErr == nil && found && ownerKey == state.apiKeyID && responseOwner == a.ID
		}
		if !matches {
			if selected.ReleaseFunc != nil {
				selected.ReleaseFunc()
			}
			return nil, decision, true, conversationIdentityRequired()
		}
		decision.StickyPreviousHit = true
	}

	return selected, decision, true, nil
}

func (s *OpenAIGatewayService) claimSelectedOpenAIConversation(ctx context.Context, selected *AccountSelectionResult) error {
	state := openAIConversationStateFromContext(ctx)
	if state == nil || selected == nil || selected.Account == nil || !selected.Account.IsOpenAIOAuthLike() {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.key == "" {
		return conversationIdentityRequired()
	}
	b, err := s.conversationBindingForAccount(ctx, selected.Account)
	if err != nil {
		return err
	}
	if state.binding != nil {
		if state.binding.IdentityHash != b.IdentityHash {
			return conversationUnavailable()
		}
		return nil
	}
	if state.continuation {
		return conversationIdentityRequired()
	}
	repo, ok := s.accountRepo.(OpenAIConversationBindingRepository)
	if !ok {
		return conversationStoreUnavailable()
	}
	winner, err := repo.ClaimOpenAIConversationBinding(ctx, state.key, b)
	if err != nil || winner == nil {
		return conversationStoreUnavailable()
	}
	state.binding = winner
	if winner.IdentityHash != b.IdentityHash {
		return conversationUnavailable()
	}
	return nil
}

// Check again at the outbound boundary in case credentials changed while waiting.
func (s *OpenAIGatewayService) validateOpenAIConversationIdentity(ctx context.Context, account, owner *Account) error {
	state := openAIConversationStateFromContext(ctx)
	if state == nil || account == nil || !account.IsOpenAIOAuthLike() {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	// Refresh identity metadata after any concurrency wait. Credentials may have
	// changed since selection, while the original snapshot still looks valid.
	fresh, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil || fresh == nil || !fresh.IsOpenAIOAuthLike() {
		return conversationUnavailable()
	}
	freshOwner, err := resolveCredentialAccount(ctx, s.accountRepo, fresh)
	if err != nil || state.binding == nil || state.binding.IdentityHash != conversationIdentityHash(freshOwner) {
		return conversationUnavailable()
	}
	if state.binding == nil || state.binding.IdentityHash != conversationIdentityHash(owner) {
		return conversationUnavailable()
	}
	return nil
}

func openAIConversationBindingKey(apiKeyID, groupID int64, seed string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("openai-conversation:v1:key:%d:group:%d:session:%s", apiKeyID, groupID, seed)))
	return fmt.Sprintf("%x", sum)
}

// Response aliases share the durable owner so response-only continuations can
// reconnect across HTTP/WS instances after Redis TTLs expire.
func (s *OpenAIGatewayService) bindOpenAIConversationResponse(ctx context.Context, groupID int64, responseID string, account *Account) {
	state := openAIConversationStateFromContext(ctx)
	if state == nil || responseID == "" || account == nil || !account.IsOpenAIOAuthLike() {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.binding == nil {
		return
	}
	repo, ok := s.accountRepo.(OpenAIConversationBindingRepository)
	if !ok {
		return
	}
	winner, err := repo.ClaimOpenAIConversationBinding(context.WithoutCancel(ctx), openAIConversationBindingKey(state.apiKeyID, groupID, "response:"+responseID), state.binding)
	if err != nil || winner == nil || winner.IdentityHash != state.binding.IdentityHash {
		slog.Error("openai_conversation_response_binding_failed", "account_id", account.ID)
	}
}
