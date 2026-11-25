package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const mixedEncryptedHistory = `{"model":"gpt-5.1","stream":false,"sequence":900719925474099312345,"input":[{"type":"reasoning","id":"rs_bad","encrypted_content":"bad-reasoning","summary":[]},{"type":"reasoning","id":"rs_good","encrypted_content":"keep-reasoning","summary":[]},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"encrypted_content\":\"user-data\"}"},{"type":"function_call_output","call_id":"call_1","output":[{"type":"input_text","text":"visible tool result"},{"type":"encrypted_content","encrypted_content":"keep-tool-result"}]}]}`
const mixedEncryptedRejection = `{"error":{"code":"invalid_encrypted_content","message":"The encrypted content for item rs_bad could not be verified. Reason: Encrypted content could not be decrypted or parsed."}}`

func TestRejectedEncryptedHistoryTargetsOnlyProvenInvalidItem(t *testing.T) {
	for _, envelope := range []bool{false, true} {
		body := []byte(mixedEncryptedHistory)
		if envelope {
			body = []byte(`{"type":"response.create","response":` + mixedEncryptedHistory + `}`)
		}
		original := string(body)
		for _, rejection := range []string{
			mixedEncryptedRejection,
			`{"error":{"code":"invalid_encrypted_content","param":"input[0].encrypted_content"}}`,
			`{"error":{"code":"invalid_encrypted_content","param":"response.input.0.encrypted_content"}}`,
		} {
			cleaned, removed, err := sanitizeOpenAIRejectedEncryptedHistory(body, []byte(rejection))
			require.NoError(t, err)
			require.Equal(t, original, string(body))
			require.Equal(t, []string{openAIEncryptedContentDigest("bad-reasoning")}, removed)
			require.NotContains(t, string(cleaned), "bad-reasoning")
			require.Contains(t, string(cleaned), "keep-reasoning")
			require.Contains(t, string(cleaned), "keep-tool-result")
			require.Contains(t, string(cleaned), "visible tool result")
			require.Contains(t, string(cleaned), "900719925474099312345")
			require.Equal(t, `{"encrypted_content":"user-data"}`, openAIRequestPayloadView(cleaned).Get(`input.#(type=="function_call").arguments`).String())
		}
		for _, rejection := range []string{
			encryptedToolOutputRejection,
			encryptedRejectionFixture,
			`{"error":{"code":"invalid_encrypted_content"}}`,
			`{"error":{"code":"invalid_encrypted_content","param":"input[3].output"}}`,
			`{"error":{"code":"invalid_encrypted_content","param":"input[1]","message":"The encrypted content for item rs_bad could not be verified"}}`,
		} {
			cleaned, removed, err := sanitizeOpenAIRejectedEncryptedHistory(body, []byte(rejection))
			require.NoError(t, err)
			require.Empty(t, removed)
			require.Equal(t, body, cleaned)
		}
	}
}

func TestRejectedAgentHistoryNeedsPlaintextAndExplicitRejection(t *testing.T) {
	for _, plaintext := range []string{"", `{"type":"input_text","text":"keep agent message"},`} {
		body := []byte(`{"input":[{"type":"agent_message","id":"amsg_bad","author":"a","recipient":"b","content":[` + plaintext + `{"type":"encrypted_content","encrypted_content":"agent-cipher"}]},{"type":"function_call","call_id":"sealed","encrypted_content":"keep-sealed-call","arguments":"{}"}]}`)
		rejection := []byte(`{"error":{"code":"invalid_encrypted_content","message":"The encrypted content for item amsg_bad could not be verified"}}`)
		cleaned, removed, err := sanitizeOpenAIRejectedEncryptedHistory(body, rejection)
		require.NoError(t, err)
		if plaintext == "" {
			require.Empty(t, removed)
			require.Equal(t, body, cleaned)
		} else {
			require.Len(t, removed, 1)
			require.NotContains(t, string(cleaned), "agent-cipher")
			require.Contains(t, string(cleaned), "keep agent message")
			require.Contains(t, string(cleaned), "keep-sealed-call")
			require.False(t, gjson.GetBytes(cleaned, "input.0.id").Exists())
		}
	}
}

