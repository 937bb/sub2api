package admin

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSettingsHideMappedUpstreamModelRoundTripAndOmission(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})

	rec := doUpdateSettings(t, h, map[string]any{"site_name": "Legacy Gateway"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.True(t, gjson.Get(rec.Body.String(), "data.hide_mapped_upstream_model").Bool())
	_, stored := repo.values[service.SettingKeyHideMappedUpstreamModel]
	require.False(t, stored, "an omitted legacy setting should remain absent and use its secure default")

	rec = doUpdateSettings(t, h, map[string]any{"hide_mapped_upstream_model": false}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.False(t, gjson.Get(rec.Body.String(), "data.hide_mapped_upstream_model").Bool())
	require.Equal(t, "false", repo.values[service.SettingKeyHideMappedUpstreamModel])

	rec = doUpdateSettings(t, h, map[string]any{"site_name": "Keep Disabled"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.False(t, gjson.Get(rec.Body.String(), "data.hide_mapped_upstream_model").Bool())
	require.Equal(t, "false", repo.values[service.SettingKeyHideMappedUpstreamModel])

	rec = doUpdateSettings(t, h, map[string]any{"hide_mapped_upstream_model": true}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.True(t, gjson.Get(rec.Body.String(), "data.hide_mapped_upstream_model").Bool())
	require.Equal(t, "true", repo.values[service.SettingKeyHideMappedUpstreamModel])
}
