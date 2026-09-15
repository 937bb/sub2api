package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexConversationWS_OmittedFollowupMetadataKeepsConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cfg := passthroughLifecycleConfig()
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
			cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
			upstream := newStagedPassthroughConn()
			svc := newPassthroughLifecycleService(cfg, upstream)
			if mode == OpenAIWSIngressModePassthrough {
				svc.settingService = newOpenAIGatewayServiceWithSettings(t, &OpenAIFastPolicySettings{Rules: []OpenAIFastPolicyRule{{
					ServiceTier: OpenAIFastTierPriority, Action: BetaPolicyActionBlock,
					Scope: BetaPolicyScopeAll, ErrorMessage: "test policy denial",
					FallbackAction: BetaPolicyActionPass,
				}}}).settingService
			}
			completed := `{"type":"response.completed","response":{"id":"resp_one","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`
			capture := &openAIWSCaptureConn{events: [][]byte{[]byte(completed), []byte(completed)}}
			dialer := &openAIWSCaptureDialer{conn: capture}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			svc.openaiWSPool = pool
			account := conversationTestAccount(AccountTypeOAuth, "full")
			account.Concurrency, account.Status, account.Schedulable = 1, StatusActive, true
			account.Credentials["access_token"] = "test-token"
			account.Extra["openai_oauth_responses_websockets_v2_mode"] = mode
			contexts := make(chan *gin.Context, 1)
			server, serverErr := startPassthroughLifecycleServerWithHooks(t, ctx, svc, account, func(c *gin.Context) *OpenAIWSIngressHooks {
				c.Set("api_key", &APIKey{ID: 1})
				contexts <- c
				return nil
			})
			defer server.Close()
			headers := make(http.Header)
			headers.Set("session-id", "stale-header-session")
			headers.Set(openAIWSTurnMetadataHeader, `{"session_id":"stale-header-session","thread_id":"stale-header-thread","sandbox":"header-sandbox"}`)
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &coderws.DialOptions{HTTPHeader: headers})
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			requests := []string{
				`{"type":"response.create","model":"gpt-5.1","client_metadata":{"x-codex-turn-metadata":"{\"session_id\":\"body-session\",\"thread_id\":\"body-thread\",\"sandbox\":\"frame-sandbox\"}"},"input":"hi"}`,
				`{"type":"response.create","model":"gpt-5.1","previous_response_id":"resp_one","input":[{"type":"function_call_output","call_id":"call_one","output":"done"}]}`,
			}
			var sent [][]byte
			for _, request := range requests {
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(request)))
				if mode == OpenAIWSIngressModePassthrough {
					sent = append(sent, requirePassthroughUpstreamWrite(t, upstream, 5*time.Second))
					upstream.Send(completed)
				}
				_, event, err := client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String(), "%s", event)
			}
			if mode == OpenAIWSIngressModePassthrough {
				blocked := `{"type":"response.create","model":"gpt-5.1","service_tier":"priority","client_metadata":{"session_id":"rejected-session","thread_id":"rejected-thread"},"input":"blocked"}`
				require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(blocked)))
				_, event, err := client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "error", gjson.GetBytes(event, "type").String(), "%s", event)
				_ = client.CloseNow()
			} else {
				_ = client.Close(coderws.StatusNormalClosure, "done")
			}
			select {
			case err := <-serverErr:
				if mode == OpenAIWSIngressModePassthrough {
					var closeErr *OpenAIWSClientCloseError
					require.ErrorAs(t, err, &closeErr)
					require.Equal(t, coderws.StatusPolicyViolation, closeErr.StatusCode())
					select {
					case request := <-upstream.writes:
						t.Fatalf("blocked frame reached upstream: %s", request)
					default:
					}
					c := <-contexts
					binding, _ := c.Get(codexWSConversationContextKey)
					require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "session", "body-session"), binding.(codexWSConversationBinding).sessionID)
				} else {
					require.NoError(t, err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if mode == OpenAIWSIngressModeCtxPool {
				require.Equal(t, 1, dialer.DialCount(), "continuation must reuse its upstream connection")
				require.Len(t, capture.writes, 2)
				for _, write := range capture.writes {
					sent = append(sent, []byte(requestToJSONString(write)))
				}
			}
			require.Len(t, sent, 2)
			for _, body := range sent {
				require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "session", "body-session"), gjson.GetBytes(body, "client_metadata.session_id").String())
				require.Equal(t, scopeCodexAccountIdentityValue(account, 1, "thread", "body-thread"), gjson.GetBytes(body, "client_metadata.thread_id").String())
			}
			firstMeta := gjson.Parse(gjson.GetBytes(sent[0], "client_metadata."+openAIWSTurnMetadataHeader).String())
			require.Equal(t, "frame-sandbox", firstMeta.Get("sandbox").String(), "frame metadata must not be overwritten by handshake metadata")
			require.NotEqual(t, gjson.GetBytes(sent[0], "client_metadata.turn_id").String(), gjson.GetBytes(sent[1], "client_metadata.turn_id").String())
			require.Equal(t, "done", gjson.GetBytes(sent[1], "input.0.output").String())
		})
	}
}
