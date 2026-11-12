package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"receiver/api"
	"receiver/database"
	"receiver/middleware"
	"receiver/repository"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	ownerUserID   = 42
	otherUserID   = 43
	adminUserID   = 1
	ownershipRole = "user"
)

func ownershipAPIKey() string { return os.Getenv("EMAIL_API_KEY") }

func uniqueEmail(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d@ownership.invalid", prefix, time.Now().UnixNano())
}

func signUserJWT(t *testing.T, userID int, email, role string, ttl time.Duration) string {
	t.Helper()
	claims := middleware.JWTClaims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		t.Fatalf("sign test JWT: %v", err)
	}
	return signed
}

func ownershipRouter(t *testing.T) *gin.Engine {
	t.Helper()
	t.Setenv("GIN_MODE", gin.TestMode)
	return api.SetupRouter()
}

// doRequest calls the router with either a JWT (Authorization header) or an API key header.
func doRequest(r *gin.Engine, method, path, token, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if apiKey != "" {
		req.Header.Set("X-Email-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func postJSON(r *gin.Engine, path, token, apiKey, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if apiKey != "" {
		req.Header.Set("X-Email-API-Key", apiKey)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func createOwnedInbox(t *testing.T, email string, ownerID int) int {
	t.Helper()
	id, err := repository.CreateInboxOwned(database.Conn(), email, ownerID)
	if err != nil {
		t.Fatalf("create owned inbox %s: %v", email, err)
	}
	return id
}

func createMessage(t *testing.T, inboxID int) int {
	t.Helper()
	id, err := repository.SaveMessage(database.Conn(), inboxID,
		"sender@ownership.invalid", "subject", "text body", "<p>html</p>", []string{"123456"}, nil)
	if err != nil {
		t.Fatalf("save message: %v", err)
	}
	return id
}

func createAttachment(t *testing.T, messageID int) int {
	t.Helper()
	path := filepath.Join(t.TempDir(), "attachment.txt")
	if err := os.WriteFile(path, []byte("attachment data"), 0o600); err != nil {
		t.Fatalf("write attachment file: %v", err)
	}
	if err := repository.SaveAttachment(database.Conn(), messageID, "attachment.txt", "text/plain", path); err != nil {
		t.Fatalf("save attachment: %v", err)
	}
	attachments, err := repository.GetAttachmentsByMessageID(database.Conn(), messageID)
	if err != nil || len(attachments) == 0 {
		t.Fatalf("load attachment id: %v (%d rows)", err, len(attachments))
	}
	return attachments[len(attachments)-1].ID
}

func errorBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error body %q: %v", w.Body.String(), err)
	}
	return payload.Error
}

func TestInboxReadEnforcesOwnership(t *testing.T) {
	r := ownershipRouter(t)
	email := uniqueEmail(t, "owned-read")
	createOwnedInbox(t, email, ownerUserID)

	owner := signUserJWT(t, ownerUserID, "owner@ownership.invalid", ownershipRole, time.Hour)
	other := signUserJWT(t, otherUserID, "other@ownership.invalid", ownershipRole, time.Hour)
	admin := signUserJWT(t, adminUserID, "admin@ownership.invalid", "admin", time.Hour)

	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email, owner, ""); w.Code != http.StatusOK {
		t.Fatalf("owner read: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email, other, ""); w.Code != http.StatusForbidden {
		t.Fatalf("foreign user read: got %d, want 403 (%s)", w.Code, w.Body.String())
	} else if msg := errorBody(t, w); msg != "Inbox ini bukan milik akun kamu" {
		t.Fatalf("foreign user error = %q", msg)
	}
	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email, admin, ""); w.Code != http.StatusOK {
		t.Fatalf("admin read: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email, "", ownershipAPIKey()); w.Code != http.StatusOK {
		t.Fatalf("api key read: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

func TestWaitAndDeleteEnforceOwnership(t *testing.T) {
	r := ownershipRouter(t)
	email := uniqueEmail(t, "owned-delete")
	createOwnedInbox(t, email, ownerUserID)

	other := signUserJWT(t, otherUserID, "other@ownership.invalid", ownershipRole, time.Hour)

	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email+"/wait?timeout=1", other, ""); w.Code != http.StatusForbidden {
		t.Fatalf("foreign wait: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	if w := doRequest(r, http.MethodDelete, "/api/inbox/"+email, other, ""); w.Code != http.StatusForbidden {
		t.Fatalf("foreign delete: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	if _, err := repository.GetInboxID(database.Conn(), email); err != nil {
		t.Fatalf("inbox must survive foreign delete: %v", err)
	}
	if w := doRequest(r, http.MethodDelete, "/api/inbox/"+email, "", ownershipAPIKey()); w.Code != http.StatusOK {
		t.Fatalf("api key delete: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

func TestMessageAndAttachmentEnforceInboxOwnership(t *testing.T) {
	r := ownershipRouter(t)
	email := uniqueEmail(t, "owned-message")
	inboxID := createOwnedInbox(t, email, ownerUserID)
	messageID := createMessage(t, inboxID)
	attachmentID := createAttachment(t, messageID)

	owner := signUserJWT(t, ownerUserID, "owner@ownership.invalid", ownershipRole, time.Hour)
	other := signUserJWT(t, otherUserID, "other@ownership.invalid", ownershipRole, time.Hour)

	paths := map[string]string{
		"message":     fmt.Sprintf("/api/message/%d", messageID),
		"attachments": fmt.Sprintf("/api/message/%d/attachments", messageID),
		"attachment":  fmt.Sprintf("/api/attachment/%d", attachmentID),
	}
	for name, path := range paths {
		if w := doRequest(r, http.MethodGet, path, other, ""); w.Code != http.StatusForbidden {
			t.Fatalf("foreign %s: got %d, want 403 (%s)", name, w.Code, w.Body.String())
		}
		if w := doRequest(r, http.MethodGet, path, owner, ""); w.Code != http.StatusOK {
			t.Fatalf("owner %s: got %d, want 200 (%s)", name, w.Code, w.Body.String())
		}
		if w := doRequest(r, http.MethodGet, path, "", ownershipAPIKey()); w.Code != http.StatusOK {
			t.Fatalf("api key %s: got %d, want 200 (%s)", name, w.Code, w.Body.String())
		}
	}
	if w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/message/%d", messageID), other, ""); w.Code != http.StatusForbidden {
		t.Fatalf("foreign message delete: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	if _, err := repository.GetMessageByID(database.Conn(), fmt.Sprintf("%d", messageID)); err != nil {
		t.Fatalf("message must survive foreign delete: %v", err)
	}
	if w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/message/%d", messageID), owner, ""); w.Code != http.StatusOK {
		t.Fatalf("owner message delete: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

func TestMissingMessageStillReturns404NotForbidden(t *testing.T) {
	r := ownershipRouter(t)
	other := signUserJWT(t, otherUserID, "other@ownership.invalid", ownershipRole, time.Hour)

	missing := fmt.Sprintf("/api/message/%d", time.Now().UnixNano()%1000000+1000000)
	if w := doRequest(r, http.MethodGet, missing, other, ""); w.Code != http.StatusNotFound {
		t.Fatalf("missing message: got %d, want 404 (%s)", w.Code, w.Body.String())
	}
	if w := doRequest(r, http.MethodGet, strings.Replace(missing, "/api/message/", "/api/attachment/", 1), other, ""); w.Code != http.StatusNotFound {
		t.Fatalf("missing attachment: got %d, want 404 (%s)", w.Code, w.Body.String())
	}
}

func TestCreateInboxRecordsOwnerFromJWT(t *testing.T) {
	r := ownershipRouter(t)
	owner := signUserJWT(t, ownerUserID, "owner@ownership.invalid", ownershipRole, time.Hour)
	apiKeyEmail := uniqueEmail(t, "apikey-created")

	ownedEmail := uniqueEmail(t, "jwt-created")
	if w := postJSON(r, "/api/inbox", owner, "", fmt.Sprintf(`{"email":%q}`, ownedEmail)); w.Code != http.StatusCreated {
		t.Fatalf("jwt create inbox: got %d, want 201 (%s)", w.Code, w.Body.String())
	}
	ownerID, err := repository.GetInboxOwnerID(database.Conn(), ownedEmail)
	if err != nil {
		t.Fatalf("read owner id: %v", err)
	}
	if !ownerID.Valid || int(ownerID.Int64) != ownerUserID {
		t.Fatalf("owner_id = %v, want %d", ownerID, ownerUserID)
	}

	if w := postJSON(r, "/api/inbox", "", ownershipAPIKey(), fmt.Sprintf(`{"email":%q}`, apiKeyEmail)); w.Code != http.StatusCreated {
		t.Fatalf("api key create inbox: got %d, want 201 (%s)", w.Code, w.Body.String())
	}
	apiOwner, err := repository.GetInboxOwnerID(database.Conn(), apiKeyEmail)
	if err != nil {
		t.Fatalf("read api key owner id: %v", err)
	}
	if apiOwner.Valid {
		t.Fatalf("api key inbox owner_id = %v, want NULL", apiOwner.Int64)
	}
}

func TestOwnerlessInboxIsDeniedToUsers(t *testing.T) {
	r := ownershipRouter(t)
	email := uniqueEmail(t, "ownerless")
	if _, err := repository.CreateInbox(database.Conn(), email); err != nil {
		t.Fatalf("create ownerless inbox: %v", err)
	}

	user := signUserJWT(t, ownerUserID, "owner@ownership.invalid", ownershipRole, time.Hour)
	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email, user, ""); w.Code != http.StatusForbidden {
		t.Fatalf("ownerless read as user: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email, "", ownershipAPIKey()); w.Code != http.StatusOK {
		t.Fatalf("ownerless read with api key: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

func TestPublicAPIRejectsInvalidJWT(t *testing.T) {
	r := ownershipRouter(t)
	email := uniqueEmail(t, "invalid-jwt")
	createOwnedInbox(t, email, ownerUserID)

	cases := map[string]string{
		"garbage":      "not-a-jwt",
		"wrong secret": signWithSecret(t, "not-the-secret", ownerUserID),
		"expired":      signUserJWT(t, ownerUserID, "owner@ownership.invalid", ownershipRole, -time.Hour),
	}
	for name, token := range cases {
		w := doRequest(r, http.MethodGet, "/api/inbox/"+email, token, "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s token: got %d, want 401 (%s)", name, w.Code, w.Body.String())
		}
		if msg := errorBody(t, w); msg != "Invalid token" {
			t.Fatalf("%s token error = %q", name, msg)
		}
	}

	valid := signUserJWT(t, ownerUserID, "owner@ownership.invalid", ownershipRole, time.Hour)
	if w := doRequest(r, http.MethodGet, "/api/inbox/"+email, valid, ""); w.Code != http.StatusOK {
		t.Fatalf("valid token: got %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

func TestSSEDeniesForeignInbox(t *testing.T) {
	r := ownershipRouter(t)
	email := uniqueEmail(t, "sse-owned")
	createOwnedInbox(t, email, ownerUserID)

	other := signUserJWT(t, otherUserID, "other@ownership.invalid", ownershipRole, time.Hour)
	w := doRequest(r, http.MethodGet, "/sse/"+email+"?token="+other, "", "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign SSE viewer: got %d, want 403 (%s)", w.Code, w.Body.String())
	}
	if msg := errorBody(t, w); msg != "Inbox ini bukan milik akun kamu" {
		t.Fatalf("foreign SSE error = %q", msg)
	}
}

func signWithSecret(t *testing.T, secret string, userID int) string {
	t.Helper()
	claims := middleware.JWTClaims{UserID: userID, Email: "owner@ownership.invalid", Role: ownershipRole}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign jwt with custom secret: %v", err)
	}
	return signed
}
