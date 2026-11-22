package service

import (
	"bufio"
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

func TestPassthroughKeepaliveCoversOutputGapsAndPartialEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	stages := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		defer writer.Close()
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_alive\"}}\n\n")
		for i, stage := range stages {
			select {
			case <-stage:
			case <-finished:
				return
			}
			switch i {
			case 0:
				_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\n")
			case 1:
				_, _ = io.WriteString(writer, "event: response.output_text.delta\n")
			case 2:
				_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"second\"}\n\n"+
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_alive\",\"usage\":{\"input_tokens\":3,\"output_tokens\":5}}}\n\n")
			}
		}
	}()
	type outcome struct {
		result *openaiStreamingResultPassthrough
		err    error
	}
	results := make(chan outcome, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
		response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: reader}
		result, err := svc.handleStreamingResponsePassthrough(r.Context(), response, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "", "")
		results <- outcome{result, err}
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 8 * time.Second
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	var body strings.Builder
	beats := 0
	for scanner.Scan() {
		line := scanner.Text()
		body.WriteString(line)
		body.WriteByte('\n')
		if line == ":" {
			require.Less(t, beats, len(stages))
			if beats == 0 {
				require.Equal(t, ":\n", body.String(), "lifecycle events must stay buffered")
			}
			close(stages[beats])
			beats++
		}
	}
	require.NoError(t, scanner.Err())
	result := <-results
	require.NoError(t, result.err)
	require.NotNil(t, result.result)
	require.Equal(t, 3, beats)
	require.Contains(t, body.String(), "\"delta\":\"first\"}\n\n:\n\n")
	require.Contains(t, body.String(), "event: response.output_text.delta\n:\ndata: ", "heartbeat must not add a blank line inside an event")
	require.Contains(t, body.String(), "response.completed")
	require.Equal(t, 3, result.result.usage.InputTokens)
	require.Equal(t, 5, result.result.usage.OutputTokens)
}

func TestPassthroughKeepaliveWriteFailureStillDrainsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	go func() {
		defer writer.Close()
		time.Sleep(2200 * time.Millisecond)
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_drain\",\"usage\":{\"input_tokens\":3,\"output_tokens\":5}}}\n\n")
	}()
	w := &failedKeepaliveWriter{ResponseRecorder: httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: reader}
	result, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, time.Now(), "", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, w.writes)
	require.Equal(t, 3, result.usage.InputTokens)
	require.Equal(t, 5, result.usage.OutputTokens)
}
