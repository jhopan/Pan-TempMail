package handlers_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"receiver/api"
	"receiver/database"
	"receiver/handlers"
	"receiver/middleware"
	"receiver/repository"
	"receiver/worker"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	testAccountID = "acc-adelian"
	testToken     = "cf-token-secret-adelian"
	testWebHost   = "mail.example.invalid"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tempmail-cf-handlers")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("JWT_SECRET", "handlers-test-secret")
	os.Setenv("EMAIL_API_KEY", "handlers-test-email-key")
	os.Setenv("DB_PATH", filepath.Join(dir, "handlers.db"))
	os.Setenv("DOMAINS", "fallback.invalid")
	if err := database.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = database.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// cfStub is a stand-in for the Cloudflare REST API. It records what the handlers sent so the
// tests can assert request shapes without touching the network.
type cfStub struct {
	mu            sync.Mutex
	zoneStatus    string
	tokenValid    bool
	catchAllBody  map[string]any
	catchAllCalls int
	workerMeta    map[string]any
	workerScript  string
	workerSecret  string
	tunnelName    string
}

func newCFStub() *cfStub {
	return &cfStub{zoneStatus: "active", tokenValid: true}
}

func (s *cfStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	write := func(status int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(payload)
	}
	ok := func(result any) {
		write(http.StatusOK, map[string]any{"success": true, "errors": []any{}, "result": result})
	}
	denied := func(status, code int, message string) {
		write(status, map[string]any{
			"success": false,
			"errors":  []any{map[string]any{"code": code, "message": message}},
			"result":  nil,
		})
	}

	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/accounts/") && strings.Contains(path, "/cfd_tunnel") && strings.HasSuffix(path, "/token"):
		ok("connector-token-value")
	case strings.HasPrefix(path, "/accounts/") && strings.Contains(path, "/cfd_tunnel") && strings.HasSuffix(path, "/connections"):
		ok([]any{map[string]any{"id": "conn-1"}, map[string]any{"id": "conn-2"}})
	case strings.HasPrefix(path, "/accounts/") && strings.Contains(path, "/cfd_tunnel") && r.Method == http.MethodGet:
		if len(s.tunnelName) == 0 {
			ok([]any{})
			return
		}
		ok([]any{map[string]any{
			"id": "tunnel-id-1", "name": s.tunnelName, "status": "healthy",
			"connections": []any{}, "created_on": "2026-01-01T00:00:00Z",
		}})
	case strings.HasPrefix(path, "/accounts/") && strings.Contains(path, "/cfd_tunnel") && r.Method == http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.tunnelName = body.Name
		ok(map[string]any{"id": "tunnel-id-1", "name": body.Name, "status": "healthy"})
	case strings.HasSuffix(path, "/subdomain"):
		ok(map[string]any{"enabled": false})
	case strings.HasPrefix(path, "/accounts/") && strings.Contains(path, "/workers/scripts/") && r.Method == http.MethodDelete:
		ok(map[string]any{})
	case strings.HasPrefix(path, "/accounts/") && strings.Contains(path, "/workers/scripts/") && r.Method == http.MethodPut:
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			denied(http.StatusBadRequest, 10000, "multipart: "+err.Error())
			return
		}
		_ = json.Unmarshal([]byte(r.FormValue("metadata")), &s.workerMeta)
		for _, binding := range s.bindings() {
			if binding["name"] == "TEMPMAIL_API_KEY" {
				s.workerSecret, _ = binding["text"].(string)
			}
		}
		file, _, err := r.FormFile("index.js")
		if err != nil {
			denied(http.StatusBadRequest, 10000, "module part missing")
			return
		}
		raw, _ := io.ReadAll(file)
		s.workerScript = string(raw)
		ok(map[string]any{"id": worker.Name(), "modified_on": "2026-01-01T00:00:00Z"})
	case path == "/accounts/"+testAccountID:
		if !s.tokenValid {
			denied(http.StatusForbidden, 9109, "Invalid access token")
			return
		}
		ok(map[string]any{"id": testAccountID, "name": "Adelian Store"})
	case strings.HasPrefix(path, "/accounts/"):
		if !s.tokenValid {
			denied(http.StatusForbidden, 9109, "Invalid access token")
			return
		}
		denied(http.StatusNotFound, 7003, "account not found")
	case strings.HasSuffix(path, "/email/routing/enable") && r.Method == http.MethodPost:
		ok(map[string]any{"enabled": true})
	case strings.HasSuffix(path, "/email/routing/dns") && r.Method == http.MethodDelete:
		ok(map[string]any{})
	case strings.HasSuffix(path, "/email/routing/rules/catch_all") && r.Method == http.MethodPut:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			denied(http.StatusBadRequest, 10000, "bad body")
			return
		}
		s.catchAllBody = body
		s.catchAllCalls++
		ok(body)
	case strings.HasPrefix(path, "/zones") && r.Method == http.MethodGet:
		if s.zoneStatus == "" {
			ok([]any{})
			return
		}
		ok([]any{map[string]any{"id": "zone-1", "name": "example.invalid", "status": s.zoneStatus}})
	default:
		denied(http.StatusNotFound, 7003, "unhandled stub route "+r.Method+" "+path)
	}
}

