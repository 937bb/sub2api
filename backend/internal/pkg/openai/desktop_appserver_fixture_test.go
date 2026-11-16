package openai

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type codexDesktopAppServerFixture struct {
	AppVersion         string   `json:"app_version"`
	EngineVersion      string   `json:"engine_version"`
	UserAgent          string   `json:"user_agent"`
	Originator         string   `json:"originator"`
	OpenAIBeta         string   `json:"openai_beta"`
	AbsentHeaders      []string `json:"absent_headers"`
	WireHeaderNames    []string `json:"wire_header_names"`
	BodyFields         []string `json:"body_fields"`
	ClientMetadata     []string `json:"client_metadata_fields"`
	TurnMetadataFields []string `json:"turn_metadata_fields"`
	RequestKind        string   `json:"request_kind"`
	AnalyticsEnabled   *bool    `json:"analytics_enabled"`
	Relations          struct {
		SessionEqualsThread      bool `json:"session_equals_thread"`
		PromptCacheEqualsSession bool `json:"prompt_cache_equals_session"`
	} `json:"relations"`
	Prewarm struct {
		Type                     string   `json:"type"`
		Generate                 bool     `json:"generate"`
		PromptCacheEqualsSession bool     `json:"prompt_cache_equals_session"`
		BodyFields               []string `json:"body_fields"`
		ClientMetadata           []string `json:"client_metadata_fields"`
		TurnMetadataFields       []string `json:"turn_metadata_fields"`
		RequestKind              string   `json:"request_kind"`
		AnalyticsEnabled         *bool    `json:"analytics_enabled"`
	} `json:"prewarm"`
	Turn struct {
		Type                     string   `json:"type"`
		HasPreviousResponseID    bool     `json:"has_previous_response_id"`
		PromptCacheEqualsSession bool     `json:"prompt_cache_equals_session"`
		BodyFields               []string `json:"body_fields"`
		ClientMetadata           []string `json:"client_metadata_fields"`
		TurnMetadataFields       []string `json:"turn_metadata_fields"`
		RequestKind              string   `json:"request_kind"`
		AnalyticsEnabled         *bool    `json:"analytics_enabled"`
	} `json:"turn"`
}

type codexDesktopTelemetryFixture struct {
	AppVersion    string `json:"app_version"`
	EngineVersion string `json:"engine_version"`
	HTTPInference struct {
		TraceparentPresent              bool `json:"traceparent_present"`
		EmptyTracestateHeaderPresent    bool `json:"empty_tracestate_header_present"`
		TraceIDMatchesExportedTelemetry bool `json:"trace_id_matches_exported_telemetry"`
	} `json:"http_inference"`
	WebSocketInference struct {
		HandshakeTraceHeadersPresent    bool   `json:"handshake_trace_headers_present"`
		FrameTraceparentField           string `json:"frame_traceparent_field"`
		EmptyFrameTracestateOmitted     bool   `json:"empty_frame_tracestate_omitted"`
		TraceIDMatchesExportedTelemetry bool   `json:"trace_id_matches_exported_telemetry"`
	} `json:"websocket_inference"`
	TurnCosts struct {
		Method                  string   `json:"method"`
		Path                    string   `json:"path"`
		WireHeaderNames         []string `json:"wire_header_names"`
		BodyFields              []string `json:"body_fields"`
		TraceHeadersPresent     bool     `json:"trace_headers_present"`
		CompatibleEmptyResponse struct {
			Turns []any `json:"turns"`
		} `json:"compatible_empty_response"`
	} `json:"turn_costs"`
	OTLPResource          map[string]string `json:"otlp_resource"`
	OTLPScopes            []string          `json:"otlp_scopes"`
	RepresentativeMetrics []string          `json:"representative_metrics"`
}

func readCodexDesktopAppServerFixture(t *testing.T, name string) codexDesktopAppServerFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	var fixture codexDesktopAppServerFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	return fixture
}

func TestCodexDesktopAppServerFixturesMatchCompiledIdentity(t *testing.T) {
	wantUA := CodexDesktopOriginator + "/" + CodexDesktopEngineVersion + CodexDesktopUserAgentSuffix
	for _, name := range []string{
		"codex_desktop_appserver_http_20261116.json",
		"codex_desktop_appserver_ws_20261116.json",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := readCodexDesktopAppServerFixture(t, name)
			require.Equal(t, CodexDesktopAppVersion, fixture.AppVersion)
			require.Equal(t, CodexDesktopEngineVersion, fixture.EngineVersion)
			require.Equal(t, CodexDesktopOriginator, fixture.Originator)
			require.Equal(t, wantUA, fixture.UserAgent)
			require.NotEmpty(t, fixture.WireHeaderNames)
			require.Contains(t, fixture.AbsentHeaders, "X-Codex-Installation-Id")
			require.Contains(t, fixture.AbsentHeaders, "Accept-Language")
			require.Contains(t, fixture.AbsentHeaders, "OAI-Product-Sku")
		})
	}
}

