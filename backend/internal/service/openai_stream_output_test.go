package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamDataStartsClientOutputOnlyForRealDeltas(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		eventType string
		data      string
		want      bool
	}{
		{
			name:      "text delta starts output",
			eventType: "response.output_text.delta",
			data:      `{"type":"response.output_text.delta","delta":"你"}`,
			want:      true,
		},
		{
			name:      "function args delta starts output",
			eventType: "response.function_call_arguments.delta",
			data:      `{"type":"response.function_call_arguments.delta","delta":"{}"}`,
			want:      true,
		},
		{
			name:      "created is preamble",
			eventType: "response.created",
			data:      `{"type":"response.created"}`,
			want:      false,
		},
		{
			name:      "output item added is metadata",
			eventType: "response.output_item.added",
			data:      `{"type":"response.output_item.added","item":{"type":"message"}}`,
			want:      false,
		},
		{
			name:      "text done is not first token",
			eventType: "response.output_text.done",
			data:      `{"type":"response.output_text.done","text":"完整文本"}`,
			want:      false,
		},
		{
			name:      "completed is terminal not token",
			eventType: "response.completed",
			data:      `{"type":"response.completed","response":{"output":[]}}`,
			want:      false,
		},
		{
			name:      "done terminal is not token",
			eventType: "response.done",
			data:      `{"type":"response.done","response":{"output":[]}}`,
			want:      false,
		},
		{
			name:      "done marker is not token",
			eventType: "",
			data:      `[DONE]`,
			want:      false,
		},
		{
			name:      "legacy chat delta without type starts output",
			eventType: "",
			data:      `{"choices":[{"delta":{"content":"你"}}]}`,
			want:      true,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, openAIStreamDataStartsClientOutput(tt.data, tt.eventType))
		})
	}
}

func TestOpenAIStreamDataShouldStartClientStreamIncludesTerminal(t *testing.T) {
	t.Parallel()

	require.True(t,
		openAIStreamDataShouldStartClientStream(
			`{"type":"response.completed","response":{"output":[]}}`,
			"response.completed",
		),
	)
	require.True(t,
		openAIStreamDataShouldStartClientStream(
			`{"type":"response.done","response":{"output":[]}}`,
			"response.done",
		),
	)
	require.True(t,
		openAIStreamDataShouldStartClientStream(
			`{"type":"response.output_text.delta","delta":"你"}`,
			"response.output_text.delta",
		),
	)
	require.True(t,
		openAIStreamDataShouldStartClientStream(
			`{"type":"response.output_item.added","item":{"type":"message"}}`,
			"response.output_item.added",
		),
	)
	require.True(t,
		openAIStreamDataShouldStartClientStream(
			`{"type":"response.content_part.added","part":{"type":"output_text"}}`,
			"response.content_part.added",
		),
	)
	require.False(t,
		openAIStreamDataShouldStartClientStream(
			`{"type":"response.created","response":{"id":"resp"}}`,
			"response.created",
		),
	)
}

func TestOpenAIWSTokenEventOnlyForRealDeltas(t *testing.T) {
	t.Parallel()

	require.True(t, isOpenAIWSTokenEvent("response.output_text.delta"))
	require.True(t, isOpenAIWSTokenEvent("response.output_audio.delta"))
	require.True(t, isOpenAIWSTokenEvent("response.function_call_arguments.delta"))

	require.False(t, isOpenAIWSTokenEvent("response.created"))
	require.False(t, isOpenAIWSTokenEvent("response.output_item.added"))
	require.False(t, isOpenAIWSTokenEvent("response.content_part.added"))
	require.False(t, isOpenAIWSTokenEvent("response.output_text.done"))
	require.False(t, isOpenAIWSTokenEvent("response.completed"))
	require.False(t, isOpenAIWSTokenEvent("response.done"))
	require.False(t, isOpenAIWSTokenEvent("response.failed"))
}

func TestOpenAIStreamingCompletedOnlyDoesNotStampFirstToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.created","response":{"id":"resp_completed_only"}}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_completed_only","usage":{"input_tokens":2,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}

	svc := &OpenAIGatewayService{}
	result, err := svc.handleStreamingResponse(
		context.Background(),
		resp,
		c,
		&Account{ID: 1, Platform: PlatformOpenAI, Name: "acc"},
		time.Now(),
		"gpt-5.5",
		"gpt-5.5",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Nil(t, result.firstTokenMs)
	require.Contains(t, rec.Body.String(), "response.completed")
	require.Equal(t, 2, result.usage.InputTokens)
	require.Equal(t, 1, result.usage.OutputTokens)
}