func TestMixedEncryptedRecoveryAcrossHTTPPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"standard", "passthrough", "bridge"} {
		for _, terminal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/terminal=%v", path, terminal), func(t *testing.T) {
				response := func(status int, body string) *http.Response {
					return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
				}
				second := response(200, `{"id":"resp_recovered","usage":{"input_tokens":1,"output_tokens":2}}`)
				if path == "bridge" {
					second = response(200, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_recovered\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}}\n\n")
					second.Header.Set("Content-Type", "text/event-stream")
				}
				if terminal {
					second = response(400, strings.ReplaceAll(mixedEncryptedRejection, "rs_bad", "rs_good"))
				}
				upstream := &httpUpstreamSequenceRecorder{responses: []*http.Response{response(400, mixedEncryptedRejection), second}}
				svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}, httpUpstream: upstream, openaiWSStateStore: NewOpenAIWSStateStore(nil)}
				account := &Account{ID: 2, Status: StatusActive, Schedulable: true, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "test-only"}, Extra: map[string]any{"openai_passthrough": path == "passthrough"}}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
				c.Set(openAIWSIngressSessionHashContextKey, "mixed-session")
				SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
				body := []byte(mixedEncryptedHistory)
				var err error
				if path == "bridge" {
					_, err = svc.proxyOpenAIWSHTTPBridgeTurn(context.Background(), c, account, "test-only", body, len(body), "gpt-5.1", "", "", "", "", 1, func([]byte) error { return nil })
				} else {
					_, err = svc.Forward(context.Background(), c, account, body)
				}
				if terminal {
					require.ErrorIs(t, err, errOpenAIInvalidEncryptedContent)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, 2, upstream.callCount)
				require.Contains(t, string(upstream.bodies[0]), "bad-reasoning")
				require.NotContains(t, string(upstream.bodies[1]), "bad-reasoning")
				require.Contains(t, string(upstream.bodies[1]), "keep-reasoning")
				require.Contains(t, string(upstream.bodies[1]), "keep-tool-result")
				require.Contains(t, string(upstream.bodies[1]), "900719925474099312345")
				invalid := svc.sessionInvalidEncryptedContentDigests(0, "mixed-session")
				require.Equal(t, map[string]struct{}{openAIEncryptedContentDigest("bad-reasoning"): {}}, invalid)
				require.Empty(t, svc.sessionInvalidEncryptedContentDigests(0, "other-session"))
			})
		}
	}
}

func TestEncryptedRoutingUnavailableOwnerFallsBackBeforeDispatch(t *testing.T) {
	for _, unavailable := range []string{"missing", "unschedulable", "quota", "wrong-group"} {
		t.Run(unavailable, func(t *testing.T) {
			group := int64(9)
			owner := Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{group}}
			other := owner
			other.ID = 2
			accounts := []Account{owner, other}
			switch unavailable {
			case "missing":
				accounts = []Account{other}
			case "unschedulable":
				accounts[0].Schedulable = false
			case "quota":
				future := time.Now().Add(time.Hour)
				accounts[0].RateLimitResetAt = &future
			case "wrong-group":
				accounts[0].GroupIDs = []int64{100}
			}
			store := NewOpenAIWSStateStore(nil)
			require.NoError(t, store.BindResponseAccount(context.Background(), group, "resp_owner", 1, time.Hour))
			svc := &OpenAIGatewayService{cfg: &config.Config{}, openaiWSStateStore: store, cache: &schedulerTestGatewayCache{}, accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{acquireResults: map[int64]bool{1: true, 2: true}})}
			ctx := svc.WithOpenAIEncryptedContentAffinity(context.Background(), &group, "session", "resp_owner", []byte(encryptedToolOutputFixture))
			scheduler := &defaultOpenAIAccountScheduler{service: svc, stats: newOpenAIAccountRuntimeStats()}
			selection, _, err := scheduler.Select(ctx, OpenAIAccountScheduleRequest{GroupID: &group, Platform: PlatformOpenAI, SessionHash: "session", StickyAccountID: 1})
			require.NoError(t, err)
			require.EqualValues(t, 2, selection.Account.ID)
			body, err := SanitizeOpenAIEncryptedContentForAccount(ctx, selection.Account, []byte(encryptedToolOutputFixture))
			require.NoError(t, err)
			require.Equal(t, encryptedToolOutputFixture, string(body))
		})
	}
}