func (s *cfStub) bindings() []map[string]any {
	if s.workerMeta == nil {
		return nil
	}
	raw, ok := s.workerMeta["bindings"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if binding, ok := item.(map[string]any); ok {
			out = append(out, binding)
		}
	}
	return out
}

func (s *cfStub) start(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	t.Setenv("CLOUDFLARE_API_BASE", server.URL)
}

func resetCF(t *testing.T) {
	t.Helper()
	db := database.Conn()
	for _, table := range []string{"cf_profiles", "cf_domains"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatalf("reset %s: %v", table, err)
		}
	}
	for _, key := range []string{
		repository.SettingActiveProfile, repository.SettingTunnelWebHost,
		repository.SettingTunnelName, repository.SettingCopies, "cf_tunnel_id",
	} {
		if _, err := db.Exec("DELETE FROM settings WHERE key = ?", key); err != nil {
			t.Fatalf("reset setting %s: %v", key, err)
		}
	}
}

func testRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	admin := r.Group("/api/admin")
	admin.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware())
	{
		admin.GET("/cloudflare/status", handlers.CFStatus)
		admin.GET("/cloudflare/profiles", handlers.CFListProfiles)
		admin.POST("/cloudflare/profiles", handlers.CFCreateProfile)
		admin.PUT("/cloudflare/profiles/:id", handlers.CFUpdateProfile)
		admin.DELETE("/cloudflare/profiles/:id", handlers.CFDeleteProfile)
		admin.POST("/cloudflare/profiles/:id/activate", handlers.CFActivateProfile)
		admin.POST("/cloudflare/profiles/:id/deploy-worker", handlers.CFDeployWorker)
		admin.DELETE("/cloudflare/profiles/:id/worker", handlers.CFDeleteWorker)
		admin.GET("/cloudflare/domains", handlers.CFListDomains)
		admin.POST("/cloudflare/domains", handlers.CFAddDomain)
		admin.DELETE("/cloudflare/domains/:domain", handlers.CFDeleteDomain)
		admin.POST("/cloudflare/domains/:domain/routing", handlers.CFSetDomainRouting)
		admin.GET("/cloudflare/tunnel", handlers.CFTunnelStatus)
		admin.PUT("/cloudflare/tunnel", handlers.CFSetTunnel)
		admin.POST("/cloudflare/tunnel/create", handlers.CFCreateTunnel)
		admin.GET("/cloudflare/copies", handlers.CFGetCopies)
		admin.PUT("/cloudflare/copies", handlers.CFSetCopies)
		admin.POST("/cloudflare/import", handlers.CFImport)
	}
	return r
}

