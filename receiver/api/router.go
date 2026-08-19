package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"receiver/handler"
	"receiver/handlers"
	"receiver/middleware"
	"receiver/web"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

// SSEBroker manages Server-Sent Events subscribers per email address.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[string]map[chan string]struct{}
}

var sseBroker = &SSEBroker{
	clients: make(map[string]map[chan string]struct{}),
}

func (b *SSEBroker) Notify(email string, event string, data string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if chans, ok := b.clients[email]; ok {
		for ch := range chans {
			select {
			case ch <- fmt.Sprintf("event: %s\ndata: %s\n\n", event, data):
			default:
			}
		}
	}
}

func (b *SSEBroker) Subscribe(email string) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 10)
	if _, ok := b.clients[email]; !ok {
		b.clients[email] = make(map[chan string]struct{})
	}
	b.clients[email][ch] = struct{}{}
	log.Printf("SSE: subscribed %s (clients: %d)", email, len(b.clients[email]))
	return ch
}

func (b *SSEBroker) Unsubscribe(email string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if chans, ok := b.clients[email]; ok {
		delete(chans, ch)
		close(ch)
		if len(chans) == 0 {
			delete(b.clients, email)
		}
	}
}

func GetBroker() *SSEBroker { return sseBroker }

func SSEHandler(c *gin.Context) {
	email := c.Param("email")
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	ch := sseBroker.Subscribe(email)
	defer sseBroker.Unsubscribe(email, ch)
	c.Writer.WriteString("event: connected\ndata: {}\n\n")
	c.Writer.Flush()
	notify := c.Writer.CloseNotify()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			io.WriteString(c.Writer, msg)
			c.Writer.Flush()
		case <-ticker.C:
			io.WriteString(c.Writer, ": keepalive\n\n")
			c.Writer.Flush()
		}
	}
}

func ginLimiterMiddleware(lmt *limiter.Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		ctx := c.Request.Context()
		limiterCtx, err := lmt.Get(ctx, ip)
		if err != nil {
			log.Printf("Rate limiter error: %v", err)
			c.Next()
			return
		}
		if limiterCtx.Reached {
			c.JSON(429, gin.H{"error": "Too many requests. Please slow down."})
			c.Abort()
			return
		}
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limiterCtx.Limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", limiterCtx.Remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", limiterCtx.Reset))
		c.Next()
	}
}

// sseAuthMiddleware validates JWT from query param for SSE connections.
// EventSource API cannot send custom headers, so token goes in ?token= query.
func sseAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.Query("token")
		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Token required"})
			c.Abort()
			return
		}
		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			// SECURITY: No fallback. Fail closed if not configured.
			log.Fatal("JWT_SECRET is not set — refusing to serve SSE")
		}
		claims := &middleware.JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// getCORSOrigins reads allowed origins from env var CORS_ORIGINS (comma-separated).
// Falls back to same-origin only (empty = no cross-origin allowed).
func getCORSOrigins() []string {
	origins := os.Getenv("CORS_ORIGINS")
	if origins == "" {
		return []string{}
	}
	var result []string
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			result = append(result, o)
		}
	}
	return result
}

func SetupRouter() *gin.Engine {
	// SECURITY: Fail fast if JWT_SECRET not set
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET environment variable is required. Generate one with: openssl rand -hex 32")
	}

	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = "release"
	}
	gin.SetMode(ginMode)
	r := gin.Default()

	// CORS from environment variable (no hardcoded IPs)
	corsOrigins := getCORSOrigins()
	if len(corsOrigins) > 0 {
		r.Use(cors.New(cors.Config{
			AllowOrigins:     corsOrigins,
			AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
			AllowCredentials: false,
			MaxAge:           12 * time.Hour,
		}))
	}

	// Rate limiters
	apiRate, _ := limiter.NewRateFromFormatted("60-M")
	delRate, _ := limiter.NewRateFromFormatted("20-M")
	loginRate, _ := limiter.NewRateFromFormatted("5-M")
	adminRate, _ := limiter.NewRateFromFormatted("30-M")

	apiStore := memory.NewStore()
	delStore := memory.NewStore()
	loginStore := memory.NewStore()
	adminStore := memory.NewStore()

	loginLimiter := limiter.New(loginStore, loginRate)
	adminLimiter := limiter.New(adminStore, adminRate)
	apiLimiter := limiter.New(apiStore, apiRate)
	delLimiter := limiter.New(delStore, delRate)

	apiMW := ginLimiterMiddleware(apiLimiter)
	delMW := ginLimiterMiddleware(delLimiter)

	// Auth handler
	authHandler := handler.NewAuthHandler()

	// Health check (no rate limit, no auth)
	r.GET("/health", handlers.Health)

	// Public config endpoint (for frontend to read domains)
	r.GET("/api/config", web.ConfigHandler)

	// First-time admin seed (only works if no admin exists)
	r.POST("/api/setup", handlers.SeedAdmin)

	// Static frontend (embedded HTML/JS/CSS)
	r.StaticFS("/app", web.StaticFS())

	// Root redirect to app
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/app/")
	})

	// Auth routes (public)
	loginMW := ginLimiterMiddleware(loginLimiter)
	r.POST("/api/auth/login", loginMW, authHandler.Login)

	// Protected routes (require authentication)
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.GET("/auth/me", authHandler.GetCurrentUser)
	}

	// Admin routes (require admin role)
	admin := r.Group("/api/admin")
	adminMW := ginLimiterMiddleware(adminLimiter)
	admin.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware(), adminMW)
	{
		admin.POST("/users", authHandler.Register)
		admin.GET("/users", authHandler.GetAllUsers)
		admin.PUT("/users/:id", authHandler.UpdateUser)
		admin.PUT("/users/:id/password", authHandler.UpdatePassword)
		admin.DELETE("/users/:id", authHandler.DeleteUser)
	}

	// Protected API routes (require authentication)
	authAPI := r.Group("/api")
	authAPI.Use(middleware.AuthMiddleware())
	{
		// Inbox endpoints
		authAPI.GET("/inbox/:email", apiMW, handlers.GetInbox)
		authAPI.DELETE("/inbox/:email", delMW, handlers.DeleteInbox)

		// Automation: create inbox programmatically
		authAPI.POST("/inbox", apiMW, handlers.CreateInboxAPI)

		// Automation: wait for message (blocking, no polling)
		authAPI.GET("/inbox/:email/wait", apiMW, handlers.WaitForMessage)

		// Message endpoints
		authAPI.GET("/message/:id", apiMW, handlers.GetMessage)
		authAPI.DELETE("/message/:id", delMW, handlers.DeleteMessage)
		authAPI.GET("/message/:id/attachments", apiMW, handlers.GetMessageAttachments)
		authAPI.GET("/attachment/:id", apiMW, handlers.GetAttachment)
	}

	// SSE route - auth via query param (EventSource cannot send custom headers)
	r.GET("/sse/:email", sseAuthMiddleware(), SSEHandler)

	return r
}
