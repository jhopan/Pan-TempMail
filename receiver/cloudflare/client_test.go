package cloudflare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "cf-test-token-do-not-log"

type captured struct {
	method string
	path   string
	query  string
	auth   string
	ctype  string
	body   []byte
}

func newStub(t *testing.T, handler http.HandlerFunc) (*Client, *captured) {
	t.Helper()
	cap := &captured{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.query = r.URL.RawQuery
		cap.auth = r.Header.Get("Authorization")
		cap.ctype = r.Header.Get("Content-Type")
		cap.body = raw
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return NewClientWithBase(testToken, server.URL), cap
}

func jsonStub(t *testing.T, status int, payload string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := io.WriteString(w, payload); err != nil {
			t.Errorf("write stub response: %v", err)
		}
	}
}

func TestAccountSuccessEnvelope(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{"id":"acc-1","name":"Acme Ltd"}}`))

	account, err := client.Account("acc-1")
	if err != nil {
		t.Fatalf("Account: %v", err)
	}
	if account.ID != "acc-1" || account.Name != "Acme Ltd" {
		t.Fatalf("account = %+v", account)
	}
	if cap.method != http.MethodGet {
		t.Fatalf("method = %s", cap.method)
	}
	if cap.path != "/accounts/acc-1" {
		t.Fatalf("path = %s", cap.path)
	}
	if cap.auth != "Bearer "+testToken {
		t.Fatalf("authorization = %q", cap.auth)
	}
}

func TestNewClientReadsBaseFromEnv(t *testing.T) {
	server := httptest.NewServer(jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{"id":"acc-1","name":"Acme"}}`))
	t.Cleanup(server.Close)
	t.Setenv("CLOUDFLARE_API_BASE", server.URL+"/")

	if _, err := NewClient(testToken).Account("acc-1"); err != nil {
		t.Fatalf("Account via env base: %v", err)
	}
}

func TestErrorEnvelopeMapsToAPIError(t *testing.T) {
	payload := fmt.Sprintf(`{"success":false,"errors":[{"code":9109,"message":"Invalid access token %s"}],"result":null}`, testToken)
	client, _ := newStub(t, jsonStub(t, http.StatusForbidden, payload))

	_, err := client.Account("acc-1")
	if err == nil {
		t.Fatal("Account: want error, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T (%v)", err, err)
	}
	if apiErr.Status != http.StatusForbidden {
		t.Fatalf("status = %d", apiErr.Status)
	}
	if len(apiErr.Codes) != 1 || apiErr.Codes[0] != 9109 {
		t.Fatalf("codes = %v", apiErr.Codes)
	}
	if len(apiErr.Messages) != 1 || !strings.Contains(apiErr.Messages[0], "Invalid access token") {
		t.Fatalf("messages = %v", apiErr.Messages)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("error text leaks token: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "9109") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("error text = %q", err.Error())
	}
}

func TestErrorEnvelopeMapsWith200Status(t *testing.T) {
	client, _ := newStub(t, jsonStub(t, http.StatusOK, `{"success":false,"errors":[{"code":1001,"message":"bad request"}],"result":null}`))

	_, err := client.Account("acc-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T (%v)", err, err)
	}
	if apiErr.Status != http.StatusOK || len(apiErr.Codes) != 1 || apiErr.Codes[0] != 1001 {
		t.Fatalf("apiErr = %+v", apiErr)
	}
}

func TestNonJSONErrorResponse(t *testing.T) {
	client, _ := newStub(t, jsonStub(t, http.StatusBadGateway, `<html>bad gateway</html>`))

	_, err := client.Account("acc-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T (%v)", err, err)
	}
	if apiErr.Status != http.StatusBadGateway {
		t.Fatalf("status = %d", apiErr.Status)
	}
	if len(apiErr.Messages) != 1 || !strings.Contains(apiErr.Messages[0], "bad gateway") {
		t.Fatalf("messages = %v", apiErr.Messages)
	}
}

