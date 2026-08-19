package handlers

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"receiver/database"
	"receiver/repository"

	"github.com/gin-gonic/gin"
)

// GetInbox returns a paginated list of messages for an inbox.
// GET /inbox/:email?page=1&per_page=20
func GetInbox(c *gin.Context) {
	email := c.Param("email")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	pages := repository.DefaultPageParams(page, perPage)

	result, err := repository.GetInboxMessages(database.Conn(), email, pages)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch inbox"})
		return
	}
	c.JSON(http.StatusOK, result)
}

// CreateInboxAPI creates a new inbox programmatically.
// POST /inbox  {"email": "user@domain.com"} or {"username": "user", "domain": "domain.com"}
// If email is omitted, a random one is generated from DOMAINS env.
func CreateInboxAPI(c *gin.Context) {
	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Domain   string `json:"domain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// If email not provided, generate from username + domain
	if req.Email == "" {
		if req.Username == "" {
			req.Username = randomString(8)
		}
		if req.Domain == "" {
			// Pick first domain from DOMAINS env
			domains := strings.Split(os.Getenv("DOMAINS"), ",")
			if len(domains) == 0 || domains[0] == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "No domain configured"})
				return
			}
			req.Domain = strings.TrimSpace(domains[0])
		}
		req.Email = req.Username + "@" + req.Domain
	}

	// Check if inbox already exists
	inboxID, err := repository.GetInboxID(database.Conn(), req.Email)
	if err != nil {
		// Create new inbox
		inboxID, err = repository.CreateInbox(database.Conn(), req.Email)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create inbox"})
			return
		}
	} else {
		// Extend expiry for existing inbox
		repository.ExtendInboxExpiry(database.Conn(), req.Email)
	}

	c.JSON(http.StatusCreated, gin.H{
		"email":     req.Email,
		"inbox_id":  inboxID,
		"expires":   "24h",
		"wait_url":  "/inbox/" + req.Email + "/wait",
		"sse_url":   "/sse/" + req.Email + "?token=<JWT>",
	})
}

// WaitForMessage blocks until a message matching criteria arrives, or timeout.
// GET /inbox/:email/wait?timeout=60&from=otp&subject=verify
// This is the automation-friendly endpoint: no polling needed.
func WaitForMessage(c *gin.Context) {
	email := c.Param("email")

	// Parse query params
	timeoutStr := c.DefaultQuery("timeout", "60")
	timeoutInt, _ := strconv.Atoi(timeoutStr)
	if timeoutInt < 1 || timeoutInt > 300 {
		timeoutInt = 60
	}
	timeout := time.Duration(timeoutInt) * time.Second

	matchFrom := c.Query("from")
	matchSubject := c.Query("subject")

	// Ensure inbox exists
	_, err := repository.GetInboxID(database.Conn(), email)
	if err != nil {
		// Auto-create inbox if not exists
		_, _ = repository.CreateInbox(database.Conn(), email)
	}

	msg, err := repository.WaitForMessage(database.Conn(), email, timeout, matchSubject, matchFrom)
	if err != nil {
		c.JSON(http.StatusRequestTimeout, gin.H{
			"error":   "timeout",
			"message": "No matching message arrived within timeout",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         msg.ID,
		"from":       msg.Sender,
		"subject":    msg.Subject,
		"text_body":  msg.TextBody,
		"html_body":  msg.HtmlBody,
		"codes":      parseCodesArray(msg.Codes),
		"links":      parseLinksArray(msg.Links),
		"received_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// GetMessage returns a single message by ID.
// GET /message/:id
func GetMessage(c *gin.Context) {
	id := c.Param("id")
	msg, err := repository.GetMessageByID(database.Conn(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":        msg.ID,
		"sender":    msg.Sender,
		"subject":   msg.Subject,
		"text_body": msg.TextBody,
		"html_body": msg.HtmlBody,
		"codes":     parseCodesArray(msg.Codes),
		"links":     parseLinksArray(msg.Links),
	})
}

// DeleteMessage deletes a message by ID.
// DELETE /message/:id
func DeleteMessage(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}
	if err := repository.DeleteMessage(database.Conn(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete message"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Message deleted"})
}

// DeleteInbox deletes an inbox and all its messages + attachments.
// DELETE /inbox/:email
func DeleteInbox(c *gin.Context) {
	email := c.Param("email")
	if err := repository.DeleteInbox(database.Conn(), email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete inbox"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Inbox deleted"})
}

// GetMessageAttachments returns attachments for a message.
// GET /message/:id/attachments
func GetMessageAttachments(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}
	attachments, err := repository.GetAttachmentsByMessageID(database.Conn(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch attachments"})
		return
	}
	c.JSON(http.StatusOK, attachments)
}

// GetAttachment downloads an attachment by ID.
// GET /attachment/:id
func GetAttachment(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid attachment ID"})
		return
	}
	att, data, err := repository.GetAttachmentByID(database.Conn(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Attachment not found"})
		return
	}
	c.Header("Content-Disposition", "attachment; filename=\""+att.Filename+"\"")
	c.Data(http.StatusOK, att.ContentType, data)
}

// Health returns a simple health check response.
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})
}

// SeedAdmin creates the first admin user. Only works if no users exist.
// POST /api/setup {"email":"admin@example.com","password":"..."}
func SeedAdmin(c *gin.Context) {
	// Check if any user exists
	var count int
	db := database.Conn()
	if err := db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Setup already completed — users exist"})
		return
	}

	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request. email and password(min 6 chars) required"})
		return
	}

	repo := repository.NewUserRepository()
	hashedPassword, err := repository.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password"})
		return
	}

	user, err := repo.Create(c.Request.Context(), req.Email, hashedPassword, "admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create admin user"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Admin user created successfully",
		"user":    user,
		"hint":    "Now login via POST /api/auth/login",
	})
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	sb := make([]byte, length)
	for i := range sb {
		sb[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond) // ensure different seeds
	}
	return string(sb)
}

// parseCodesArray parses a JSON array string into a string slice.
func parseCodesArray(jsonStr string) []string {
	if jsonStr == "" || jsonStr == "[]" {
		return []string{}
	}
	// Simple parser: extract quoted strings from JSON array
	var result []string
	inQuote := false
	var current strings.Builder
	for _, c := range jsonStr {
		if c == '"' {
			if inQuote {
				result = append(result, current.String())
				current.Reset()
			}
			inQuote = !inQuote
		} else if inQuote {
			if c == '\\' {
				continue // skip escape
			}
			current.WriteRune(c)
		}
	}
	return result
}

// parseLinksArray parses a JSON array string into a string slice.
func parseLinksArray(jsonStr string) []string {
	return parseCodesArray(jsonStr) // same logic
}
