package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
)

// checkStub is a Cloudflare API stand-in for the domain check tests. Unlike cfStub it serves the
// DNS record, Email Routing DNS and catch-all endpoints the check endpoint reads, and it counts
// every mutating call so a check can be proven read-only.
type checkStub struct {
	mu              sync.Mutex
	zones           []map[string]any
	dnsRecords      []map[string]any
	catchAllEnabled bool
	catchAllTarget  string
	workerExists    bool
	mutations       int
}

func newCheckStub() *checkStub {
	return &checkStub{
		zones: []map[string]any{
			{"id": "zone-1", "name": "example.invalid", "status": "active"},
			{"id": "zone-2", "name": "domain-a.invalid", "status": "active"},
			{"id": "zone-3", "name": "other.invalid", "status": "active"},
		},
		dnsRecords: []map[string]any{
			{"id": "rec-web", "type": "CNAME", "name": testWebHost, "content": "tunnel-id-1.cfargotunnel.com", "proxied": true, "ttl": 1},
			{"id": "rec-mx", "type": "MX", "name": "domain-a.invalid", "content": "route1.mx.cloudflare.net", "proxied": false, "ttl": 300, "priority": 10},
			{"id": "rec-txt", "type": "TXT", "name": "domain-a.invalid", "content": "v=spf1 include:_spf.mx.cloudflare.net ~all", "proxied": false, "ttl": 300},
		},
		catchAllEnabled: true,
		catchAllTarget:  worker.Name(),
		workerExists:    true,
	}
}