func jwtFor(t *testing.T, role string) string {
	t.Helper()
	claims := &middleware.JWTClaims{UserID: 1, Email: "admin@example.invalid", Role: role}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	return signed
}

func doJSON(t *testing.T, r *gin.Engine, method, path, role, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+jwtFor(t, role))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func createProfile(t *testing.T, r *gin.Engine, id string, activate bool) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"label":"CF Adelian","account_id":%q,"token":%q,"activate":%t}`,
		id, testAccountID, testToken, activate)
	return doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/profiles", "admin", body)
}

func TestAdminCloudflareRoutesRegistered(t *testing.T) {
	os.Setenv("JWT_SECRET", "handlers-test-secret")
	router := api.SetupRouter()
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	expected := []string{
		"GET /api/admin/cloudflare/status",
		"POST /api/admin/cloudflare/profiles",
		"PUT /api/admin/cloudflare/profiles/:id",
		"DELETE /api/admin/cloudflare/profiles/:id",
		"POST /api/admin/cloudflare/profiles/:id/activate",
		"POST /api/admin/cloudflare/profiles/:id/deploy-worker",
		"DELETE /api/admin/cloudflare/profiles/:id/worker",
		"POST /api/admin/cloudflare/domains",
		"DELETE /api/admin/cloudflare/domains/:domain",
		"POST /api/admin/cloudflare/domains/:domain/routing",
		"PUT /api/admin/cloudflare/tunnel",
		"POST /api/admin/cloudflare/tunnel/create",
		"PUT /api/admin/cloudflare/copies",
		"POST /api/admin/cloudflare/import",
	}
	for _, route := range expected {
		if !registered[route] {
			t.Errorf("route not registered: %s", route)
		}
	}
}

func TestAdminPanelShortcutIsReachableAndNotLinked(t *testing.T) {
	router := api.SetupRouter()

	for _, path := range []string{"/admin", "/admin/"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/app/admin.html" {
			t.Fatalf("%s: got %d location %q", path, rec.Code, rec.Header().Get("Location"))
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/app/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inbox page: got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "admin.html") {
		t.Fatal("inbox page still advertises the admin panel")
	}
}

func TestAdminCloudflareRequiresAdminRole(t *testing.T) {
	resetCF(t)
	r := testRouter()
	if code, _ := doJSON(t, r, http.MethodGet, "/api/admin/cloudflare/status", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous request: got %d, want 401", code)
	}
	if code, _ := doJSON(t, r, http.MethodGet, "/api/admin/cloudflare/status", "user", ""); code != http.StatusForbidden {
		t.Fatalf("non-admin request: got %d, want 403", code)
	}
}

func TestCreateProfileValidatesTokenAndHidesSecret(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()

	stub.tokenValid = false
	if code, _ := createProfile(t, r, "cf-bad", false); code != http.StatusBadRequest {
		t.Fatalf("invalid token: got %d, want 400", code)
	}

	stub.tokenValid = true
	if code, body := createProfile(t, r, "bad id!", false); code != http.StatusBadRequest {
		t.Fatalf("invalid id: got %d, want 400 (%s)", code, body)
	}

	// No label in the request: the Cloudflare account name becomes the profile label.
	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/profiles", "admin",
		fmt.Sprintf(`{"id":"cf-adelian","account_id":%q,"token":%q,"activate":true}`, testAccountID, testToken))
	if code != http.StatusCreated {
		t.Fatalf("create profile: got %d, body %s", code, body)
	}
	if strings.Contains(body, testToken) {
		t.Fatalf("response leaked the API token: %s", body)
	}
	var view struct {
		ID        string `json:"id"`
		Label     string `json:"label"`
		AccountID string `json:"account_id"`
		Active    bool   `json:"active"`
		HasToken  bool   `json:"has_token"`
	}
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode profile view: %v", err)
	}
	if view.ID != "cf-adelian" || !view.Active || !view.HasToken || view.AccountID != testAccountID {
		t.Fatalf("unexpected profile view: %s", body)
	}
	if view.Label != "Adelian Store" {
		t.Fatalf("label should fall back to the Cloudflare account name, got %q", view.Label)
	}

	if code, _ := createProfile(t, r, "cf-adelian", false); code != http.StatusConflict {
		t.Fatalf("duplicate profile: got %d, want 409", code)
	}
	if active := repository.GetSetting(database.Conn(), repository.SettingActiveProfile); active != "cf-adelian" {
		t.Fatalf("active profile = %q, want cf-adelian", active)
	}
}

func TestAddDomainUsesZoneLookupAndRejectsInactiveZone(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}

	stub.zoneStatus = "pending"
	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"domain-b.invalid"}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "belum aktif") {
		t.Fatalf("pending zone: got %d %s", code, body)
	}

	stub.zoneStatus = "active"
	code, body = doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"Domain-B.invalid"}`)
	if code != http.StatusCreated {
		t.Fatalf("add domain: got %d %s", code, body)
	}
	var view struct {
		Domain  string `json:"domain"`
		Profile string `json:"profile"`
		ZoneID  string `json:"zone_id"`
		Routing bool   `json:"routing"`
	}
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode domain view: %v", err)
	}
	if view.Domain != "domain-b.invalid" || view.ZoneID != "zone-1" || view.Profile != "cf-adelian" || view.Routing {
		t.Fatalf("unexpected domain view: %s", body)
	}

	if code, _ := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"domain-b.invalid"}`); code != http.StatusConflict {
		t.Fatalf("duplicate domain: got %d, want 409", code)
	}
}

func TestSetDomainRoutingWritesCatchAllAndListsDomain(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}
	if code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"domain-b.invalid"}`); code != http.StatusCreated {
		t.Fatalf("setup domain: %d %s", code, body)
	}

	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains/domain-b.invalid/routing", "admin",
		`{"routing":true,"unlock_dns":true}`)
	if code != http.StatusOK {
		t.Fatalf("enable routing: got %d %s", code, body)
	}
	expected := map[string]any{
		"enabled": true,
		"name":    "TempMail catch-all",
		"matchers": []any{
			map[string]any{"type": "all"},
		},
		"actions": []any{
			map[string]any{"type": "worker", "value": []any{worker.Name()}},
		},
	}
	if !reflect.DeepEqual(stub.catchAllBody, expected) {
		t.Fatalf("catch-all body mismatch:\n got %#v\nwant %#v", stub.catchAllBody, expected)
	}
	var toggled struct {
		Routing     bool `json:"routing"`
		Enabled     bool `json:"enabled"`
		DNSUnlocked bool `json:"dns_unlocked"`
	}
	if err := json.Unmarshal([]byte(body), &toggled); err != nil {
		t.Fatalf("decode routing response: %v", err)
	}
	if !toggled.Routing || !toggled.Enabled || !toggled.DNSUnlocked {
		t.Fatalf("unexpected routing response: %s", body)
	}
	listed, err := repository.ListedDomains(database.Conn())
	if err != nil {
		t.Fatalf("listed domains: %v", err)
	}
	if !reflect.DeepEqual(listed, []string{"domain-b.invalid"}) {
		t.Fatalf("listed domains = %#v, want [domain-b.invalid]", listed)
	}

	code, body = doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains/domain-b.invalid/routing", "admin",
		`{"routing":false}`)
	if code != http.StatusOK {
		t.Fatalf("disable routing: got %d %s", code, body)
	}
	if enabled, _ := stub.catchAllBody["enabled"].(bool); enabled {
		t.Fatalf("disable should send enabled=false, got %#v", stub.catchAllBody)
	}
	listed, _ = repository.ListedDomains(database.Conn())
	if len(listed) != 0 {
		t.Fatalf("disabled domain must not be listed, got %#v", listed)
	}
}

