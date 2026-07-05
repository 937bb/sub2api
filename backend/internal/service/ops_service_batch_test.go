package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpsServiceRecordErrorBatch_SanitizesAndBatches(t *testing.T) {
	t.Parallel()

	var captured []*OpsInsertErrorLogInput
	repo := &opsRepoMock{
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			captured = append(captured, inputs...)
			return int64(len(inputs)), nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	msg := " upstream failed: https://example.com?access_token=secret-value "
	detail := `{"authorization":"Bearer secret-token"}`
	entries := []*OpsInsertErrorLogInput{
		{
			ErrorBody:            `{"error":"bad","access_token":"secret"}`,
			UpstreamStatusCode:   intPtr(-10),
			UpstreamErrorMessage: strPtr(msg),
			UpstreamErrorDetail:  strPtr(detail),
			UpstreamErrors: []*OpsUpstreamErrorEvent{
				{
					AccountID:          -2,
					UpstreamStatusCode: 429,
					UpstreamURL:        "https://api.example.com/v1/chat/completions?api_key=secret#frag",
					Message:            " token leaked ",
					Detail:             `{"refresh_token":"secret"}`,
				},
			},
		},
		{
			ErrorPhase: "upstream",
			ErrorType:  "upstream_error",
			CreatedAt:  time.Now().UTC(),
		},
	}

	require.NoError(t, svc.RecordErrorBatch(context.Background(), entries))
	require.Len(t, captured, 2)

	first := captured[0]
	require.Equal(t, "internal", first.ErrorPhase)
	require.Equal(t, "api_error", first.ErrorType)
	require.Nil(t, first.UpstreamStatusCode)
	require.NotNil(t, first.UpstreamErrorMessage)
	require.NotContains(t, *first.UpstreamErrorMessage, "secret-value")
	require.Contains(t, *first.UpstreamErrorMessage, "access_token=***")
	require.NotNil(t, first.UpstreamErrorDetail)
	require.NotContains(t, *first.UpstreamErrorDetail, "secret-token")
	require.NotContains(t, first.ErrorBody, "secret")
	require.Nil(t, first.UpstreamErrors)
	require.NotNil(t, first.UpstreamErrorsJSON)
	require.NotContains(t, *first.UpstreamErrorsJSON, "secret")
	require.NotContains(t, *first.UpstreamErrorsJSON, "api_key")
	require.Contains(t, *first.UpstreamErrorsJSON, `"upstream_endpoint":"/v1/chat/completions"`)
	require.Contains(t, *first.UpstreamErrorsJSON, "[REDACTED]")

	second := captured[1]
	require.Equal(t, "upstream", second.ErrorPhase)
	require.Equal(t, "upstream_error", second.ErrorType)
	require.False(t, second.CreatedAt.IsZero())
}

func TestOpsUpstreamErrorEventEndpointFromSafeURL(t *testing.T) {
	t.Parallel()

	require.Equal(t, "/v1/responses/compact", endpointFromSafeUpstreamURL("https://api.openai.com/v1/responses/compact"))
	require.Equal(t, "/v1/responses", endpointFromSafeUpstreamURL("wss://chatgpt.com/backend-api/codex/responses"))
	require.Equal(t, "/v1/responses/compact", endpointFromSafeUpstreamURL("https://chatgpt.com/backend-api/codex/responses/compact"))
	require.Equal(t, "/v1/chat/completions", endpointFromSafeUpstreamURL("https://compat.example.com/base/v1/chat/completions"))
	require.Equal(t, "", endpointFromSafeUpstreamURL("https://example.com/not-openai"))
	require.Equal(t, "", endpointFromSafeUpstreamURL("https://example.com/proxy/v1/responses-archive"))
	require.Equal(t, "", endpointFromSafeUpstreamURL("https://compat.example.com/base/v1/chat/completions-archive"))
	require.Equal(t, "", endpointFromSafeUpstreamURL("https://compat.example.com/base/v1/chat/completions/archive"))
	require.Equal(t, "", endpointFromSafeUpstreamURL("https://api.openai.com/v1/images/generations/archive"))
	require.Equal(t, "/v1beta/models", endpointFromSafeUpstreamURL("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"))
	require.Equal(t, "/v1beta/models", endpointFromSafeUpstreamURL("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:streamGenerateContent"))
	require.Equal(t, "", endpointFromSafeUpstreamURL("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:countTokens"))
	require.Equal(t, "", endpointFromSafeUpstreamURL("https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent/debug"))
	require.Equal(t, "/v1/responses", endpointFromSafeUpstreamURL("/proxy/v1/responses"))
}

func TestOpsServiceRecordErrorBatch_PreservesExplicitEventEndpoint(t *testing.T) {
	t.Parallel()

	var captured []*OpsInsertErrorLogInput
	repo := &opsRepoMock{
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			captured = append(captured, input)
			return 1, nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	require.NoError(t, svc.RecordError(context.Background(), &OpsInsertErrorLogInput{
		ErrorPhase: "upstream",
		ErrorType:  "upstream_error",
		UpstreamErrors: []*OpsUpstreamErrorEvent{
			{
				AccountID:          101,
				UpstreamStatusCode: http.StatusTooManyRequests,
				UpstreamURL:        "https://api.openai.com/v1/responses?key=secret",
				UpstreamEndpoint:   "/v1/chat/completions",
				Message:            "rate limited",
			},
		},
	}))

	require.Len(t, captured, 1)
	require.NotNil(t, captured[0].UpstreamErrorsJSON)
	require.Contains(t, *captured[0].UpstreamErrorsJSON, `"upstream_endpoint":"/v1/chat/completions"`)
	require.Contains(t, *captured[0].UpstreamErrorsJSON, `"upstream_url":"https://api.openai.com/v1/responses"`)
	require.NotContains(t, *captured[0].UpstreamErrorsJSON, "secret")
}

func TestOpsServiceRecordErrorBatch_FallsBackToSingleInsert(t *testing.T) {
	t.Parallel()

	var (
		batchCalls  int
		singleCalls int
	)
	repo := &opsRepoMock{
		BatchInsertErrorLogsFn: func(ctx context.Context, inputs []*OpsInsertErrorLogInput) (int64, error) {
			batchCalls++
			return 0, errors.New("batch failed")
		},
		InsertErrorLogFn: func(ctx context.Context, input *OpsInsertErrorLogInput) (int64, error) {
			singleCalls++
			return int64(singleCalls), nil
		},
	}
	svc := NewOpsService(repo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	err := svc.RecordErrorBatch(context.Background(), []*OpsInsertErrorLogInput{
		{ErrorMessage: "first"},
		{ErrorMessage: "second"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, batchCalls)
	require.Equal(t, 2, singleCalls)
}

func strPtr(v string) *string {
	return &v
}