func TestCodexDesktopAppServerRequestRelationships(t *testing.T) {
	httpFixture := readCodexDesktopAppServerFixture(t, "codex_desktop_appserver_http_20261116.json")
	require.True(t, httpFixture.Relations.SessionEqualsThread)
	require.True(t, httpFixture.Relations.PromptCacheEqualsSession)
	require.Equal(t, "turn", httpFixture.RequestKind)
	require.NotNil(t, httpFixture.AnalyticsEnabled)
	require.False(t, *httpFixture.AnalyticsEnabled)
	require.Contains(t, httpFixture.TurnMetadataFields, "analytics_enabled")
	require.Contains(t, httpFixture.TurnMetadataFields, "installation_id")
	require.Contains(t, httpFixture.TurnMetadataFields, "session_id")
	require.Contains(t, httpFixture.TurnMetadataFields, "thread_id")
	require.Contains(t, httpFixture.TurnMetadataFields, "turn_id")
	require.Contains(t, httpFixture.TurnMetadataFields, "window_id")

	wsFixture := readCodexDesktopAppServerFixture(t, "codex_desktop_appserver_ws_20261116.json")
	require.Equal(t, "responses_websockets=2026-02-06", wsFixture.OpenAIBeta)
	require.Equal(t, "response.create", wsFixture.Prewarm.Type)
	require.False(t, wsFixture.Prewarm.Generate)
	require.True(t, wsFixture.Prewarm.PromptCacheEqualsSession)
	require.Equal(t, "prewarm", wsFixture.Prewarm.RequestKind)
	require.NotNil(t, wsFixture.Prewarm.AnalyticsEnabled)
	require.False(t, *wsFixture.Prewarm.AnalyticsEnabled)
	require.NotContains(t, wsFixture.Prewarm.ClientMetadata, "root_turn_id")
	require.NotContains(t, wsFixture.Prewarm.TurnMetadataFields, "turn_started_at_unix_ms")
	require.Equal(t, "response.create", wsFixture.Turn.Type)
	require.True(t, wsFixture.Turn.HasPreviousResponseID)
	require.True(t, wsFixture.Turn.PromptCacheEqualsSession)
	require.Equal(t, "turn", wsFixture.Turn.RequestKind)
	require.NotNil(t, wsFixture.Turn.AnalyticsEnabled)
	require.False(t, *wsFixture.Turn.AnalyticsEnabled)
	require.Contains(t, wsFixture.Turn.ClientMetadata, "root_turn_id")
	require.Contains(t, wsFixture.Turn.TurnMetadataFields, "turn_started_at_unix_ms")
}

func TestCodexDesktopTelemetryFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/codex_desktop_telemetry_20261116.json")
	require.NoError(t, err)
	var fixture codexDesktopTelemetryFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.Equal(t, CodexDesktopAppVersion, fixture.AppVersion)
	require.Equal(t, CodexDesktopEngineVersion, fixture.EngineVersion)
	require.True(t, fixture.HTTPInference.TraceparentPresent)
	require.True(t, fixture.HTTPInference.EmptyTracestateHeaderPresent)
	require.True(t, fixture.HTTPInference.TraceIDMatchesExportedTelemetry)
	require.False(t, fixture.WebSocketInference.HandshakeTraceHeadersPresent)
	require.Equal(t, "ws_request_header_traceparent", fixture.WebSocketInference.FrameTraceparentField)
	require.True(t, fixture.WebSocketInference.EmptyFrameTracestateOmitted)
	require.True(t, fixture.WebSocketInference.TraceIDMatchesExportedTelemetry)
	require.Equal(t, "POST", fixture.TurnCosts.Method)
	require.Equal(t, "/v1/analytics/codex/turn-costs", fixture.TurnCosts.Path)
	require.Equal(t, []string{"turn_ids"}, fixture.TurnCosts.BodyFields)
	require.False(t, fixture.TurnCosts.TraceHeadersPresent)
	require.NotNil(t, fixture.TurnCosts.CompatibleEmptyResponse.Turns)
	require.Empty(t, fixture.TurnCosts.CompatibleEmptyResponse.Turns)
	require.Equal(t, "codex-app-server", fixture.OTLPResource["service.name"])
	require.Equal(t, CodexDesktopEngineVersion, fixture.OTLPResource["service.version"])
	require.Contains(t, fixture.OTLPScopes, "codex-app-server")
	require.Contains(t, fixture.RepresentativeMetrics, "codex.websocket.continuation")
}