func TestRoutingResolvesZoneForImportedDomain(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}
	if code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"domain-b.invalid"}`); code != http.StatusCreated {
		t.Fatalf("setup domain: %d %s", code, body)
	}
	// A domain imported from the legacy state file has no zone id: routing must look it up once.
	if _, err := database.Conn().Exec("UPDATE cf_domains SET zone_id = '' WHERE domain = 'domain-b.invalid'"); err != nil {
		t.Fatalf("blank zone id: %v", err)
	}
	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains/domain-b.invalid/routing", "admin",
		`{"routing":true}`)
	if code != http.StatusOK {
		t.Fatalf("routing with blank zone id: got %d %s", code, body)
	}
	record, err := repository.GetCFDomain(database.Conn(), "domain-b.invalid")
	if err != nil {
		t.Fatalf("read domain: %v", err)
	}
	if record.ZoneID != "zone-1" || !record.Routing {
		t.Fatalf("zone not resolved or routing not stored: %+v", record)
	}
	if stub.catchAllBody == nil {
		t.Fatal("catch-all was not called")
	}
}

func TestRoutingRejectsDomainWithoutZone(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}
	if code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"domain-b.invalid"}`); code != http.StatusCreated {
		t.Fatalf("setup domain: %d %s", code, body)
	}
	if _, err := database.Conn().Exec("UPDATE cf_domains SET zone_id = '' WHERE domain = 'domain-b.invalid'"); err != nil {
		t.Fatalf("blank zone id: %v", err)
	}
	stub.zoneStatus = "pending"
	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains/domain-b.invalid/routing", "admin",
		`{"routing":true}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "Zone domain-b.invalid") {
		t.Fatalf("unresolvable zone: got %d %s", code, body)
	}
	if stub.catchAllBody != nil {
		t.Fatal("catch-all must not be touched when the zone cannot be resolved")
	}
}

func TestDeleteProfileBlockedWhileDomainsReferenceIt(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}
	if code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"domain-b.invalid"}`); code != http.StatusCreated {
		t.Fatalf("setup domain: %d %s", code, body)
	}
	if code, _ := doJSON(t, r, http.MethodDelete, "/api/admin/cloudflare/profiles/cf-adelian", "admin", ""); code != http.StatusConflict {
		t.Fatalf("delete with domain: got %d, want 409", code)
	}
	if code, body := doJSON(t, r, http.MethodDelete, "/api/admin/cloudflare/domains/domain-b.invalid", "admin", ""); code != http.StatusOK {
		t.Fatalf("delete domain: %d %s", code, body)
	}
	if code, body := doJSON(t, r, http.MethodDelete, "/api/admin/cloudflare/profiles/cf-adelian", "admin", ""); code != http.StatusOK {
		t.Fatalf("delete profile: %d %s", code, body)
	}
	if active := repository.GetSetting(database.Conn(), repository.SettingActiveProfile); active != "" {
		t.Fatalf("active profile should be cleared, got %q", active)
	}
}

