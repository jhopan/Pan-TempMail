package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultAPIBase = "https://api.cloudflare.com/client/v4"
	moduleName     = "index.js"
	maxResponse    = 8 << 20
)

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Tunnel struct {
	ID          string
	Name        string
	Status      string
	Connections int
	CreatedOn   string
}

// Binding is a Worker binding. Type is "secret_text" or "plain_text".
type Binding struct {
	Name string
	Type string
	Text string
}

type Deployment struct {
	ID         string `json:"id"`
	ModifiedOn string `json:"modified_on"`
}

type Client struct {
	base  string
	token string
	http  *http.Client
}

func NewClient(token string) *Client {
	base := os.Getenv("CLOUDFLARE_API_BASE")
	if base == "" {
		base = defaultAPIBase
	}
	return NewClientWithBase(token, base)
}

func NewClientWithBase(token, baseURL string) *Client {
	return &Client{
		base:  strings.TrimRight(baseURL, "/"),
		token: token,
		http:  &http.Client{Timeout: 20 * time.Second},
	}
}

type envelope struct {
	Success bool            `json:"success"`
	Errors  []envelopeError `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type envelopeError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) do(method, path, contentType string, body []byte) (json.RawMessage, error) {
	req, err := http.NewRequest(method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cloudflare: build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudflare: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("cloudflare: read response: %w", err)
	}

	var env envelope
	decodeErr := json.Unmarshal(raw, &env)
	if resp.StatusCode < 200 || resp.StatusCode > 299 || !env.Success || decodeErr != nil {
		apiErr := &APIError{Status: resp.StatusCode}
		for _, item := range env.Errors {
			apiErr.Codes = append(apiErr.Codes, item.Code)
			apiErr.Messages = append(apiErr.Messages, item.Message)
		}
		if len(apiErr.Messages) == 0 {
			apiErr.Messages = append(apiErr.Messages, bodySnippet(raw))
		}
		apiErr.redact(c.token)
		return nil, apiErr
	}
	return env.Result, nil
}

func bodySnippet(raw []byte) string {
	snippet := strings.TrimSpace(string(raw))
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	if snippet == "" {
		return "empty response body"
	}
	return snippet
}

func (c *Client) doJSON(method, path string, payload any) (json.RawMessage, error) {
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("cloudflare: encode request: %w", err)
		}
		body = encoded
	}
	return c.do(method, path, "application/json", body)
}

func decode(raw json.RawMessage, label string, out any) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("cloudflare: decode %s: %w", label, err)
	}
	return nil
}

func (c *Client) Account(accountID string) (Account, error) {
	raw, err := c.doJSON(http.MethodGet, "/accounts/"+url.PathEscape(accountID), nil)
	if err != nil {
		return Account{}, err
	}
	var account Account
	if err := decode(raw, "account", &account); err != nil {
		return Account{}, err
	}
	return account, nil
}

func (c *Client) ZoneID(domain string) (string, error) {
	raw, err := c.doJSON(http.MethodGet, "/zones?name="+url.QueryEscape(domain), nil)
	if err != nil {
		return "", err
	}
	var zones []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := decode(raw, "zones", &zones); err != nil {
		return "", err
	}
	if len(zones) == 0 || zones[0].Status != "active" {
		return "", ErrZoneNotFound
	}
	return zones[0].ID, nil
}

func (c *Client) EnableEmailRouting(zoneID string) error {
	_, err := c.doJSON(http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/email/routing/enable", nil)
	// 2008 means Email Routing was already on, which is the desired state.
	if hasCode(err, 2008) {
		return nil
	}
	return err
}

func (c *Client) UnlockEmailRoutingDNS(zoneID string) error {
	_, err := c.doJSON(http.MethodDelete, "/zones/"+url.PathEscape(zoneID)+"/email/routing/dns", nil)
	return err
}

type catchAllRequest struct {
	Enabled  bool             `json:"enabled"`
	Name     string           `json:"name"`
	Matchers []catchAllMatch  `json:"matchers"`
	Actions  []catchAllAction `json:"actions"`
}

type catchAllMatch struct {
	Type string `json:"type"`
}

type catchAllAction struct {
	Type  string   `json:"type"`
	Value []string `json:"value"`
}

func (c *Client) SetCatchAll(zoneID, workerName string, enabled bool) error {
	payload := catchAllRequest{
		Enabled:  enabled,
		Name:     "Pan-TempMail catch-all",
		Matchers: []catchAllMatch{{Type: "all"}},
		Actions:  []catchAllAction{{Type: "worker", Value: []string{workerName}}},
	}
	_, err := c.doJSON(http.MethodPut, "/zones/"+url.PathEscape(zoneID)+"/email/routing/rules/catch_all", payload)
	return err
}

type workerMetadata struct {
	MainModule        string          `json:"main_module"`
	CompatibilityDate string          `json:"compatibility_date"`
	Bindings          []workerBinding `json:"bindings"`
}

type workerBinding struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Text string `json:"text"`
}

func (c *Client) DeployWorker(accountID, name, compatDate, script string, bindings []Binding) (Deployment, error) {
	metadata := workerMetadata{
		MainModule:        moduleName,
		CompatibilityDate: compatDate,
		Bindings:          make([]workerBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		metadata.Bindings = append(metadata.Bindings, workerBinding{Name: binding.Name, Type: binding.Type, Text: binding.Text})
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return Deployment{}, fmt.Errorf("cloudflare: encode worker metadata: %w", err)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	// Cloudflare expects a JSON metadata part plus one file part per module part; the module
	// part carries a filename so it is uploaded as a file rather than a form field.
	if err := writePart(writer, "metadata", "", "application/json", encoded); err != nil {
		return Deployment{}, err
	}
	if err := writePart(writer, moduleName, moduleName, "application/javascript+module", []byte(script)); err != nil {
		return Deployment{}, err
	}
	if err := writer.Close(); err != nil {
		return Deployment{}, fmt.Errorf("cloudflare: build multipart body: %w", err)
	}

	path := "/accounts/" + url.PathEscape(accountID) + "/workers/scripts/" + url.PathEscape(name)
	raw, err := c.do(http.MethodPut, path, writer.FormDataContentType(), buf.Bytes())
	if err != nil {
		return Deployment{}, err
	}
	var deployment Deployment
	if err := decode(raw, "deployment", &deployment); err != nil {
		return Deployment{}, err
	}
	return deployment, nil
}

func writePart(writer *multipart.Writer, name, filename, contentType string, body []byte) error {
	header := make(textproto.MIMEHeader)
	if filename == "" {
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"`, name))
	} else {
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, name, filename))
	}
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return fmt.Errorf("cloudflare: create multipart part %s: %w", name, err)
	}
	if _, err := part.Write(body); err != nil {
		return fmt.Errorf("cloudflare: write multipart part %s: %w", name, err)
	}
	return nil
}