func (s *checkStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	case strings.HasPrefix(path, "/zones/"):
		rest := strings.TrimPrefix(path, "/zones/")
		zoneID, tail := rest, ""
		if idx := strings.Index(rest, "/"); idx >= 0 {
			zoneID, tail = rest[:idx], rest[idx+1:]
		}
		switch {
		case tail == "email/routing/dns" && r.Method == http.MethodGet:
			ok(s.requiredRecords(zoneID))
		case tail == "email/routing/enable" && r.Method == http.MethodPost:
			s.mutations++
			ok(map[string]any{"enabled": true})
		case tail == "email/routing/rules/catch_all" && r.Method == http.MethodGet:
			ok(map[string]any{"enabled": s.catchAllEnabled, "actions": s.catchAllActions()})
		case tail == "email/routing/rules/catch_all" && r.Method == http.MethodPut:
			var body struct {
				Enabled bool `json:"enabled"`
				Actions []struct {
					Type  string   `json:"type"`
					Value []string `json:"value"`
				} `json:"actions"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				denied(http.StatusBadRequest, 10000, "bad catch-all body: "+err.Error())
				return
			}
			s.mutations++
			s.catchAllEnabled = body.Enabled
			s.catchAllTarget = ""
			for _, action := range body.Actions {
				if action.Type == "worker" && len(action.Value) > 0 {
					s.catchAllTarget = action.Value[0]
				}
			}
			ok(map[string]any{"enabled": s.catchAllEnabled})
		case tail == "dns_records" && r.Method == http.MethodGet:
			ok(s.filtered(r.URL.Query().Get("type"), r.URL.Query().Get("name")))
		case tail == "dns_records" && r.Method == http.MethodPost:
			var record map[string]any
			if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
				denied(http.StatusBadRequest, 10000, "bad dns record body: "+err.Error())
				return
			}
			s.mutations++
			record["id"] = fmt.Sprintf("created-%d", len(s.dnsRecords)+1)
			s.dnsRecords = append(s.dnsRecords, record)
			ok(record)
		case strings.HasPrefix(tail, "dns_records/") && r.Method == http.MethodDelete:
			id := strings.TrimPrefix(tail, "dns_records/")
			kept := make([]map[string]any, 0, len(s.dnsRecords))
			for _, record := range s.dnsRecords {
				if record["id"] == id {
					continue
				}
				kept = append(kept, record)
			}
			s.dnsRecords = kept
			s.mutations++
			ok(map[string]any{"id": id})
		default:
			denied(http.StatusNotFound, 7003, "unhandled zone route "+r.Method+" "+path)
		}
	case path == "/zones" && r.Method == http.MethodGet:
		if name := r.URL.Query().Get("name"); name != "" {
			ok(s.zonesNamed(name))
			return
		}
		ok(s.zones)
	case strings.HasPrefix(path, "/accounts/"):
		switch {
		case strings.HasSuffix(path, "/subdomain") && r.Method == http.MethodPost:
			s.mutations++
			ok(map[string]any{"enabled": false})
		case strings.Contains(path, "/workers/scripts/") && r.Method == http.MethodPut:
			s.mutations++
			s.workerExists = true
			ok(map[string]any{"id": worker.Name(), "modified_on": "2026-01-01T00:00:00Z"})
		case strings.Contains(path, "/workers/scripts/") && r.Method == http.MethodGet:
			if !s.workerExists {
				denied(http.StatusNotFound, 10007, "workers.api.error.script_not_found")
				break
			}
			ok(map[string]any{"id": worker.Name(), "modified_on": "2026-01-01T00:00:00Z"})
		case strings.Contains(path, "/cfd_tunnel") && r.Method == http.MethodGet:
			ok([]any{})
		case r.Method == http.MethodGet:
			ok(map[string]any{"id": testAccountID, "name": "Adelian Store"})
		default:
			denied(http.StatusNotFound, 7003, "unhandled account route "+r.Method+" "+path)
		}
	default:
		denied(http.StatusNotFound, 7003, "unhandled stub route "+r.Method+" "+path)
	}
}

func (s *checkStub) zoneName(zoneID string) string {
	for _, zone := range s.zones {
		if zone["id"] == zoneID {
			name, _ := zone["name"].(string)
			return name
		}
	}
	return ""
}

func (s *checkStub) zonesNamed(name string) []map[string]any {
	out := []map[string]any{}
	for _, zone := range s.zones {
		if zone["name"] == name {
			out = append(out, zone)
		}
	}
	return out
}

// requiredRecords mirrors what Cloudflare asks a zone to publish for Email Routing.
func (s *checkStub) requiredRecords(zoneID string) []map[string]any {
	name := s.zoneName(zoneID)
	if name == "" {
		return []map[string]any{}
	}
	return []map[string]any{
		{"id": "req-mx", "type": "MX", "name": name, "content": "route1.mx.cloudflare.net", "proxied": false, "ttl": 300, "priority": 10},
		{"id": "req-txt", "type": "TXT", "name": name, "content": "v=spf1 include:_spf.mx.cloudflare.net ~all", "proxied": false, "ttl": 300},
	}
}

func (s *checkStub) catchAllActions() []map[string]any {
	if s.catchAllTarget == "" {
		return []map[string]any{{"type": "drop", "value": []string{}}}
	}
	return []map[string]any{{"type": "worker", "value": []string{s.catchAllTarget}}}
}

func (s *checkStub) filtered(recordType, name string) []map[string]any {
	out := []map[string]any{}
	for _, record := range s.dnsRecords {
		if recordType != "" {
			have, _ := record["type"].(string)
			if !strings.EqualFold(have, recordType) {
				continue
			}
		}
		if name != "" {
			have, _ := record["name"].(string)
			if !strings.EqualFold(strings.TrimSuffix(have, "."), name) {
				continue
			}
		}
		out = append(out, record)
	}
	return out
}

func (s *checkStub) start(t *testing.T) {
	t.Helper()
	server := httptest.NewServer(s)
	t.Cleanup(server.Close)
	t.Setenv("CLOUDFLARE_API_BASE", server.URL)
}

func (s *checkStub) resetMutations() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutations = 0
}

func (s *checkStub) mutationCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutations
}

func (s *checkStub) addRecord(recordType, name, content string, proxied bool, priority *int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := map[string]any{
		"id":      fmt.Sprintf("extra-%d", len(s.dnsRecords)+1),
		"type":    recordType,
		"name":    name,
		"content": content,
		"proxied": proxied,
		"ttl":     1,
	}
	if priority != nil {
		record["priority"] = *priority
	}
	s.dnsRecords = append(s.dnsRecords, record)
}

func (s *checkStub) removeRecords(recordType, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := make([]map[string]any, 0, len(s.dnsRecords))
	for _, record := range s.dnsRecords {
		haveType, _ := record["type"].(string)
		haveName, _ := record["name"].(string)
		if strings.EqualFold(haveType, recordType) && strings.EqualFold(haveName, name) {
			continue
		}
		kept = append(kept, record)
	}
	s.dnsRecords = kept
}

func (s *checkStub) contentsFor(recordType, name string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{}
	for _, record := range s.dnsRecords {
		haveType, _ := record["type"].(string)
		haveName, _ := record["name"].(string)
		if strings.EqualFold(haveType, recordType) && strings.EqualFold(haveName, name) {
			content, _ := record["content"].(string)
			out = append(out, content)
		}
	}
	return out
}

func (s *checkStub) hasRecord(recordType, name, content string, proxied bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.dnsRecords {
		haveType, _ := record["type"].(string)
		haveName, _ := record["name"].(string)
		haveContent, _ := record["content"].(string)
		haveProxied, _ := record["proxied"].(bool)
		if strings.EqualFold(haveType, recordType) && strings.EqualFold(haveName, name) &&
			haveContent == content && haveProxied == proxied {
			return true
		}
	}
	return false
}

func (s *checkStub) setCatchAllTarget(target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.catchAllTarget = target
}

func (s *checkStub) catchAll() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.catchAllEnabled, s.catchAllTarget
}

// checkRouter wires only the routes the check tests need, under the real admin middleware.
func checkRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	admin := r.Group("/api/admin")
	admin.Use(middleware.AuthMiddleware(), middleware.AdminMiddleware())
	{
		admin.POST("/cloudflare/profiles", handlers.CFCreateProfile)
		admin.POST("/cloudflare/domains", handlers.CFAddDomain)
		admin.PUT("/cloudflare/tunnel", handlers.CFSetTunnel)
		admin.GET("/cloudflare/domains/:domain/check", handlers.CFDomainCheck)
		admin.POST("/cloudflare/domains/:domain/fix", handlers.CFDomainFix)
		admin.POST("/cloudflare/tunnel/cname", handlers.CFTunnelCNAME)
	}
	return r
}

// setupCheckDomain registers one profile plus one domain against the stub, points the tunnel at
// testWebHost and records the Worker as deployed, so a healthy baseline only depends on the
// Cloudflare facts the stub serves.
func setupCheckDomain(t *testing.T, stub *checkStub) *gin.Engine {
	t.Helper()
	resetCF(t)
	stub.start(t)
	r := checkRouter()
	if code, body := createProfile(t, r, "cf-a", true); code != http.StatusCreated {
		t.Fatalf("setup profile: %d %s", code, body)
	}
	if code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains", "admin",
		`{"domain":"domain-a.invalid"}`); code != http.StatusCreated {
		t.Fatalf("setup domain: %d %s", code, body)
	}
	if code, body := doJSON(t, r, http.MethodPut, "/api/admin/cloudflare/tunnel", "admin",
		fmt.Sprintf(`{"web_host":%q,"id":"tunnel-id-1","name":%q}`, testWebHost, worker.Name())); code != http.StatusOK {
		t.Fatalf("setup tunnel: %d %s", code, body)
	}
	if err := repository.SetCFProfileWorkerDeployed(database.Conn(), "cf-a", true); err != nil {
		t.Fatalf("mark worker deployed: %v", err)
	}
	stub.resetMutations()
	return r
}

type checkZoneJSON struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type checkDNSRecordJSON struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Proxied  bool   `json:"proxied"`
	Priority *int   `json:"priority"`
}

type checkIssueJSON struct {
	Key     string `json:"key"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Fixable bool   `json:"fixable"`
}

type domainCheckJSON struct {
	Domain  string        `json:"domain"`
	Profile string        `json:"profile"`
	Zone    checkZoneJSON `json:"zone"`
	Worker  struct {
		Name     string `json:"name"`
		Deployed bool   `json:"deployed"`
	} `json:"worker"`
	Routing struct {
		Enabled bool   `json:"enabled"`
		Target  string `json:"target"`
		OK      bool   `json:"ok"`
	} `json:"routing"`
	EmailDNS struct {
		Ready     bool                 `json:"ready"`
		Missing   []checkDNSRecordJSON `json:"missing"`
		Conflicts []checkDNSRecordJSON `json:"conflicts"`
	} `json:"email_dns"`
	WebHost struct {
		Configured bool                 `json:"configured"`
		Host       string               `json:"host"`
		Expected   string               `json:"expected"`
		Zone       checkZoneJSON        `json:"zone"`
		Records    []checkDNSRecordJSON `json:"records"`
		OK         bool                 `json:"ok"`
	} `json:"web_host"`
	Issues []checkIssueJSON `json:"issues"`
}

type fixStepJSON struct {
	Key     string `json:"key"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Detail  string `json:"detail"`
}

type domainFixJSON struct {
	Domain  string        `json:"domain"`
	Done    []fixStepJSON `json:"done"`
	Skipped []fixStepJSON `json:"skipped"`
	Failed  []fixStepJSON `json:"failed"`
}

func checkDomain(t *testing.T, r *gin.Engine, domain string) domainCheckJSON {
	t.Helper()
	code, body := doJSON(t, r, http.MethodGet, "/api/admin/cloudflare/domains/"+domain+"/check", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("check %s: got %d %s", domain, code, body)
	}
	var view domainCheckJSON
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode check view: %v (%s)", err, body)
	}
	return view
}

func fixDomain(t *testing.T, r *gin.Engine, domain, body string) (int, string, domainFixJSON) {
	t.Helper()
	code, raw := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/domains/"+domain+"/fix", "admin", body)
	var report domainFixJSON
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(raw), &report); err != nil {
			t.Fatalf("decode fix report: %v (%s)", err, raw)
		}
	}
	return code, raw, report
}

func issueFor(t *testing.T, issues []checkIssueJSON, key string) checkIssueJSON {
	t.Helper()
	for _, issue := range issues {
		if issue.Key == key {
			return issue
		}
	}
	t.Fatalf("issue %q missing from %+v", key, issues)
	return checkIssueJSON{}
}

func stepFor(t *testing.T, steps []fixStepJSON, key string) fixStepJSON {
	t.Helper()
	for _, step := range steps {
		if step.Key == key {
			return step
		}
	}
	t.Fatalf("step %q missing from %+v", key, steps)
	return fixStepJSON{}
}

func TestCFCheckRoutesAreRegisteredUnderAdminGroup(t *testing.T) {
	registered := map[string]bool{}
	for _, route := range api.SetupRouter().Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	for _, route := range []string{
		"GET /api/admin/cloudflare/domains/:domain/check",
		"POST /api/admin/cloudflare/domains/:domain/fix",
		"POST /api/admin/cloudflare/tunnel/cname",
	} {
		if !registered[route] {
			t.Errorf("route not registered: %s", route)
		}
	}
}

func TestCFDomainCheckReportsHealthyDomain(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)

	view := checkDomain(t, r, "domain-a.invalid")
	if view.Domain != "domain-a.invalid" || view.Profile != "cf-a" {
		t.Fatalf("unexpected identity: %+v", view)
	}
	if view.Zone.ID != "zone-2" || view.Zone.Name != "domain-a.invalid" || view.Zone.Status != "active" {
		t.Fatalf("unexpected zone: %+v", view.Zone)
	}
	if view.Worker.Name != worker.Name() || !view.Worker.Deployed {
		t.Fatalf("unexpected worker: %+v", view.Worker)
	}
	if !view.Routing.Enabled || !view.Routing.OK || view.Routing.Target != worker.Name() {
		t.Fatalf("unexpected routing: %+v", view.Routing)
	}
	if !view.EmailDNS.Ready || len(view.EmailDNS.Missing) != 0 || len(view.EmailDNS.Conflicts) != 0 {
		t.Fatalf("unexpected email dns: %+v", view.EmailDNS)
	}
	if !view.WebHost.Configured || !view.WebHost.OK {
		t.Fatalf("unexpected web host: %+v", view.WebHost)
	}
	if view.WebHost.Host != testWebHost || view.WebHost.Expected != "tunnel-id-1.cfargotunnel.com" {
		t.Fatalf("unexpected web host target: %+v", view.WebHost)
	}
	if view.WebHost.Zone.ID != "zone-1" || len(view.WebHost.Records) != 1 || !view.WebHost.Records[0].Proxied {
		t.Fatalf("unexpected web host records: %+v", view.WebHost)
	}
	if len(view.Issues) != 0 {
		t.Fatalf("healthy domain must report no issue, got %+v", view.Issues)
	}
	if mutations := stub.mutationCount(); mutations != 0 {
		t.Fatalf("check must not mutate Cloudflare, saw %d mutating calls", mutations)
	}
}

func TestCFDomainCheckFlagsRoutingTargetMismatch(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	stub.setCatchAllTarget("worker-lain")

	view := checkDomain(t, r, "domain-a.invalid")
	if view.Routing.OK {
		t.Fatalf("routing should not be ok: %+v", view.Routing)
	}
	if view.Routing.Target != "worker-lain" || !view.Routing.Enabled {
		t.Fatalf("unexpected routing: %+v", view.Routing)
	}
	issue := issueFor(t, view.Issues, "routing")
	if !issue.Fixable || issue.Level != "bad" {
		t.Fatalf("routing issue must be a fixable bad issue: %+v", issue)
	}
	if !strings.Contains(issue.Message, "worker-lain") {
		t.Fatalf("routing issue should name the wrong target: %+v", issue)
	}
	if stub.mutationCount() != 0 {
		t.Fatalf("check must not mutate Cloudflare")
	}
}

func TestCFDomainCheckFlagsMissingWebHostCNAME(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	stub.removeRecords("CNAME", testWebHost)

	view := checkDomain(t, r, "domain-a.invalid")
	if view.WebHost.OK {
		t.Fatalf("web host should not be ok without a record: %+v", view.WebHost)
	}
	if len(view.WebHost.Records) != 0 {
		t.Fatalf("expected no web host records, got %+v", view.WebHost.Records)
	}
	if view.WebHost.Zone.ID != "zone-1" || !view.WebHost.Configured {
		t.Fatalf("unexpected web host state: %+v", view.WebHost)
	}
	issue := issueFor(t, view.Issues, "cname")
	if !issue.Fixable {
		t.Fatalf("cname issue must be fixable: %+v", issue)
	}
	if !strings.Contains(issue.Message, testWebHost) {
		t.Fatalf("cname issue should name the host: %+v", issue)
	}
	// The missing web host record must not be confused with broken mail routing.
	if !view.EmailDNS.Ready || len(view.EmailDNS.Missing) != 0 {
		t.Fatalf("email dns should stay ready: %+v", view.EmailDNS)
	}
}

func TestCFDomainCheckReportsForeignMXConflict(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	priority := 5
	stub.addRecord("MX", "domain-a.invalid", "mail.other.invalid", false, &priority)

	view := checkDomain(t, r, "domain-a.invalid")
	if len(view.EmailDNS.Conflicts) != 1 {
		t.Fatalf("expected one MX conflict, got %+v", view.EmailDNS.Conflicts)
	}
	if view.EmailDNS.Conflicts[0].Content != "mail.other.invalid" {
		t.Fatalf("unexpected conflict record: %+v", view.EmailDNS.Conflicts[0])
	}
	if !view.EmailDNS.Ready {
		t.Fatalf("the required MX is still present, ready should stay true: %+v", view.EmailDNS)
	}
	issue := issueFor(t, view.Issues, "mx_conflict")
	if issue.Fixable {
		t.Fatalf("a foreign MX must never be advertised as fixable: %+v", issue)
	}
	if !strings.Contains(issue.Message, "mail.other.invalid") {
		t.Fatalf("mx conflict issue should name the record: %+v", issue)
	}
	if !view.WebHost.OK {
		t.Fatalf("unrelated web host state changed: %+v", view.WebHost)
	}
}

func TestCFDomainCheckRejectsUnregisteredDomain(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)

	if code, body := doJSON(t, r, http.MethodGet, "/api/admin/cloudflare/domains/tidak-ada.invalid/check", "admin", ""); code != http.StatusNotFound {
		t.Fatalf("unregistered domain: got %d %s", code, body)
	}
}

func TestCFDomainFixCreatesMissingCNAME(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	stub.removeRecords("CNAME", testWebHost)

	code, body, report := fixDomain(t, r, "domain-a.invalid", `{"cname":true}`)
	if code != http.StatusOK {
		t.Fatalf("fix cname: got %d %s", code, body)
	}
	if report.Domain != "domain-a.invalid" {
		t.Fatalf("unexpected report domain: %s", body)
	}
	step := stepFor(t, report.Done, "cname")
	if !step.OK {
		t.Fatalf("done step must be ok: %+v", step)
	}
	if len(report.Skipped) != 0 || len(report.Failed) != 0 {
		t.Fatalf("expected only a done step, got %s", body)
	}
	if !stub.hasRecord("CNAME", testWebHost, "tunnel-id-1.cfargotunnel.com", true) {
		t.Fatalf("CNAME was not created, records: %+v", stub.contentsFor("CNAME", testWebHost))
	}
	// The report keys of a done step carry ok, while the skipped and failed arrays do not.
	var raw struct {
		Done    []map[string]any `json:"done"`
		Skipped []map[string]any `json:"skipped"`
		Failed  []map[string]any `json:"failed"`
	}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode raw report: %v", err)
	}
	if okValue, found := raw.Done[0]["ok"]; !found || okValue != true {
		t.Fatalf("done step shape: %s", body)
	}
	if len(raw.Skipped) != 0 {
		t.Fatalf("skipped should be empty: %s", body)
	}

	view := checkDomain(t, r, "domain-a.invalid")
	if !view.WebHost.OK || len(view.Issues) != 0 {
		t.Fatalf("check after repair: %+v %+v", view.WebHost, view.Issues)
	}
}

func TestCFDomainFixSkipsConflictingCNAMEUnlessOverwrite(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	stub.removeRecords("CNAME", testWebHost)
	stub.addRecord("CNAME", testWebHost, "legacy.example.invalid", true, nil)

	code, body, report := fixDomain(t, r, "domain-a.invalid", `{"cname":true}`)
	if code != http.StatusOK {
		t.Fatalf("fix cname: got %d %s", code, body)
	}
	step := stepFor(t, report.Skipped, "cname")
	if step.Message == "" {
		t.Fatalf("skipped step needs a message: %+v", step)
	}
	if len(report.Done) != 0 || len(report.Failed) != 0 {
		t.Fatalf("expected only a skipped step, got %s", body)
	}
	var raw struct {
		Skipped []map[string]any `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("decode raw report: %v", err)
	}
	if _, found := raw.Skipped[0]["ok"]; found {
		t.Fatalf("skipped step must not carry ok: %s", body)
	}
	if got := stub.contentsFor("CNAME", testWebHost); len(got) != 1 || got[0] != "legacy.example.invalid" {
		t.Fatalf("conflicting CNAME must be untouched, got %+v", got)
	}

	// overwrite_cname replaces only the host's own CNAME records.
	code, body, report = fixDomain(t, r, "domain-a.invalid", `{"cname":true,"overwrite_cname":true}`)
	if code != http.StatusOK {
		t.Fatalf("fix overwrite: got %d %s", code, body)
	}
	step = stepFor(t, report.Done, "cname")
	if !step.OK || len(report.Skipped) != 0 || len(report.Failed) != 0 {
		t.Fatalf("expected a done step, got %s", body)
	}
	if got := stub.contentsFor("CNAME", testWebHost); len(got) != 1 || got[0] != "tunnel-id-1.cfargotunnel.com" {
		t.Fatalf("CNAME was not replaced, got %+v", got)
	}
	// Nothing else was deleted: the mail records are still there.
	if len(stub.contentsFor("MX", "domain-a.invalid")) != 1 {
		t.Fatal("the fix deleted unrelated records")
	}
}

func TestCFDomainFixRestoresRouting(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	stub.setCatchAllTarget("worker-lain")
	if code, body, untouch := fixDomain(t, r, "domain-a.invalid", `{}`); code != http.StatusOK ||
		len(untouch.Done)+len(untouch.Skipped)+len(untouch.Failed) != 0 {
		t.Fatalf("an empty selection must run nothing: got %d %s", code, body)
	}
	if _, target := stub.catchAll(); target != "worker-lain" {
		t.Fatalf("an empty selection changed the catch-all: %q", target)
	}

	code, body, report := fixDomain(t, r, "domain-a.invalid", `{"routing":true}`)
	if code != http.StatusOK {
		t.Fatalf("fix routing: got %d %s", code, body)
	}
	step := stepFor(t, report.Done, "routing")
	if !step.OK {
		t.Fatalf("routing repair failed: %s", body)
	}
	enabled, target := stub.catchAll()
	if !enabled || target != worker.Name() {
		t.Fatalf("catch-all not restored: enabled=%t target=%q", enabled, target)
	}
	stored, err := repository.GetCFDomain(database.Conn(), "domain-a.invalid")
	if err != nil {
		t.Fatalf("read stored domain: %v", err)
	}
	if !stored.Routing || !stored.Enabled {
		t.Fatalf("domain state not persisted: %+v", stored)
	}
	if view := checkDomain(t, r, "domain-a.invalid"); !view.Routing.OK {
		t.Fatalf("routing still not ok after repair: %+v", view.Routing)
	}
}

func TestCFDomainFixCreatesMissingEmailDNSRecords(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	stub.removeRecords("MX", "domain-a.invalid")
	stub.removeRecords("TXT", "domain-a.invalid")

	view := checkDomain(t, r, "domain-a.invalid")
	if view.EmailDNS.Ready || len(view.EmailDNS.Missing) != 2 {
		t.Fatalf("expected two missing records, got %+v", view.EmailDNS)
	}
	missing := issueFor(t, view.Issues, "email_dns")
	if !missing.Fixable {
		t.Fatalf("email_dns issue must be fixable: %+v", missing)
	}

	code, body, report := fixDomain(t, r, "domain-a.invalid", `{"email_dns":true}`)
	if code != http.StatusOK {
		t.Fatalf("fix email dns: got %d %s", code, body)
	}
	if step := stepFor(t, report.Done, "email_dns"); !step.OK {
		t.Fatalf("email dns repair failed: %s", body)
	}
	if len(report.Skipped) != 0 || len(report.Failed) != 0 {
		t.Fatalf("expected only a done step, got %s", body)
	}
	if !stub.hasRecord("MX", "domain-a.invalid", "route1.mx.cloudflare.net", false) {
		t.Fatal("required MX was not created")
	}
	if !stub.hasRecord("TXT", "domain-a.invalid", "v=spf1 include:_spf.mx.cloudflare.net ~all", false) {
		t.Fatal("required TXT was not created")
	}

	view = checkDomain(t, r, "domain-a.invalid")
	if !view.EmailDNS.Ready || len(view.EmailDNS.Missing) != 0 {
		t.Fatalf("email dns still not ready: %+v", view.EmailDNS)
	}
}

func TestCFTunnelCNAMEReportsAndCreatesWebHostRecord(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)

	code, body := doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/tunnel/cname", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("tunnel cname: got %d %s", code, body)
	}
	type tunnelCNAMEView struct {
		WebHost  string        `json:"web_host"`
		Expected string        `json:"expected"`
		Created  bool          `json:"created"`
		Existing bool          `json:"existing"`
		Zone     checkZoneJSON `json:"zone"`
	}
	var existing tunnelCNAMEView
	if err := json.Unmarshal([]byte(body), &existing); err != nil {
		t.Fatalf("decode tunnel cname: %v (%s)", err, body)
	}
	if existing.WebHost != testWebHost || existing.Expected != "tunnel-id-1.cfargotunnel.com" {
		t.Fatalf("unexpected tunnel cname view: %s", body)
	}
	if existing.Created || !existing.Existing || existing.Zone.ID != "zone-1" || existing.Zone.Name != "example.invalid" {
		t.Fatalf("existing record should be reported, not recreated: %s", body)
	}

	stub.removeRecords("CNAME", testWebHost)
	code, body = doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/tunnel/cname", "admin", "")
	if code != http.StatusOK {
		t.Fatalf("tunnel cname create: got %d %s", code, body)
	}
	var created tunnelCNAMEView
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("decode tunnel cname: %v (%s)", err, body)
	}
	if !created.Created || created.Existing {
		t.Fatalf("missing record should be created: %s", body)
	}
	if !stub.hasRecord("CNAME", testWebHost, "tunnel-id-1.cfargotunnel.com", true) {
		t.Fatal("tunnel CNAME was not created")
	}

	db := database.Conn()
	if err := repository.SetSetting(db, repository.SettingTunnelWebHost, ""); err != nil {
		t.Fatalf("clear web host: %v", err)
	}
	code, body = doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/tunnel/cname", "admin", "")
	if code != http.StatusBadRequest || !strings.Contains(body, "Web host") {
		t.Fatalf("missing web host: got %d %s", code, body)
	}

	if err := repository.SetSetting(db, repository.SettingTunnelWebHost, testWebHost); err != nil {
		t.Fatalf("restore web host: %v", err)
	}
	if err := repository.SetSetting(db, "cf_tunnel_id", ""); err != nil {
		t.Fatalf("clear tunnel id: %v", err)
	}
	code, body = doJSON(t, r, http.MethodPost, "/api/admin/cloudflare/tunnel/cname", "admin", "")
	if code != http.StatusBadRequest || !strings.Contains(body, "ID tunnel") {
		t.Fatalf("missing tunnel id: got %d %s", code, body)
	}
}

