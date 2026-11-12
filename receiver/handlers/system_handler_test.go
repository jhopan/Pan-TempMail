package handlers_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"receiver/api"
	"receiver/database"
	"receiver/handlers"
	"receiver/middleware"
	"receiver/worker"

	"github.com/gin-gonic/gin"
)

// systemRouter mirrors the admin group in api.SetupRouter so the handlers see the same JWT claims.
func systemRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	admin := r.Group("/api/admin")
	admin.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware())
	{
		admin.GET("/system/status", handlers.SystemStatus)
		admin.POST("/system/action", handlers.SystemAction)
	}
	return r
}

// helperDirFor creates a temp helper dir with the in/ and out/ subdirectories the contract defines.
func helperDirFor(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"in", "out"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o750); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	t.Setenv("HELPER_DIR", dir)
	return dir
}

func helperPending(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "in", "*.json"))
	if err != nil {
		t.Fatalf("glob in/: %v", err)
	}
	return files
}

type systemStatus struct {
	Helper struct {
		Installed bool   `json:"installed"`
		Dir       string `json:"dir"`
		Pending   int    `json:"pending"`
	} `json:"helper"`
	DB struct {
		Path        string `json:"path"`
		Bytes       int64  `json:"bytes"`
		Inboxes     int    `json:"inboxes"`
		Messages    int    `json:"messages"`
		Attachments int    `json:"attachments"`
	} `json:"db"`
	App struct {
		UptimeSeconds int64  `json:"uptime_seconds"`
		RSSBytes      uint64 `json:"rss_bytes"`
		GoVersion     string `json:"go_version"`
		SiteName      string `json:"site_name"`
		Port          string `json:"port"`
	} `json:"app"`
	Cloudflare struct {
		Profiles      int    `json:"profiles"`
		Domains       int    `json:"domains"`
		ActiveProfile string `json:"active_profile"`
		WorkerName    string `json:"worker_name"`
	} `json:"cloudflare"`
}

func seedMail(t *testing.T) {
	t.Helper()
	db := database.Conn()
	for _, table := range []string{"attachments", "messages", "inboxes"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatalf("reset %s: %v", table, err)
		}
	}
	res, err := db.Exec("INSERT INTO inboxes (email) VALUES (?)", "seed@helper.invalid")
	if err != nil {
		t.Fatalf("insert inbox: %v", err)
	}
	inboxID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("inbox id: %v", err)
	}
	for range 2 {
		if _, err := db.Exec("INSERT INTO messages (inbox_id, sender, subject) VALUES (?, ?, ?)",
			inboxID, "sender@helper.invalid", "seeded"); err != nil {
			t.Fatalf("insert message: %v", err)
		}
	}
}

func TestSystemStatus(t *testing.T) {
	dir := helperDirFor(t)
	for i, name := range []string{"a1b2c3d4e5f60718.json", "0011223344556677.json"} {
		if err := os.WriteFile(filepath.Join(dir, "in", name), []byte("{}"), 0o640); err != nil {
			t.Fatalf("seed pending %d: %v", i, err)
		}
	}
	// Only *.json requests count as pending; a stray file or a half-written temp must not.
	for _, name := range []string{"notes.txt", ".a1b2c3d4e5f60718.tmp"} {
		if err := os.WriteFile(filepath.Join(dir, "in", name), []byte("x"), 0o640); err != nil {
			t.Fatalf("seed stray %s: %v", name, err)
		}
	}
	t.Setenv("SITE_NAME", "TempMail")
	t.Setenv("PORT", "8081")
	t.Setenv("CF_WORKER_NAME", "")
	seedMail(t)

	code, body := doJSON(t, systemRouter(), http.MethodGet, "/api/admin/system/status", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", code, body)
	}
	var got systemStatus
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode status: %v (%s)", err, body)
	}
	if !got.Helper.Installed || got.Helper.Dir != dir || got.Helper.Pending != 2 {
		t.Errorf("helper = %+v, want installed dir=%q pending=2", got.Helper, dir)
	}
	if got.DB.Path != os.Getenv("DB_PATH") {
		t.Errorf("db.path = %q, want %q", got.DB.Path, os.Getenv("DB_PATH"))
	}
	if got.DB.Inboxes != 1 || got.DB.Messages != 2 || got.DB.Attachments != 0 {
		t.Errorf("db counts = %+v, want 1 inbox, 2 messages, 0 attachments", got.DB)
	}
	if got.DB.Bytes <= 0 {
		t.Errorf("db.bytes = %d, want > 0", got.DB.Bytes)
	}
	if got.App.GoVersion == "" || got.App.SiteName != "TempMail" || got.App.Port != "8081" {
		t.Errorf("app = %+v", got.App)
	}
	if got.App.UptimeSeconds < 0 {
		t.Errorf("app.uptime_seconds = %d, want >= 0", got.App.UptimeSeconds)
	}
	if got.Cloudflare.WorkerName != worker.Name() {
		t.Errorf("cloudflare.worker_name = %q, want %q", got.Cloudflare.WorkerName, worker.Name())
	}
	if got.Cloudflare.Profiles != 0 || got.Cloudflare.Domains != 0 {
		t.Errorf("cloudflare counts = %+v, want 0 before any profile is created", got.Cloudflare)
	}
	if strings.Contains(body, "token") {
		t.Errorf("status body leaks a token field: %s", body)
	}
}

