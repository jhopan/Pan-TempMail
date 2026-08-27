package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyMiddleware allows access via X-Email-API-Key header.
// Used by automation scripts and web UI guest mode (no JWT login required).
func APIKeyMiddleware() gin.HandlerFunc {
	apiKey := os.Getenv("EMAIL_API_KEY")
	if apiKey == "" {
		// No API key configured — fall through to 403
		apiKey = "__DISABLED__"
	}
	return func(c *gin.Context) {
		// If client sends JWT, let AuthMiddleware handle it instead
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			c.Next()
			return
		}
		key := c.GetHeader("X-Email-API-Key")
		if key == "" {
			// Try query param fallback (for SSE/EventSource)
			key = c.Query("api_key")
		}
		if key != apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "API key required (X-Email-API-Key header)",
			})
			return
		}
		c.Next()
	}
}
