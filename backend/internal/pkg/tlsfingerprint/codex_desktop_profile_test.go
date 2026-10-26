package tlsfingerprint

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"

	utls "github.com/refraction-networking/utls"
	"github.com/stretchr/testify/require"
)

func TestCodexDesktopMacOSProfileMatchesCapturedJA3(t *testing.T) {
	profile := CodexDesktopMacOSProfile()
	require.Equal(t, uint16(utls.VersionTLS10), profile.TLSVersionMin)
	require.Equal(t, uint16(utls.VersionTLS12), profile.TLSVersionMax)
	require.Empty(t, profile.ALPNProtocols)

	ja3 := strings.Join([]string{
		strconv.Itoa(int(profile.TLSVersionMax)),
		joinUint16(profile.CipherSuites),
		joinUint16(profile.Extensions),
		joinUint16(profile.Curves),
		joinUint16(profile.PointFormats),
	}, ",")
	require.Equal(t, "771,255-49196-49195-49188-49187-49162-49161-49160-49200-49199-49192-49191-49172-49171-49170-157-156-61-60-53-47-10,0-10-11-13-5-18-23,23-24-25,0", ja3)
	sum := md5.Sum([]byte(ja3))
	require.Equal(t, "e4d448cdfe06dc1243c1eb026c74ac9a", hex.EncodeToString(sum[:]))

	spec := buildClientHelloSpecFromProfile(profile)
	require.Equal(t, uint16(utls.VersionTLS10), spec.TLSVersMin)
	require.Equal(t, uint16(utls.VersionTLS12), spec.TLSVersMax)
	require.Len(t, spec.Extensions, 7)
}

func joinUint16(values []uint16) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Itoa(int(value)))
	}
	return strings.Join(parts, "-")
}