// A Worker deleted outside the panel must not be reported as ready: the check asks Cloudflare.
func TestCFDomainCheckReadsWorkerFromCloudflare(t *testing.T) {
	stub := newCheckStub()
	stub.workerExists = false
	r := setupCheckDomain(t, stub)

	view := checkDomain(t, r, "domain-a.invalid")
	if view.Worker.Deployed {
		t.Fatalf("Worker tanpa script di Cloudflare tidak boleh siap: %+v", view.Worker)
	}
	if !issueFixable(view.Issues, "worker") {
		t.Fatalf("issue worker fixable hilang: %+v", view.Issues)
	}

	code, body, report := fixDomain(t, r, "domain-a.invalid", `{"worker":true}`)
	if code != http.StatusOK {
		t.Fatalf("fix worker: %d %s", code, body)
	}
	if step := stepFor(t, report.Done, "worker"); !step.OK {
		t.Fatalf("langkah worker harus selesai: %s", body)
	}
	after := checkDomain(t, r, "domain-a.invalid")
	if !after.Worker.Deployed || issueFixable(after.Issues, "worker") {
		t.Fatalf("setelah perbaikan Worker harus siap: %+v %+v", after.Worker, after.Issues)
	}
}

// A Worker that exists in Cloudflare while the database says otherwise is reported as drift.
func TestCFDomainCheckFlagsWorkerDrift(t *testing.T) {
	stub := newCheckStub()
	r := setupCheckDomain(t, stub)
	if err := repository.SetCFProfileWorkerDeployed(database.Conn(), "cf-a", false); err != nil {
		t.Fatalf("clear worker flag: %v", err)
	}

	view := checkDomain(t, r, "domain-a.invalid")
	if !view.Worker.Deployed {
		t.Fatalf("Worker ada di Cloudflare, harus siap: %+v", view.Worker)
	}
	drifted := false
	for _, issue := range view.Issues {
		if issue.Key == "worker" && issue.Level == "warn" && issue.Fixable {
			drifted = true
		}
	}
	if !drifted {
		t.Fatalf("drift Worker tidak dilaporkan: %+v", view.Issues)
	}
}

func issueFixable(issues []checkIssueJSON, key string) bool {
	for _, issue := range issues {
		if issue.Key == key && issue.Fixable {
			return true
		}
	}
	return false
}