func TestDeployWorkerRequiresWebHostAndSendsWorkerScript(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}

	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/profiles/cf-adelian/deploy-worker", "admin", "{}")
	if code != http.StatusBadRequest || !strings.Contains(body, "Web host") {
		t.Fatalf("missing web host: got %d %s", code, body)
	}

	if code, body := doJSON(t, r, http.MethodPut, "/api/admin/cloudflare/tunnel", "admin",
		`{"web_host":"`+testWebHost+`","name":"tempmailbyjhopanstore","id":"tunnel-from-menu"}`); code != http.StatusOK {
		t.Fatalf("set tunnel: %d %s", code, body)
	}
	if stored := repository.GetSetting(database.Conn(), "cf_tunnel_id"); stored != "tunnel-from-menu" {
		t.Fatalf("tunnel id from the menu was not stored, got %q", stored)
	}
	if code, body := doJSON(t, r, http.MethodPut, "/api/admin/cloudflare/copies", "admin",
		`{"destinations":["copy@example.invalid"]}`); code != http.StatusOK {
		t.Fatalf("set copies: %d %s", code, body)
	}

	code, body = doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/profiles/cf-adelian/deploy-worker", "admin", "{}")
	if code != http.StatusOK {
		t.Fatalf("deploy worker: %d %s", code, body)
	}
	if strings.Contains(body, testToken) || strings.Contains(body, os.Getenv("EMAIL_API_KEY")) {
		t.Fatalf("deploy response leaked a secret: %s", body)
	}
	if stub.workerScript != worker.Script() {
		t.Fatalf("uploaded script differs from embedded worker script (%d vs %d bytes)",
			len(stub.workerScript), len(worker.Script()))
	}
	if got, _ := stub.workerMeta["main_module"].(string); got != "index.js" {
		t.Fatalf("main_module = %q, want index.js", got)
	}
	if got, _ := stub.workerMeta["compatibility_date"].(string); got != worker.CompatDate() {
		t.Fatalf("compatibility_date = %q, want %q", got, worker.CompatDate())
	}
	var forward string
	for _, binding := range stub.bindings() {
		if binding["name"] == "FORWARD_LIST" {
			forward, _ = binding["text"].(string)
		}
		if server, _ := binding["text"].(string); binding["name"] == "TEMPMAIL_SERVER_URL" && server != "https://"+testWebHost {
			t.Fatalf("TEMPMAIL_SERVER_URL = %q", server)
		}
	}
	if forward != "copy@example.invalid" {
		t.Fatalf("FORWARD_LIST = %q", forward)
	}
	if stub.workerSecret != os.Getenv("EMAIL_API_KEY") {
		t.Fatalf("worker did not receive EMAIL_API_KEY value")
	}
	profile, err := repository.GetCFProfile(database.Conn(), "cf-adelian")
	if err != nil || !profile.WorkerDeployed {
		t.Fatalf("profile worker_deployed not recorded: %+v (%v)", profile, err)
	}
}

