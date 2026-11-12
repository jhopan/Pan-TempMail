package cloudflare

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestZoneStatusReturnsZone(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[{"id":"zone-1","name":"contoh.id","status":"pending"}]}`))

	zone, err := client.ZoneStatus("contoh.id")
	if err != nil {
		t.Fatalf("ZoneStatus: %v", err)
	}
	if zone.ID != "zone-1" || zone.Name != "contoh.id" || zone.Status != "pending" {
		t.Fatalf("zone = %+v", zone)
	}
	if cap.path != "/zones" || cap.query != "name=contoh.id" {
		t.Fatalf("request = %s?%s", cap.path, cap.query)
	}
}

func TestZoneStatusMissingIsErrZoneNotFound(t *testing.T) {
	client, _ := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[]}`))

	_, err := client.ZoneStatus("hilang.id")
	if !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("err = %v, want ErrZoneNotFound", err)
	}
}

func TestListZones(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[
		{"id":"zone-1","name":"contoh.id","status":"active"},
		{"id":"zone-2","name":"contoh.com","status":"active"}]}`))

	zones, err := client.ListZones()
	if err != nil {
		t.Fatalf("ListZones: %v", err)
	}
	if len(zones) != 2 || zones[1].ID != "zone-2" || zones[1].Name != "contoh.com" {
		t.Fatalf("zones = %+v", zones)
	}
	if cap.path != "/zones" || cap.query != "per_page=50" {
		t.Fatalf("request = %s?%s", cap.path, cap.query)
	}
}

func TestFindZoneForHostLongestSuffix(t *testing.T) {
	zonesPayload := `{"success":true,"errors":[],"result":[
		{"id":"zone-base","name":"example.com","status":"active"},
		{"id":"zone-sub","name":"b.example.com","status":"active"},
		{"id":"zone-partial","name":"notexample.com","status":"active"}]}`
	client, _ := newStub(t, jsonStub(t, http.StatusOK, zonesPayload))

	zone, err := client.FindZoneForHost("a.b.example.com")
	if err != nil {
		t.Fatalf("FindZoneForHost: %v", err)
	}
	if zone.ID != "zone-sub" {
		t.Fatalf("zone = %+v, want the longest suffix zone b.example.com", zone)
	}

	zone, err = client.FindZoneForHost("example.com")
	if err != nil {
		t.Fatalf("FindZoneForHost base: %v", err)
	}
	if zone.ID != "zone-base" {
		t.Fatalf("zone = %+v, want example.com not notexample.com", zone)
	}
}

func TestFindZoneForHostNoMatch(t *testing.T) {
	client, _ := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[
		{"id":"zone-base","name":"example.com","status":"active"}]}`))

	for _, host := range []string{"example.org", "notexample.com", ""} {
		if _, err := client.FindZoneForHost(host); !errors.Is(err, ErrZoneNotFound) {
			t.Fatalf("FindZoneForHost(%q) err = %v, want ErrZoneNotFound", host, err)
		}
	}
}

func TestGetCatchAllWorkerTarget(t *testing.T) {
	payload := `{"success":true,"errors":[],"result":{"id":"rule-1","enabled":true,
		"matchers":[{"type":"all"}],
		"actions":[{"type":"worker","value":["tempmailbyjhopanstore"]}]}}`
	client, cap := newStub(t, jsonStub(t, http.StatusOK, payload))

	rule, err := client.GetCatchAll("zone-1")
	if err != nil {
		t.Fatalf("GetCatchAll: %v", err)
	}
	if !rule.Enabled || rule.Target != "tempmailbyjhopanstore" {
		t.Fatalf("rule = %+v", rule)
	}
	if cap.path != "/zones/zone-1/email/routing/rules/catch_all" {
		t.Fatalf("path = %s", cap.path)
	}
}

func TestGetCatchAllNonWorkerTargetIsEmpty(t *testing.T) {
	payload := `{"success":true,"errors":[],"result":{"id":"rule-1","enabled":false,
		"matchers":[{"type":"all"}],
		"actions":[{"type":"forward","value":["ops@contoh.id"]}]}}`
	client, _ := newStub(t, jsonStub(t, http.StatusOK, payload))

	rule, err := client.GetCatchAll("zone-1")
	if err != nil {
		t.Fatalf("GetCatchAll: %v", err)
	}
	if rule.Enabled || rule.Target != "" {
		t.Fatalf("rule = %+v, want disabled with no worker target", rule)
	}
}