func TestZoneID(t *testing.T) {
	t.Run("active", func(t *testing.T) {
		client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[{"id":"zone-1","name":"example.com","status":"active"}]}`))

		zoneID, err := client.ZoneID("example.com")
		if err != nil {
			t.Fatalf("ZoneID: %v", err)
		}
		if zoneID != "zone-1" {
			t.Fatalf("zoneID = %q", zoneID)
		}
		if cap.path != "/zones" || cap.query != "name=example.com" {
			t.Fatalf("request = %s?%s", cap.path, cap.query)
		}
	})

	t.Run("pending", func(t *testing.T) {
		client, _ := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[{"id":"zone-1","name":"example.com","status":"pending"}]}`))

		if _, err := client.ZoneID("example.com"); !errors.Is(err, ErrZoneNotFound) {
			t.Fatalf("err = %v, want ErrZoneNotFound", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		client, _ := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[]}`))

		if _, err := client.ZoneID("example.com"); !errors.Is(err, ErrZoneNotFound) {
			t.Fatalf("err = %v, want ErrZoneNotFound", err)
		}
	})
}

func TestEnableEmailRouting(t *testing.T) {
	t.Run("already enabled", func(t *testing.T) {
		client, cap := newStub(t, jsonStub(t, http.StatusBadRequest, `{"success":false,"errors":[{"code":2008,"message":"Email Routing is already enabled"}],"result":null}`))

		if err := client.EnableEmailRouting("zone-1"); err != nil {
			t.Fatalf("EnableEmailRouting: %v", err)
		}
		if cap.method != http.MethodPost || cap.path != "/zones/zone-1/email/routing/enable" {
			t.Fatalf("request = %s %s", cap.method, cap.path)
		}
	})

	t.Run("other error", func(t *testing.T) {
		client, _ := newStub(t, jsonStub(t, http.StatusForbidden, `{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}],"result":null}`))

		var apiErr *APIError
		if err := client.EnableEmailRouting("zone-1"); !errors.As(err, &apiErr) {
			t.Fatalf("err = %v, want *APIError", err)
		}
	})

	t.Run("ok", func(t *testing.T) {
		client, _ := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{}}`))

		if err := client.EnableEmailRouting("zone-1"); err != nil {
			t.Fatalf("EnableEmailRouting: %v", err)
		}
	})
}