func TestSystemStatusWithoutHelperDir(t *testing.T) {
	t.Setenv("HELPER_DIR", filepath.Join(t.TempDir(), "absent"))

	code, body := doJSON(t, systemRouter(), http.MethodGet, "/api/admin/system/status", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", code, body)
	}
	var got systemStatus
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode status: %v (%s)", err, body)
	}
	if got.Helper.Installed || got.Helper.Pending != 0 {
		t.Errorf("helper = %+v, want installed=false pending=0", got.Helper)
	}
}

func TestSystemActionRejectsBadRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"unknown action", `{"action":"nuke-everything"}`},
		{"logs without lines", `{"action":"logs","lines":0}`},
		{"logs too many lines", `{"action":"logs","lines":501}`},
		{"tunnel-install short token", `{"action":"tunnel-install","token":"short"}`},
		{"tunnel-install bad chars", `{"action":"tunnel-install","token":"aaaaaaaaaaaaaaaaaaaa$aaaaaaaa"}`},
		{"tunnel-install long token", `{"action":"tunnel-install","token":"` + strings.Repeat("a", 4097) + `"}`},
		{"tunnel-logout without confirm", `{"action":"tunnel-logout","confirm":"reset"}`},
		{"broken json", `{"action":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := helperDirFor(t)
			code, body := doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin", tc.body)
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", code, body)
			}
			var errBody map[string]any
			if err := json.Unmarshal([]byte(body), &errBody); err != nil {
				t.Fatalf("decode error body: %v (%s)", err, body)
			}
			if msg, _ := errBody["error"].(string); msg == "" {
				t.Errorf("error body has no message: %s", body)
			}
			if pending := helperPending(t, dir); len(pending) != 0 {
				t.Errorf("invalid action wrote a request file: %v", pending)
			}
		})
	}
}

func TestSystemActionWaitsForHelperResult(t *testing.T) {
	dir := helperDirFor(t)
	t.Setenv("HELPER_WAIT_SECONDS", "5")

	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			files, _ := filepath.Glob(filepath.Join(dir, "in", "*.json"))
			if len(files) == 0 {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			raw, err := os.ReadFile(files[0])
			if err != nil {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			var req struct {
				ID          string `json:"id"`
				Action      string `json:"action"`
				Lines       int    `json:"lines"`
				RequestedBy string `json:"requested_by"`
			}
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Errorf("helper stub read invalid request: %v", err)
				return
			}
			if req.Action != "logs" || req.Lines != 100 || req.RequestedBy != "admin@example.invalid" {
				t.Errorf("helper stub got unexpected request: %s", raw)
				return
			}
			result, _ := json.Marshal(map[string]any{
				"id": req.ID, "action": req.Action, "ok": true, "status": "completed",
				"output": "ok", "extra_field": "kept",
			})
			if err := os.WriteFile(filepath.Join(dir, "out", req.ID+".json"), result, 0o640); err != nil {
				t.Errorf("helper stub write result: %v", err)
				return
			}
			// The real helper consumes the request file after processing it.
			if err := os.Remove(files[0]); err != nil {
				t.Errorf("helper stub remove request: %v", err)
			}
			return
		}
		t.Error("helper stub never saw a request file")
	}()

	code, body := doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin",
		`{"action":"logs","lines":100}`)
	<-done
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", code, body)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("decode result: %v (%s)", err, body)
	}
	if result["output"] != "ok" || result["status"] != "completed" || result["extra_field"] != "kept" {
		t.Errorf("result = %v, want the helper answer verbatim", result)
	}
	if pending := helperPending(t, dir); len(pending) != 0 {
		t.Errorf("request file not removed: %v", pending)
	}
	out, err := filepath.Glob(filepath.Join(dir, "out", "*.json"))
	if err != nil {
		t.Fatalf("glob out/: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("result file not removed: %v", out)
	}
}

func TestSystemActionTimesOutWithoutHelper(t *testing.T) {
	dir := helperDirFor(t)
	t.Setenv("HELPER_WAIT_SECONDS", "1")

	code, body := doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin",
		`{"action":"doctor"}`)
	if code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 (%s)", code, body)
	}
	var errBody struct {
		Error string `json:"error"`
		ID    string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &errBody); err != nil {
		t.Fatalf("decode timeout body: %v (%s)", err, body)
	}
	if errBody.Error != "helper tidak menjawab" || errBody.ID == "" {
		t.Errorf("timeout body = %s", body)
	}
	pending := helperPending(t, dir)
	if len(pending) != 1 {
		t.Fatalf("request file must stay for the helper, got %v", pending)
	}
	if filepath.Base(pending[0]) != errBody.ID+".json" {
		t.Errorf("request file %q does not match id %q", pending[0], errBody.ID)
	}
	raw, err := os.ReadFile(pending[0])
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	var req struct {
		ID          string `json:"id"`
		Action      string `json:"action"`
		Lines       int    `json:"lines"`
		Token       string `json:"token"`
		Confirm     string `json:"confirm"`
		Delay       int    `json:"delay"`
		RequestedBy string `json:"requested_by"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	base := strings.TrimSuffix(filepath.Base(pending[0]), ".json")
	if len(base) != 16 || req.ID != base {
		t.Errorf("request id = %q, want 16 hex chars matching the file name %q", req.ID, base)
	}
	if req.Action != "doctor" || req.Lines != 0 || req.Token != "" || req.Confirm != "" {
		t.Errorf("request payload = %+v, want doctor with empty params", req)
	}
}

