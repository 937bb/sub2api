package http2

import (
	"testing"

	"github.com/imroc/req/v3/http2"
)

func TestCustomReceiveWindow(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings []http2.Setting
		want     int32
	}{
		{"default", nil, transportDefaultStreamFlow},
		{"omitted", []http2.Setting{{ID: http2.SettingEnablePush, Val: 0}}, 65535},
		{"codex", []http2.Setting{{ID: http2.SettingInitialWindowSize, Val: 2097152}}, 2097152},
		{"zero", []http2.Setting{{ID: http2.SettingInitialWindowSize, Val: 0}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cc := &ClientConn{t: &Transport{Settings: tc.settings}}
			if got := cc.initialStreamReceiveWindow(); got != tc.want {
				t.Fatalf("window = %d, want %d", got, tc.want)
			}
		})
	}
}