func TestUnlockEmailRoutingDNS(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{}}`))

	if err := client.UnlockEmailRoutingDNS("zone-1"); err != nil {
		t.Fatalf("UnlockEmailRoutingDNS: %v", err)
	}
	if cap.method != http.MethodDelete || cap.path != "/zones/zone-1/email/routing/dns" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
}

func TestSetCatchAll(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{}}`))

		if err := client.SetCatchAll("zone-1", "tempmailbyjhopanstore", true); err != nil {
			t.Fatalf("SetCatchAll: %v", err)
		}
		if cap.method != http.MethodPut || cap.path != "/zones/zone-1/email/routing/rules/catch_all" {
			t.Fatalf("request = %s %s", cap.method, cap.path)
		}
		want := `{"enabled":true,"name":"Pan-TempMail catch-all","matchers":[{"type":"all"}],"actions":[{"type":"worker","value":["tempmailbyjhopanstore"]}]}`
		if string(cap.body) != want {
			t.Fatalf("body =\n%s\nwant\n%s", cap.body, want)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{}}`))

		if err := client.SetCatchAll("zone-1", "tempmailbyjhopanstore", false); err != nil {
			t.Fatalf("SetCatchAll: %v", err)
		}
		want := `{"enabled":false,"name":"Pan-TempMail catch-all","matchers":[{"type":"all"}],"actions":[{"type":"worker","value":["tempmailbyjhopanstore"]}]}`
		if string(cap.body) != want {
			t.Fatalf("body =\n%s\nwant\n%s", cap.body, want)
		}
	})
}

func TestDeployWorkerMultipart(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{"id":"script-1","modified_on":"2026-01-02T03:04:05Z"}}`))

	bindings := []Binding{
		{Name: "TEMPMAIL_SERVER_URL", Type: "secret_text", Text: "https://mail.example.com"},
		{Name: "FORWARD_LIST", Type: "plain_text", Text: "a@b.c"},
	}
	const script = "export default { async email() {} };\n"
	deployment, err := client.DeployWorker("acc-1", "tempmailbyjhopanstore", "2024-01-01", script, bindings)
	if err != nil {
		t.Fatalf("DeployWorker: %v", err)
	}
	if deployment.ID != "script-1" || deployment.ModifiedOn != "2026-01-02T03:04:05Z" {
		t.Fatalf("deployment = %+v", deployment)
	}
	if cap.method != http.MethodPut || cap.path != "/accounts/acc-1/workers/scripts/tempmailbyjhopanstore" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
	if !strings.Contains(string(cap.body), `name="metadata"`) || !strings.Contains(string(cap.body), `name="index.js"`) {
		t.Fatalf("multipart body missing part names:\n%s", cap.body)
	}

	mediaType, params, err := mime.ParseMediaType(cap.ctype)
	if err != nil {
		t.Fatalf("parse content type: %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %s", mediaType)
	}

	parts := map[string]string{}
	partTypes := map[string]string{}
	partFileNames := map[string]string{}
	reader := multipart.NewReader(bytes.NewReader(cap.body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		raw, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		parts[part.FormName()] = string(raw)
		partTypes[part.FormName()] = part.Header.Get("Content-Type")
		partFileNames[part.FormName()] = part.FileName()
	}

	if _, ok := parts["metadata"]; !ok {
		t.Fatalf("parts = %v", parts)
	}
	if parts["index.js"] != script {
		t.Fatalf("script part = %q", parts["index.js"])
	}
	if partFileNames["index.js"] != "index.js" {
		t.Fatalf("module part must be sent as a file with a filename, got %q", partFileNames["index.js"])
	}
	if partFileNames["metadata"] != "" {
		t.Fatalf("metadata part must stay a form field, got filename %q", partFileNames["metadata"])
	}
	if partTypes["index.js"] != "application/javascript+module" {
		t.Fatalf("script part content type = %q", partTypes["index.js"])
	}
	if partTypes["metadata"] != "application/json" {
		t.Fatalf("metadata part content type = %q", partTypes["metadata"])
	}

	var meta struct {
		MainModule        string `json:"main_module"`
		CompatibilityDate string `json:"compatibility_date"`
		Bindings          []struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal([]byte(parts["metadata"]), &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if meta.MainModule != "index.js" {
		t.Fatalf("main_module = %q", meta.MainModule)
	}
	if meta.CompatibilityDate != "2024-01-01" {
		t.Fatalf("compatibility_date = %q", meta.CompatibilityDate)
	}
	if len(meta.Bindings) != 2 || meta.Bindings[0].Name != "TEMPMAIL_SERVER_URL" || meta.Bindings[0].Type != "secret_text" || meta.Bindings[0].Text != "https://mail.example.com" {
		t.Fatalf("bindings = %+v", meta.Bindings)
	}
}

func TestDeployWorkerWithoutBindings(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{"id":"script-1"}}`))

	if _, err := client.DeployWorker("acc-1", "worker", "2024-01-01", "export default {};", nil); err != nil {
		t.Fatalf("DeployWorker: %v", err)
	}
	if !strings.Contains(string(cap.body), `"bindings":[]`) {
		t.Fatalf("metadata missing empty bindings array:\n%s", cap.body)
	}
}

func TestSetWorkerSubdomain(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{}}`))

	if err := client.SetWorkerSubdomain("acc-1", "worker", true, false); err != nil {
		t.Fatalf("SetWorkerSubdomain: %v", err)
	}
	if cap.method != http.MethodPost || cap.path != "/accounts/acc-1/workers/scripts/worker/subdomain" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
	if string(cap.body) != `{"enabled":true,"previews_enabled":false}` {
		t.Fatalf("body = %s", cap.body)
	}
}

