package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"receiver/api"
	"receiver/database"

	"github.com/gin-gonic/gin"
)

// emptyUsers clears the users table so the first-run flow can be exercised.
// The row count is restored to zero on cleanup, which is the pre-setup state.
func emptyUsers(t *testing.T) {
	t.Helper()
	db := database.Conn()
	if db == nil {
		t.Fatal("no database connection")
	}
	if _, err := db.Exec("DELETE FROM users"); err != nil {
		t.Fatalf("clear users table: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DELETE FROM users"); err != nil {
			t.Errorf("cleanup users table: %v", err)
		}
	})
}

func getSetupStatus(t *testing.T, router *gin.Engine) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/setup", nil))

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return recorder.Code, payload
}

func postSetup(t *testing.T, router *gin.Engine, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		t.Fatalf("encode setup payload: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestSetupStatusReportsFirstRun walks the whole first-run contract on a public
// route: empty database wants setup, the seed turns the flag off, and a second
// seed is still refused.
func TestSetupStatusReportsFirstRun(t *testing.T) {
	t.Setenv("GIN_MODE", gin.TestMode)
	router := api.SetupRouter()
	emptyUsers(t)

	code, payload := getSetupStatus(t, router)
	if code != http.StatusOK {
		t.Fatalf("GET /api/setup status = %d, want %d", code, http.StatusOK)
	}
	if payload["needs_setup"] != true {
		t.Fatalf("needs_setup on empty database = %v, want true", payload["needs_setup"])
	}
	if len(payload) != 1 {
		t.Fatalf("GET /api/setup leaked extra fields: %v", payload)
	}

	created := postSetup(t, router, "first-admin@setup.invalid", "rahasia123")
	if created.Code != http.StatusCreated {
		t.Fatalf("POST /api/setup status = %d (%s), want %d", created.Code, created.Body.String(), http.StatusCreated)
	}

	code, payload = getSetupStatus(t, router)
	if code != http.StatusOK {
		t.Fatalf("GET /api/setup after seed status = %d, want %d", code, http.StatusOK)
	}
	if payload["needs_setup"] != false {
		t.Fatalf("needs_setup after seed = %v, want false", payload["needs_setup"])
	}

	again := postSetup(t, router, "second-admin@setup.invalid", "rahasia123")
	if again.Code != http.StatusConflict {
		t.Fatalf("second POST /api/setup status = %d, want %d", again.Code, http.StatusConflict)
	}
}
