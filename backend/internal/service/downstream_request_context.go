package service

import "github.com/gin-gonic/gin"

// downstreamRequestContextErr distinguishes a real client disconnect from an
// upstream transport that independently returned a context cancellation.
func downstreamRequestContextErr(c *gin.Context) error {
	if c == nil || c.Request == nil {
		return nil
	}
	return c.Request.Context().Err()
}
