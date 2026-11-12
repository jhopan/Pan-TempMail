package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"receiver/database"
	"receiver/repository"
	"receiver/web"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tempmail-web")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("DB_PATH", filepath.Join(dir, "web.db"))
	os.Setenv("DOMAINS", "fallback.invalid")
	os.Setenv("JWT_SECRET", "web-test-secret")
	if err := database.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = database.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func configDomains(t *testing.T) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	web.ConfigHandler(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status %d", rec.Code)
	}
	var payload struct {
		Domains []string `json:"domains"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	return payload.Domains
}

func TestConfigPrefersRoutedDomainsOverEnvFallback(t *testing.T) {
	db := database.Conn()
	if _, err := db.Exec("DELETE FROM cf_domains"); err != nil {
		t.Fatalf("clean domains: %v", err)
	}
	if got := configDomains(t); !reflect.DeepEqual(got, []string{"fallback.invalid"}) {
		t.Fatalf("without records config should fall back to DOMAINS, got %#v", got)
	}

	routed := repository.CFDomain{Domain: "live.invalid", ProfileID: "cf-a", ZoneID: "zone-1", Enabled: true, Routing: true}
	listed := repository.CFDomain{Domain: "listed-only.invalid", ProfileID: "cf-a", ZoneID: "zone-2", Enabled: true}
	for _, record := range []repository.CFDomain{routed, listed} {
		if err := repository.UpsertCFDomain(db, record); err != nil {
			t.Fatalf("seed domain %s: %v", record.Domain, err)
		}
	}
	if got := configDomains(t); !reflect.DeepEqual(got, []string{"live.invalid"}) {
		t.Fatalf("config should expose only routed domains, got %#v", got)
	}
}