type subdomainRequest struct {
	Enabled         bool `json:"enabled"`
	PreviewsEnabled bool `json:"previews_enabled"`
}

func (c *Client) SetWorkerSubdomain(accountID, name string, enabled, previews bool) error {
	payload := subdomainRequest{Enabled: enabled, PreviewsEnabled: previews}
	path := "/accounts/" + url.PathEscape(accountID) + "/workers/scripts/" + url.PathEscape(name) + "/subdomain"
	_, err := c.doJSON(http.MethodPost, path, payload)
	return err
}

func (c *Client) DeleteWorker(accountID, name string) error {
	path := "/accounts/" + url.PathEscape(accountID) + "/workers/scripts/" + url.PathEscape(name)
	_, err := c.doJSON(http.MethodDelete, path, nil)
	if hasStatus(err, http.StatusNotFound) {
		return nil
	}
	return err
}

// WorkerScript reports whether the account already holds a Worker script with that name, so a
// stored deploy flag can be checked against Cloudflare instead of being trusted.
func (c *Client) WorkerScript(accountID, name string) (bool, error) {
	path := "/accounts/" + url.PathEscape(accountID) + "/workers/scripts/" + url.PathEscape(name)
	if _, err := c.doJSON(http.MethodGet, path, nil); err != nil {
		if hasStatus(err, http.StatusNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

type tunnelPayload struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedOn string `json:"created_on"`
	// Deprecated by Cloudflare and trending to an empty array; TunnelConnections is authoritative.
	Connections []json.RawMessage `json:"connections"`
}

func (t tunnelPayload) tunnel() Tunnel {
	return Tunnel{
		ID:          t.ID,
		Name:        t.Name,
		Status:      t.Status,
		Connections: len(t.Connections),
		CreatedOn:   t.CreatedOn,
	}
}

// TunnelConnections counts the connectors currently registered for a tunnel.
func (c *Client) TunnelConnections(accountID, tunnelID string) (int, error) {
	path := "/accounts/" + url.PathEscape(accountID) + "/cfd_tunnel/" + url.PathEscape(tunnelID) + "/connections"
	raw, err := c.doJSON(http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}
	var connections []json.RawMessage
	if err := decode(raw, "tunnel connections", &connections); err != nil {
		return 0, err
	}
	return len(connections), nil
}

func (c *Client) ListTunnels(accountID string) ([]Tunnel, error) {
	raw, err := c.doJSON(http.MethodGet, "/accounts/"+url.PathEscape(accountID)+"/cfd_tunnel?is_deleted=false", nil)
	if err != nil {
		return nil, err
	}
	var payloads []tunnelPayload
	if err := decode(raw, "tunnels", &payloads); err != nil {
		return nil, err
	}
	tunnels := make([]Tunnel, 0, len(payloads))
	for _, payload := range payloads {
		tunnels = append(tunnels, payload.tunnel())
	}
	return tunnels, nil
}

type createTunnelRequest struct {
	Name      string `json:"name"`
	ConfigSrc string `json:"config_src"`
}

func (c *Client) CreateTunnel(accountID, name string) (Tunnel, error) {
	payload := createTunnelRequest{Name: name, ConfigSrc: "cloudflare"}
	raw, err := c.doJSON(http.MethodPost, "/accounts/"+url.PathEscape(accountID)+"/cfd_tunnel", payload)
	if err != nil {
		return Tunnel{}, err
	}
	var created tunnelPayload
	if err := decode(raw, "tunnel", &created); err != nil {
		return Tunnel{}, err
	}
	return created.tunnel(), nil
}

func (c *Client) TunnelToken(accountID, tunnelID string) (string, error) {
	path := "/accounts/" + url.PathEscape(accountID) + "/cfd_tunnel/" + url.PathEscape(tunnelID) + "/token"
	raw, err := c.doJSON(http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	var token string
	if err := decode(raw, "tunnel token", &token); err != nil {
		return "", err
	}
	return token, nil
}