func TestDeleteWorker(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":{}}`))

		if err := client.DeleteWorker("acc-1", "worker"); err != nil {
			t.Fatalf("DeleteWorker: %v", err)
		}
		if cap.method != http.MethodDelete || cap.path != "/accounts/acc-1/workers/scripts/worker" {
			t.Fatalf("request = %s %s", cap.method, cap.path)
		}
	})

	t.Run("missing is success", func(t *testing.T) {
		client, _ := newStub(t, jsonStub(t, http.StatusNotFound, `{"success":false,"errors":[{"code":10007,"message":"script not found"}],"result":null}`))

		if err := client.DeleteWorker("acc-1", "worker"); err != nil {
			t.Fatalf("DeleteWorker: %v", err)
		}
	})

	t.Run("other error", func(t *testing.T) {
		client, _ := newStub(t, jsonStub(t, http.StatusForbidden, `{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}],"result":null}`))

		var apiErr *APIError
		if err := client.DeleteWorker("acc-1", "worker"); !errors.As(err, &apiErr) {
			t.Fatalf("err = %v, want *APIError", err)
		}
	})
}

func TestListTunnels(t *testing.T) {
	payload := `{"success":true,"errors":[],"result":[{"id":"t-1","name":"tempmailbyjhopanstore","status":"healthy","created_on":"2026-02-03T04:05:06Z","connections":[{"id":"c-1"},{"id":"c-2"}]}]}`
	client, cap := newStub(t, jsonStub(t, http.StatusOK, payload))

	tunnels, err := client.ListTunnels("acc-1")
	if err != nil {
		t.Fatalf("ListTunnels: %v", err)
	}
	if cap.path != "/accounts/acc-1/cfd_tunnel" || cap.query != "is_deleted=false" {
		t.Fatalf("request = %s?%s", cap.path, cap.query)
	}
	if len(tunnels) != 1 {
		t.Fatalf("tunnels = %+v", tunnels)
	}
	want := Tunnel{ID: "t-1", Name: "tempmailbyjhopanstore", Status: "healthy", Connections: 2, CreatedOn: "2026-02-03T04:05:06Z"}
	if tunnels[0] != want {
		t.Fatalf("tunnel = %+v, want %+v", tunnels[0], want)
	}
}

func TestTunnelConnections(t *testing.T) {
	payload := `{"success":true,"errors":[],"result":[{"id":"c-1","client_version":"2026.1.0"},{"id":"c-2"}]}`
	client, cap := newStub(t, jsonStub(t, http.StatusOK, payload))

	count, err := client.TunnelConnections("acc-1", "t-1")
	if err != nil {
		t.Fatalf("TunnelConnections: %v", err)
	}
	if cap.path != "/accounts/acc-1/cfd_tunnel/t-1/connections" {
		t.Fatalf("request path = %s", cap.path)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	client, _ = newStub(t, jsonStub(t, http.StatusForbidden, `{"success":false,"errors":[{"code":9109,"message":"Invalid access token"}],"result":null}`))
	if _, err := client.TunnelConnections("acc-1", "t-1"); err == nil {
		t.Fatal("error response must surface as an error")
	}
}

func TestCreateTunnel(t *testing.T) {
	payload := `{"success":true,"errors":[],"result":{"id":"t-1","name":"tm","status":"down","created_on":"2026-02-03T04:05:06Z"}}`
	client, cap := newStub(t, jsonStub(t, http.StatusOK, payload))

	tunnel, err := client.CreateTunnel("acc-1", "tm")
	if err != nil {
		t.Fatalf("CreateTunnel: %v", err)
	}
	if cap.method != http.MethodPost || cap.path != "/accounts/acc-1/cfd_tunnel" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
	if string(cap.body) != `{"name":"tm","config_src":"cloudflare"}` {
		t.Fatalf("body = %s", cap.body)
	}
	want := Tunnel{ID: "t-1", Name: "tm", Status: "down", CreatedOn: "2026-02-03T04:05:06Z"}
	if tunnel != want {
		t.Fatalf("tunnel = %+v, want %+v", tunnel, want)
	}
}

func TestTunnelToken(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":"eyJhIjoiYWJjIn0.token"}`))

	token, err := client.TunnelToken("acc-1", "t-1")
	if err != nil {
		t.Fatalf("TunnelToken: %v", err)
	}
	if token != "eyJhIjoiYWJjIn0.token" {
		t.Fatalf("token = %q", token)
	}
	if cap.method != http.MethodGet || cap.path != "/accounts/acc-1/cfd_tunnel/t-1/token" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
}
