package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// APIKeyMiddleware authenticates the public API group. A Bearer JWT identifies the logged-in
// user and is validated with JWT_SECRET; without it the shared X-Email-API-Key header (or
// ?api_key for EventSource) is required and the caller stays anonymous (full automation access).
func APIKeyMiddleware() gin.HandlerFunc {
	apiKey := os.Getenv("EMAIL_API_KEY")
	if apiKey == "" {
		// No API key configured — fall through to 403
		apiKey = "__DISABLED__"
	}
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			claims := &JWTClaims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return getJWTSecret(), nil
			})
			if err != nil || !token.Valid {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				return
			}
			c.Set("user_id", claims.UserID)
			c.Set("email", claims.Email)
			c.Set("role", claims.Role)
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
