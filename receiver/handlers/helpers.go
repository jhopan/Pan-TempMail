package handlers

import (
	"net/http"
	"regexp"

	"receiver/database"
	"receiver/repository"

	"github.com/gin-gonic/gin"
)

// regexpCompile compiles a regex pattern safely.
func regexpCompile(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re
}

// AuthorizeInbox reports whether the caller may act on the inbox named email.
// Admins and API-key/guest callers (no user identity on the request) keep full access;
// a logged-in user only reaches inboxes whose owner_id is their own user id.
// On refusal it writes the 403 response and returns false.
func AuthorizeInbox(c *gin.Context, email string) bool {
	if role, ok := c.Get("role"); ok && role == "admin" {
		return true
	}
	identity, hasIdentity := c.Get("user_id")
	if !hasIdentity {
		// API-key or SSE guest: automation mode, not tied to any account.
		return true
	}
	userID, ok := identity.(int)
	if !ok {
		return denyInbox(c)
	}
	ownerID, err := repository.GetInboxOwnerID(database.Conn(), email)
	if err != nil || !ownerID.Valid || int(ownerID.Int64) != userID {
		return denyInbox(c)
	}
	return true
}

// callerUserID returns the logged-in user id, or 0 when the request carries no user identity.
func callerUserID(c *gin.Context) int {
	identity, ok := c.Get("user_id")
	if !ok {
		return 0
	}
	userID, ok := identity.(int)
	if !ok {
		return 0
	}
	return userID
}

func denyInbox(c *gin.Context) bool {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Inbox ini bukan milik akun kamu"})
	return false
}