func TestCopiesValidationAndStatusPayload(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}
	if code, _ := doJSON(t, r, http.MethodPut, "/api/admin/cloudflare/copies", "admin",
		`{"destinations":["not-an-address"]}`); code != http.StatusBadRequest {
		t.Fatalf("invalid destination must be rejected")
	}

	code, body := doJSON(t, r, http.MethodGet, "/api/admin/cloudflare/status", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("status: %d %s", code, body)
	}
	if strings.Contains(body, testToken) {
		t.Fatalf("status leaked the API token: %s", body)
	}
	var status struct {
		ActiveProfile string `json:"active_profile"`
		Profiles      []struct {
			ID       string `json:"id"`
			HasToken bool   `json:"has_token"`
		} `json:"profiles"`
		Tunnel struct {
			Name  string `json:"name"`
			Error string `json:"error"`
		} `json:"tunnel"`
		WorkerName string `json:"worker_name"`
		APIBase    string `json:"api_base"`
	}
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.ActiveProfile != "cf-adelian" || len(status.Profiles) != 1 || !status.Profiles[0].HasToken {
		t.Fatalf("unexpected status: %s", body)
	}
	if status.WorkerName != worker.Name() || status.APIBase == "" {
		t.Fatalf("status missing worker/api info: %s", body)
	}
}

