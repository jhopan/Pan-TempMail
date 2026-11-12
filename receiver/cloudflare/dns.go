package cloudflare

import (
	"net/http"
	"net/url"
	"strings"
)

// Zone is a Cloudflare zone; Status is the zone state such as "active" or "pending".
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// DNSRecord is a zone DNS record. ID is empty on create requests; Priority is
// only meaningful for MX records. TTL 1 means automatic.
type DNSRecord struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Proxied  bool   `json:"proxied"`
	TTL      int    `json:"ttl,omitempty"`
	Priority *int   `json:"priority,omitempty"`
}

// CatchAll is the zone catch-all routing rule. Target is the worker name the
// rule hands mail to, and empty when the action is not a worker action.
type CatchAll struct {
	Enabled bool
	Target  string
}

// ZoneStatus looks the zone up by domain name without filtering on status, so
// the panel can report a pending or moved zone instead of hiding it.
func (c *Client) ZoneStatus(domain string) (Zone, error) {
	raw, err := c.doJSON(http.MethodGet, "/zones?name="+url.QueryEscape(domain), nil)
	if err != nil {
		return Zone{}, err
	}
	var zones []Zone
	if err := decode(raw, "zones", &zones); err != nil {
		return Zone{}, err
	}
	if len(zones) == 0 {
		return Zone{}, ErrZoneNotFound
	}
	return zones[0], nil
}

// ListZones lists the zones the token can see.
func (c *Client) ListZones() ([]Zone, error) {
	raw, err := c.doJSON(http.MethodGet, "/zones?per_page=50", nil)
	if err != nil {
		return nil, err
	}
	var zones []Zone
	if err := decode(raw, "zones", &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

// FindZoneForHost returns the zone with the longest label suffix of host, so a
// subdomain zone wins over its parent. A partial label match is not a match:
// "notexample.com" never matches the host "example.com".
func (c *Client) FindZoneForHost(host string) (Zone, error) {
	zones, err := c.ListZones()
	if err != nil {
		return Zone{}, err
	}
	host = normalizeHost(host)
	var best Zone
	bestLabels := 0
	for _, zone := range zones {
		name := normalizeHost(zone.Name)
		if name == "" || !hostSuffix(host, name) {
			continue
		}
		labels := strings.Count(name, ".") + 1
		if labels > bestLabels {
			best = zone
			bestLabels = labels
		}
	}
	if bestLabels == 0 {
		return Zone{}, ErrZoneNotFound
	}
	return best, nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

func hostSuffix(host, suffix string) bool {
	if host == suffix {
		return true
	}
	return strings.HasSuffix(host, "."+suffix)
}

type catchAllPayload struct {
	Enabled bool             `json:"enabled"`
	Actions []catchAllAction `json:"actions"`
}

func (c *Client) GetCatchAll(zoneID string) (CatchAll, error) {
	path := "/zones/" + url.PathEscape(zoneID) + "/email/routing/rules/catch_all"
	raw, err := c.doJSON(http.MethodGet, path, nil)
	if err != nil {
		return CatchAll{}, err
	}
	var payload catchAllPayload
	if err := decode(raw, "catch-all rule", &payload); err != nil {
		return CatchAll{}, err
	}
	rule := CatchAll{Enabled: payload.Enabled}
	for _, action := range payload.Actions {
		if action.Type == "worker" && len(action.Value) > 0 {
			rule.Target = action.Value[0]
			break
		}
	}
	return rule, nil
}

// EmailRoutingDNS returns the records Cloudflare requires for Email Routing.
func (c *Client) EmailRoutingDNS(zoneID string) ([]DNSRecord, error) {
	path := "/zones/" + url.PathEscape(zoneID) + "/email/routing/dns"
	raw, err := c.doJSON(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := decode(raw, "email routing dns", &records); err != nil {
		return nil, err
	}
	return records, nil
}

// ListDNSRecords filters the zone records; empty filters are omitted.
func (c *Client) ListDNSRecords(zoneID, recordType, name string) ([]DNSRecord, error) {
	query := url.Values{}
	query.Set("per_page", "100")
	if recordType != "" {
		query.Set("type", recordType)
	}
	if name != "" {
		query.Set("name", name)
	}
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records?" + query.Encode()
	raw, err := c.doJSON(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var records []DNSRecord
	if err := decode(raw, "dns records", &records); err != nil {
		return nil, err
	}
	return records, nil
}

func (c *Client) CreateDNSRecord(zoneID string, rec DNSRecord) (DNSRecord, error) {
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records"
	raw, err := c.doJSON(http.MethodPost, path, rec)
	if err != nil {
		return DNSRecord{}, err
	}
	var created DNSRecord
	if err := decode(raw, "dns record", &created); err != nil {
		return DNSRecord{}, err
	}
	return created, nil
}

func (c *Client) DeleteDNSRecord(zoneID, recordID string) error {
	path := "/zones/" + url.PathEscape(zoneID) + "/dns_records/" + url.PathEscape(recordID)
	_, err := c.doJSON(http.MethodDelete, path, nil)
	if hasStatus(err, http.StatusNotFound) {
		return nil
	}
	return err
}
