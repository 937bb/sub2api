package middleware

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/gin-gonic/gin"
)

// UnambiguousModel rejects ambiguous selectors before model mapping, routing
// and billing. It is independent of group allowlists and preserves body bytes.
func UnambiguousModel() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
			if err != nil {
				if RejectRequestBodyReadError(c, "middleware.unambiguous_model", err) {
					return
				}
				AbortWithRequestBodyReadFailure(c, "middleware.unambiguous_model", err)
				return
			}
			requestmodel.ResetRequestBody(c.Request, body)
			if err := requestmodel.ValidateBodySelectors(c.GetHeader("Content-Type"), body); err != nil {
				MarkIngressRejected(c, IngressRejectReason("ambiguous_model"))
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": gin.H{
					"type": "invalid_request_error", "code": "ambiguous_model", "param": "model", "message": err.Error(),
				}})
				return
			}
		}
		c.Next()
	}
}
