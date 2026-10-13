package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// This exercises the real HTTP middleware with production-sized request bodies.
// It does not contact an upstream or change the production application.
func TestExcelBPSImageAdmissionLargeBodiesRespectByteBudget(t *testing.T) {
	for _, tc := range []struct {
		bodyMiB int
		allowed int
	}{{8, 16}, {16, 8}} {
		t.Run(fmt.Sprintf("%dMiB", tc.bodyMiB), func(t *testing.T) {
			const prefix = `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,`
			// The current user turn is text, but an earlier turn contains an image.
			const suffix = `"}]},{"role":"user","content":"continue"}]}`
			body := prefix + strings.Repeat("A", (tc.bodyMiB<<20)-len(prefix)-len(suffix)) + suffix
			require.Len(t, body, tc.bodyMiB<<20)
			entered := make(chan struct{}, tc.allowed)
			release := make(chan struct{})
			var once sync.Once
			var wg sync.WaitGroup
			t.Cleanup(func() { once.Do(func() { close(release) }); wg.Wait() })
			router := bpsImageTestRouter(bpsImageTestSettings{
				enabled: true, bodyLimitMiB: 64, budgetMiB: 1024, maxRequests: 128,
			}, func(c *gin.Context) {
				if c.GetHeader("Hold") == "true" {
					entered <- struct{}{}
					<-release
				}
				c.Status(http.StatusNoContent)
			})
			for i := 0; i < tc.allowed; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
					req.Header.Set("Hold", "true")
					rw := httptest.NewRecorder()
					router.ServeHTTP(rw, req)
					if rw.Code != http.StatusNoContent {
						t.Errorf("admitted image returned %d", rw.Code)
					}
				}()
				select {
				case <-entered:
				case <-time.After(10 * time.Second):
					t.Fatal("image did not enter handler")
				}
			}
			blocked := httptest.NewRecorder()
			router.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
			require.Equal(t, http.StatusServiceUnavailable, blocked.Code)
			require.Contains(t, blocked.Body.String(), "basispoints_image_request_busy")
			text := httptest.NewRecorder()
			router.ServeHTTP(text, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hello"}`)))
			require.Equal(t, http.StatusNoContent, text.Code)
			once.Do(func() { close(release) })
			wg.Wait()
			for i := 0; i < 3; i++ {
				recovered := httptest.NewRecorder()
				router.ServeHTTP(recovered, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
				require.Equal(t, http.StatusNoContent, recovered.Code)
			}
			t.Logf("128 configured slots, %d MiB body: %d admitted, next image rejected; text passes; image succeeds after release", tc.bodyMiB, tc.allowed)
		})
	}
}
