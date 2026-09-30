package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMappedResponseModelPreservesOtherData(t *testing.T) {
	svc := &OpenAIGatewayService{}
	body := `{"model":"alias","text":"mapped alias","tool":{"model":"mapped","arguments":"{\"model\":\"alias\"}"}}`
	want := `{"model":"public","text":"mapped alias","tool":{"model":"mapped","arguments":"{\"model\":\"alias\"}"}}`
	require.Equal(t, want, string(svc.replaceModelInResponseBody([]byte(body), "mapped", "public")))
	require.Equal(t, "data: "+want, svc.replaceModelInSSELine("data: "+body, "mapped", "public"))
	for _, body := range []string{
		`{"model":"alias",`, `{"model":"alias"} trailing`, `{"model":null}`, `{"model":42}`, `{"model":{}}`, `{"model":[]}`, `{"model":true}`,
		`{"text":"alias","tool":{"model":"alias"}}`,
	} {
		require.Equal(t, body, string(svc.replaceModelInResponseBody([]byte(body), "mapped", "public")))
		require.Equal(t, "data: "+body, svc.replaceModelInSSELine("data: "+body, "mapped", "public"))
	}
	for _, models := range [][2]string{{"same", "same"}, {"", "public"}, {"mapped", ""}} {
		body := `{"model":"alias","response":{"model":"alias"}}`
		require.Equal(t, body, string(svc.replaceModelInResponseBody([]byte(body), models[0], models[1])))
		require.Equal(t, "data: "+body, svc.replaceModelInSSELine("data: "+body, models[0], models[1]))
	}
	require.Equal(t, `data: {"response":{"model":42}}`, svc.replaceModelInSSELine(`data: {"response":{"model":42}}`, "mapped", "public"))
	require.Equal(t, `{"model":"public"}`, string(svc.replaceModelInResponseBody([]byte(`{"model":""}`), "mapped", "public")))
}

// Exercise both streaming processors so substring fast paths cannot bypass the rewrite.
func TestMappedResponseModelForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, returned := range []string{"zhipu/glm-5.3", "glm-5.3-alias"} {
			for _, mapped := range []string{"ZHIPU/GLM-5.3", "public"} {
				for _, kind := range []string{"json", "chat", "responses"} {
					name := kind + "/" + returned + "/" + mapped
					if passthrough {
						name += "/passthrough"
					}
					t.Run(name, func(t *testing.T) {
						model := returned
						if mapped != "public" {
							model = "public"
						}
						payload := `{"model":"` + returned + `","choices":[{"delta":{"content":"keep alias","tool_calls":[{"function":{"arguments":"{\"model\":\"alias\"}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
						want := strings.Replace(payload, `"model":"`+returned+`"`, `"model":"`+model+`"`, 1)
						contentType := "application/json"
						body := payload
						if kind == "responses" {
							payload = `{"type":"response.completed","response":{"id":"resp_1","model":"` + returned + `","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`
							want = strings.Replace(payload, `"model":"`+returned+`"`, `"model":"`+model+`"`, 1)
						}
						if kind != "json" {
							contentType = "text/event-stream"
							body = "data: " + payload + "\n\ndata: [DONE]\n\n"
						}
						rec := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(rec)
						c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
						resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
						svc := &OpenAIGatewayService{}
						account := &Account{ID: 1}
						var err error
						if kind == "json" {
							if passthrough {
								_, err = svc.handleNonStreamingResponsePassthrough(context.Background(), resp, c, account, "public", mapped)
							} else {
								_, err = svc.handleNonStreamingResponse(context.Background(), resp, c, account, "public", mapped)
							}
						} else {
							if passthrough {
								_, err = svc.handleStreamingResponsePassthrough(context.Background(), resp, c, account, time.Now(), "public", mapped)
							} else {
								_, err = svc.handleStreamingResponse(context.Background(), resp, c, account, time.Now(), "public", mapped)
							}
						}
						require.NoError(t, err)
						require.Contains(t, rec.Body.String(), want)
						require.Equal(t, returned, observedUpstreamResponseModel(c))
					})
				}
			}
		}
	}
}

func TestRawChatCompletionsMappedModelPrivacySwitch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		hidden bool
		want   string
	}{
		{name: "hidden", hidden: true, want: "public-model"},
		{name: "exposed", hidden: false, want: "upstream-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cacheHideMappedUpstreamModelForTest(t, tc.hidden)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			body := "data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"model\":\"upstream-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"}}]}\n\n" +
				"data: {\"id\":\"chatcmpl_1\",\"object\":\"chat.completion.chunk\",\"model\":\"upstream-model\",\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n" +
				"data: [DONE]\n\n"
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}
			svc := &OpenAIGatewayService{
				cfg:            &config.Config{},
				settingService: NewSettingService(&gatewayTTLSettingRepo{}, &config.Config{}),
			}

			result, err := svc.streamRawChatCompletions(
				c, resp, &Account{ID: 1}, "public-model", "public-model", "upstream-model",
				nil, nil, time.Now(), 2,
			)

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Contains(t, rec.Body.String(), `"model":"`+tc.want+`"`)
			require.Equal(t, "upstream-model", observedUpstreamResponseModel(c))
		})
	}
}