func TestSystemActionFireAndForget(t *testing.T) {
	dir := helperDirFor(t)

	code, body := doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin",
		`{"action":"restart-service"}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", code, body)
	}
	var accepted struct {
		ID     string `json:"id"`
		Action string `json:"action"`
		Status string `json:"status"`
		OK     bool   `json:"ok"`
	}
	if err := json.Unmarshal([]byte(body), &accepted); err != nil {
		t.Fatalf("decode accepted: %v (%s)", err, body)
	}
	if accepted.Status != "accepted" || !accepted.OK || accepted.Action != "restart-service" {
		t.Errorf("accepted body = %s", body)
	}
	pending := helperPending(t, dir)
	if len(pending) != 1 {
		t.Fatalf("restart-service must leave its request in in/, got %v", pending)
	}
	raw, err := os.ReadFile(pending[0])
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	var req struct {
		ID          string `json:"id"`
		Action      string `json:"action"`
		Delay       int    `json:"delay"`
		RequestedBy string `json:"requested_by"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if req.Delay != 2 {
		t.Errorf("restart-service delay = %d, want the default 2", req.Delay)
	}
	if req.RequestedBy != "admin@example.invalid" {
		t.Errorf("requested_by = %q, want the JWT email", req.RequestedBy)
	}

	code, body = doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin",
		`{"action":"tunnel-logout","confirm":"RESET","delay":99}`)
	if code != http.StatusOK || !strings.Contains(body, `"status":"accepted"`) {
		t.Fatalf("tunnel-logout = %d %s, want 200 accepted", code, body)
	}
	if pending := helperPending(t, dir); len(pending) != 2 {
		t.Errorf("tunnel-logout request missing, in/ holds %v", pending)
	}
}

func TestSystemActionCreatesHelperSubdirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HELPER_DIR", dir)
	t.Setenv("HELPER_WAIT_SECONDS", "1")

	code, body := doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin",
		`{"action":"backup-now"}`)
	if code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504 (%s)", code, body)
	}
	for _, sub := range []string{"in", "out"} {
		info, err := os.Stat(filepath.Join(dir, sub))
		if err != nil || !info.IsDir() {
			t.Fatalf("subdir %s was not created: %v", sub, err)
		}
	}
	if pending := helperPending(t, dir); len(pending) != 1 {
		t.Errorf("request file missing, in/ holds %v", pending)
	}
}

func TestSystemActionMissingHelperDir(t *testing.T) {
	t.Setenv("HELPER_DIR", filepath.Join(t.TempDir(), "absent"))

	code, body := doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin",
		`{"action":"doctor"}`)
	if code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 (%s)", code, body)
	}
	if !strings.Contains(body, "root helper belum dipasang di server ini") {
		t.Errorf("body = %s", body)
	}
}

func TestSystemActionAcceptsValidTunnelToken(t *testing.T) {
	dir := helperDirFor(t)
	t.Setenv("HELPER_WAIT_SECONDS", "1")
	token := strings.Repeat("a", 18) + "._~-"

	code, body := doJSON(t, systemRouter(), http.MethodPost, "/api/admin/system/action", "admin",
		`{"action":"tunnel-install","token":"`+token+`"}`)
	if code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504: a token inside the contract regex must reach the helper (%s)", code, body)
	}
	pending := helperPending(t, dir)
	if len(pending) != 1 {
		t.Fatalf("valid tunnel-install must leave a request file, in/ holds %v", pending)
	}
	raw, err := os.ReadFile(pending[0])
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	var req struct {
		Action string `json:"action"`
		Token  string `json:"token"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if req.Action != "tunnel-install" || req.Token != token {
		t.Errorf("request = %s, want the token verbatim", raw)
	}
}

func TestAdminSystemRoutesRegistered(t *testing.T) {
	os.Setenv("JWT_SECRET", "handlers-test-secret")
	router := api.SetupRouter()
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/admin/system/status",
		"POST /api/admin/system/action",
	} {
		if !registered[route] {
			t.Errorf("route not registered: %s", route)
		}
	}
}
