package handlers_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"receiver/handlers"

	"github.com/gin-gonic/gin"
)

func workerRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/email/receive", handlers.ReceiveEmailViaHTTP)
	return router
}

func workerPayload(to string, rawSize int) []byte {
	raw := "From: sender@example.invalid\r\nTo: " + to + "\r\nSubject: uji\r\n\r\n" + strings.Repeat("x", rawSize)
	body, _ := json.Marshal(map[string]string{
		"from": "sender@example.invalid",
		"to":   to,
		"raw":  raw,
	})
	return body
}

func postWorker(t *testing.T, router *gin.Engine, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/email/receive", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWorkerSignatureEnforcedWhenSecretConfigured(t *testing.T) {
	t.Setenv("EMAIL_API_KEY", "worker-key")
	t.Setenv("WORKER_SIGNING_SECRET", "sign-me")
	router := workerRouter()
	body := workerPayload("sig-1@example.invalid", 32)
	apiKey := map[string]string{"X-Email-API-Key": "worker-key"}

	if code := postWorker(t, router, body, apiKey).Code; code != http.StatusUnauthorized {
		t.Fatalf("tanpa signature: dapat %d, ingin 401", code)
	}

	bad := map[string]string{"X-Email-API-Key": "worker-key", "X-Email-Signature": sign("secret-lain", body)}
	if code := postWorker(t, router, body, bad).Code; code != http.StatusUnauthorized {
		t.Fatalf("signature salah: dapat %d, ingin 401", code)
	}

	good := map[string]string{"X-Email-API-Key": "worker-key", "X-Email-Signature": sign("sign-me", body)}
	recorder := postWorker(t, router, body, good)
	if recorder.Code >= 300 {
		t.Fatalf("signature benar: dapat %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestWorkerSignatureOptionalWhenSecretAbsent(t *testing.T) {
	t.Setenv("EMAIL_API_KEY", "worker-key")
	t.Setenv("WORKER_SIGNING_SECRET", "")
	router := workerRouter()
	body := workerPayload("sig-2@example.invalid", 16)

	recorder := postWorker(t, router, body, map[string]string{"X-Email-API-Key": "worker-key"})
	if recorder.Code >= 300 {
		t.Fatalf("tanpa secret harus tetap diterima, dapat %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "sig-2@example.invalid") {
		t.Fatalf("balasan tidak memuat penerima: %s", recorder.Body.String())
	}
}

func TestWorkerRejectsOversizedEmail(t *testing.T) {
	t.Setenv("EMAIL_API_KEY", "worker-key")
	t.Setenv("WORKER_SIGNING_SECRET", "")
	t.Setenv("MAX_RAW_EMAIL_BYTES", "4096")
	router := workerRouter()

	body := workerPayload("big@example.invalid", 8192)
	recorder := postWorker(t, router, body, map[string]string{"X-Email-API-Key": "worker-key"})
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("raw terlalu besar: dapat %d, ingin 413 (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "limit_bytes") {
		t.Fatalf("balasan tidak menyebut batas: %s", recorder.Body.String())
	}
}

func TestWorkerRejectsBodyLargerThanLimit(t *testing.T) {
	t.Setenv("EMAIL_API_KEY", "worker-key")
	t.Setenv("WORKER_SIGNING_SECRET", "")
	t.Setenv("MAX_RAW_EMAIL_BYTES", "1024")
	router := workerRouter()

	huge := []byte(fmt.Sprintf(`{"from":"a@b.c","to":"a@b.c","raw":%q}`, strings.Repeat("y", 4096*4)))
	recorder := postWorker(t, router, huge, map[string]string{"X-Email-API-Key": "worker-key"})
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body melebihi batas: dapat %d, ingin 413 (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestWorkerRequiresApiKey(t *testing.T) {
	t.Setenv("EMAIL_API_KEY", "worker-key")
	t.Setenv("WORKER_SIGNING_SECRET", "")
	router := workerRouter()

	recorder := postWorker(t, router, workerPayload("no-key@example.invalid", 8), nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("tanpa API key: dapat %d, ingin 401", recorder.Code)
	}
}