func TestTunnelCreateReturnsConnectorTokenAndCommands(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()
	if code, body := createProfile(t, r, "cf-adelian", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}

	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/tunnel/create", "admin",
		`{"name":"tempmailbyjhopanstore","web_host":"`+testWebHost+`"}`)
	if code != http.StatusOK {
		t.Fatalf("create tunnel: %d %s", code, body)
	}
	var created struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Token    string   `json:"token"`
		Commands []string `json:"commands"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode tunnel: %v", err)
	}
	if created.ID != "tunnel-id-1" || created.Token != "connector-token-value" {
		t.Fatalf("unexpected tunnel: %s", body)
	}
	joined := strings.Join(created.Commands, "\n")
	if !strings.Contains(joined, "cloudflared service install") || !strings.Contains(joined, "cfargotunnel.com") {
		t.Fatalf("install commands incomplete: %s", joined)
	}

	code, body = doJSON(t, r, http.MethodGet, "/api/admin/cloudflare/tunnel", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("tunnel status: %d %s", code, body)
	}
	var view struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Connections int    `json:"connections"`
		WebHost     string `json:"web_host"`
		Source      string `json:"source"`
	}
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode tunnel view: %v", err)
	}
	if view.ID != "tunnel-id-1" || view.Status != "healthy" || view.Connections != 2 || view.WebHost != testWebHost || view.Source != "api" {
		t.Fatalf("unexpected tunnel view: %s", body)
	}
}

func TestImportStateFileLoadsProfilesDomainsAndTunnel(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()

	state := map[string]any{
		"state": map[string]any{
			"active_profile": "wrangler-oauth",
			"profiles": map[string]any{
				"wrangler-oauth": map[string]any{
					"label": "CF A", "account_id": "acc-a", "token": "legacy-token", "worker_deployed": true,
				},
			},
			"domains": map[string]any{
				"Domain-A.invalid": map[string]any{"profile": "wrangler-oauth", "zone_id": "zone-a", "enabled": true, "routing": true},
			},
			"tunnel":            map[string]any{"id": "tunnel-a", "name": "tempmailbyjhopanstore", "web_host": "mail.example.invalid"},
			"copy_destinations": "copy@example.invalid",
		},
	}
	payload, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/import", "admin", string(payload))
	if code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if strings.Contains(body, "legacy-token") {
		t.Fatalf("import response leaked the imported token: %s", body)
	}
	var result struct {
		Imported struct {
			Profiles      int    `json:"profiles"`
			Domains       int    `json:"domains"`
			Tunnel        bool   `json:"tunnel"`
			ActiveProfile string `json:"active_profile"`
		} `json:"imported"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if result.Imported.Profiles != 1 || result.Imported.Domains != 1 || !result.Imported.Tunnel || result.Imported.ActiveProfile != "wrangler-oauth" {
		t.Fatalf("unexpected import summary: %s", body)
	}

	listed, err := repository.ListedDomains(database.Conn())
	if err != nil {
		t.Fatalf("listed domains: %v", err)
	}
	if !reflect.DeepEqual(listed, []string{"domain-a.invalid"}) {
		t.Fatalf("listed domains = %#v", listed)
	}
	sealed, err := repository.GetCFProfileToken(database.Conn(), "wrangler-oauth")
	if err != nil {
		t.Fatalf("stored token: %v", err)
	}
	if sealed == "legacy-token" || !strings.HasPrefix(sealed, "v1:") {
		t.Fatalf("imported token must be sealed at rest, got %q", sealed)
	}

	// Re-running the import must not duplicate rows or fail.
	if code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/import", "admin", string(payload)); code != http.StatusOK {
		t.Fatalf("second import: %d %s", code, body)
	}
	profiles, err := repository.ListCFProfiles(database.Conn())
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("import is not idempotent, %d profiles", len(profiles))
	}
}

func TestCopyDestinationsRoundTrip(t *testing.T) {
	resetCF(t)
	stub := newCFStub()
	stub.start(t)
	r := testRouter()

	code, body := doJSON(t, r, http.MethodPut, "/api/admin/cloudflare/copies", "admin",
		`{"destinations":["a@example.invalid"," b@example.invalid ",""]}`)
	if code != http.StatusOK {
		t.Fatalf("set copies: %d %s", code, body)
	}
	code, body = doJSON(t, r, http.MethodGet, "/api/admin/cloudflare/copies", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("get copies: %d %s", code, body)
	}
	var got struct {
		Destinations []string `json:"destinations"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode copies: %v", err)
	}
	if !reflect.DeepEqual(got.Destinations, []string{"a@example.invalid", "b@example.invalid"}) {
		t.Fatalf("unexpected copies: %s", body)
	}
}
