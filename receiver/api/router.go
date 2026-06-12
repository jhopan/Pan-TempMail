package api

import (
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"receiver/handler"
	"receiver/handlers"
	"receiver/middleware"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

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

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://213.35.108.142:3000", "http://213.35.108.142", "http://localhost:3000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		MaxAge:           12 * time.Hour,
	}))

	// Rate limiters
	apiRate, _ := limiter.NewRateFromFormatted("60-M")   // 60 req/min per IP
	delRate, _ := limiter.NewRateFromFormatted("20-M")
	loginRate, _ := limiter.NewRateFromFormatted("5-M")  // 5 login attempts/min
	adminRate, _ := limiter.NewRateFromFormatted("30-M") // 30 admin req/min    // 20 deletes/min per IP

	apiStore := memory.NewStore()
	delStore := memory.NewStore()

	loginStore := memory.NewStore()
	loginLimiter := limiter.New(loginStore, loginRate)
	adminStore := memory.NewStore()
	adminLimiter := limiter.New(adminStore, adminRate)
	apiLimiter := limiter.New(apiStore, apiRate)
	delLimiter := limiter.New(delStore, delRate)

	apiMW := ginLimiterMiddleware(apiLimiter)
	delMW := ginLimiterMiddleware(delLimiter)

	// Auth handler
	authHandler := handler.NewAuthHandler()

	// Health (no rate limit)
	r.GET("/health", handlers.Health)

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
	authAPI := r.Group("/")
	authAPI.Use(middleware.AuthMiddleware())
	{
	authAPI.GET("/inbox/:email", apiMW, handlers.GetInbox)
	authAPI.DELETE("/inbox/:email", delMW, handlers.DeleteInbox)
	authAPI.GET("/message/:id", apiMW, handlers.GetMessage)
	authAPI.DELETE("/message/:id", delMW, handlers.DeleteMessage)
	authAPI.GET("/message/:id/attachments", apiMW, handlers.GetMessageAttachments)
	authAPI.GET("/attachment/:id", apiMW, handlers.GetAttachment)

		authAPI.GET("/sse/:email", SSEHandler)
	}

	return r
}