func TestEmailRoutingDNSParsesMXPriority(t *testing.T) {
	payload := `{"success":true,"errors":[],"result":[
		{"type":"MX","name":"contoh.id","content":"route1.mx.cloudflare.net","priority":10,"ttl":1},
		{"type":"TXT","name":"_dmarc.contoh.id","content":"v=DMARC1; p=none","ttl":1}]}`
	client, cap := newStub(t, jsonStub(t, http.StatusOK, payload))

	records, err := client.EmailRoutingDNS("zone-1")
	if err != nil {
		t.Fatalf("EmailRoutingDNS: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v", records)
	}
	mx := records[0]
	if mx.Type != "MX" || mx.Name != "contoh.id" || mx.Content != "route1.mx.cloudflare.net" {
		t.Fatalf("mx = %+v", mx)
	}
	if mx.Priority == nil || *mx.Priority != 10 {
		t.Fatalf("mx priority = %v", mx.Priority)
	}
	if cap.path != "/zones/zone-1/email/routing/dns" {
		t.Fatalf("path = %s", cap.path)
	}
}

func TestListDNSRecordsFilters(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":[
		{"id":"rec-1","type":"MX","name":"contoh.id","content":"route1.mx.cloudflare.net","priority":1,"ttl":1}]}`))

	records, err := client.ListDNSRecords("zone-1", "MX", "contoh.id")
	if err != nil {
		t.Fatalf("ListDNSRecords: %v", err)
	}
	if len(records) != 1 || records[0].ID != "rec-1" {
		t.Fatalf("records = %+v", records)
	}
	if cap.path != "/zones/zone-1/dns_records" {
		t.Fatalf("path = %s", cap.path)
	}
	query, err := url.ParseQuery(cap.query)
	if err != nil {
		t.Fatalf("parse query %q: %v", cap.query, err)
	}
	if query.Get("type") != "MX" || query.Get("name") != "contoh.id" || query.Get("per_page") != "100" {
		t.Fatalf("query = %s", cap.query)
	}

	if _, err := client.ListDNSRecords("zone-1", "", ""); err != nil {
		t.Fatalf("ListDNSRecords without filters: %v", err)
	}
	query, err = url.ParseQuery(cap.query)
	if err != nil {
		t.Fatalf("parse query %q: %v", cap.query, err)
	}
	if _, ok := query["type"]; ok {
		t.Fatalf("empty type filter must be omitted, query = %s", cap.query)
	}
	if _, ok := query["name"]; ok {
		t.Fatalf("empty name filter must be omitted, query = %s", cap.query)
	}
	if query.Get("per_page") != "100" {
		t.Fatalf("query = %s", cap.query)
	}
}

func TestCreateDNSRecordBodyAndID(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusOK, `{"success":true,"errors":[],"result":
		{"id":"rec-9","type":"MX","name":"contoh.id","content":"route1.mx.cloudflare.net","priority":1,"ttl":1}}`))

	priority := 1
	created, err := client.CreateDNSRecord("zone-1", DNSRecord{
		Type:     "MX",
		Name:     "contoh.id",
		Content:  "route1.mx.cloudflare.net",
		TTL:      1,
		Priority: &priority,
	})
	if err != nil {
		t.Fatalf("CreateDNSRecord: %v", err)
	}
	if created.ID != "rec-9" {
		t.Fatalf("created = %+v", created)
	}
	if cap.method != http.MethodPost || cap.path != "/zones/zone-1/dns_records" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
	if !strings.Contains(string(cap.body), `"priority":1`) {
		t.Fatalf("body = %s, want priority", cap.body)
	}
	if strings.Contains(string(cap.body), `"id"`) {
		t.Fatalf("body = %s, want no id on create", cap.body)
	}
	var sent map[string]any
	if err := json.Unmarshal(cap.body, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["type"] != "MX" || sent["name"] != "contoh.id" || sent["content"] != "route1.mx.cloudflare.net" {
		t.Fatalf("body = %s", cap.body)
	}
	if sent["proxied"] != false || sent["ttl"] != float64(1) {
		t.Fatalf("body = %s", cap.body)
	}
}

func TestDeleteDNSRecordNotFoundIsNil(t *testing.T) {
	client, cap := newStub(t, jsonStub(t, http.StatusNotFound, `{"success":false,"errors":[{"code":81044,"message":"Record does not exist."}],"result":null}`))

	if err := client.DeleteDNSRecord("zone-1", "rec-1"); err != nil {
		t.Fatalf("DeleteDNSRecord: %v", err)
	}
	if cap.method != http.MethodDelete || cap.path != "/zones/zone-1/dns_records/rec-1" {
		t.Fatalf("request = %s %s", cap.method, cap.path)
	}
}

func TestDeleteDNSRecordFailurePropagates(t *testing.T) {
	client, _ := newStub(t, jsonStub(t, http.StatusForbidden, `{"success":false,"errors":[{"code":9109,"message":"Unauthorized"}],"result":null}`))

	if err := client.DeleteDNSRecord("zone-1", "rec-1"); err == nil {
		t.Fatal("DeleteDNSRecord err = nil, want API error")
	}
}
