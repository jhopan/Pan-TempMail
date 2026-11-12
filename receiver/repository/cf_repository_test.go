package repository

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"receiver/database"
)

// newCFTestDB opens a migrated temp SQLite database through the package connection helper.
func newCFTestDB(t *testing.T) *sql.DB {
	t.Helper()
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "t.db"))
	if err := database.Connect(); err != nil {
		t.Fatalf("connect temp database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close temp database: %v", err)
		}
	})
	return database.Conn()
}

func TestMigrationCreatesCFTables(t *testing.T) {
	db := newCFTestDB(t)

	for _, name := range []string{"cf_profiles", "cf_domains"} {
		var got string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&got)
		if err != nil {
			t.Fatalf("table %s not created by migration: %v", name, err)
		}
	}

	var idx string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, "idx_cf_domains_profile").Scan(&idx); err != nil {
		t.Fatalf("index idx_cf_domains_profile not created by migration: %v", err)
	}
}

func TestCFSettingConstants(t *testing.T) {
	want := map[string]string{
		SettingActiveProfile: "cf_active_profile",
		SettingTunnelWebHost: "cf_tunnel_web_host",
		SettingTunnelName:    "cf_tunnel_name",
		SettingCopies:        "cf_copies",
	}
	for got, expect := range want {
		if got != expect {
			t.Errorf("setting constant = %q, want %q", got, expect)
		}
	}
}

func TestCFProfileCRUD(t *testing.T) {
	db := newCFTestDB(t)

	if err := CreateCFProfile(db, CFProfile{ID: "acc1", Label: "Personal", AccountID: "acct-123"}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := CreateCFProfile(db, CFProfile{ID: "acc2", Label: "Work", AccountID: "acct-999"}); err != nil {
		t.Fatalf("create second profile: %v", err)
	}

	got, err := GetCFProfile(db, "acc1")
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if got.Label != "Personal" || got.AccountID != "acct-123" {
		t.Errorf("profile meta = %q/%q, want Personal/acct-123", got.Label, got.AccountID)
	}
	if got.WorkerDeployed {
		t.Error("fresh profile should not be marked worker deployed")
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not parsed: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}

	list, err := ListCFProfiles(db)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("listed %d profiles, want 2", len(list))
	}

	if err := UpdateCFProfileMeta(db, "acc1", "Home", "acct-555"); err != nil {
		t.Fatalf("update meta: %v", err)
	}
	got, err = GetCFProfile(db, "acc1")
	if err != nil {
		t.Fatalf("get profile after meta update: %v", err)
	}
	if got.Label != "Home" || got.AccountID != "acct-555" {
		t.Errorf("profile meta after update = %q/%q, want Home/acct-555", got.Label, got.AccountID)
	}

	if err := SetCFProfileToken(db, "acc1", "v1:sealed-token"); err != nil {
		t.Fatalf("set token: %v", err)
	}
	token, err := GetCFProfileToken(db, "acc1")
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	if token != "v1:sealed-token" {
		t.Errorf("token = %q, want v1:sealed-token", token)
	}

	if err := SetCFProfileWorkerDeployed(db, "acc1", true); err != nil {
		t.Fatalf("set worker deployed: %v", err)
	}
	got, err = GetCFProfile(db, "acc1")
	if err != nil {
		t.Fatalf("get profile after deploy flag: %v", err)
	}
	if !got.WorkerDeployed {
		t.Error("worker_deployed flag not persisted")
	}
	if err := SetCFProfileWorkerDeployed(db, "acc1", false); err != nil {
		t.Fatalf("clear worker deployed: %v", err)
	}
	got, err = GetCFProfile(db, "acc1")
	if err != nil {
		t.Fatalf("get profile after clearing deploy flag: %v", err)
	}
	if got.WorkerDeployed {
		t.Error("worker_deployed flag not cleared")
	}

	if err := DeleteCFProfile(db, "acc1"); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if _, err := GetCFProfile(db, "acc1"); !errors.Is(err, ErrCFNotFound) {
		t.Errorf("get deleted profile err = %v, want ErrCFNotFound", err)
	}
	list, err = ListCFProfiles(db)
	if err != nil {
		t.Fatalf("list profiles after delete: %v", err)
	}
	if len(list) != 1 || list[0].ID != "acc2" {
		t.Errorf("profiles after delete = %+v, want only acc2", list)
	}
}

func TestCFNotFound(t *testing.T) {
	db := newCFTestDB(t)

	if _, err := GetCFProfile(db, "missing"); !errors.Is(err, ErrCFNotFound) {
		t.Errorf("GetCFProfile err = %v, want ErrCFNotFound", err)
	}
	if _, err := GetCFProfileToken(db, "missing"); !errors.Is(err, ErrCFNotFound) {
		t.Errorf("GetCFProfileToken err = %v, want ErrCFNotFound", err)
	}
	if _, err := GetCFDomain(db, "missing.example"); !errors.Is(err, ErrCFNotFound) {
		t.Errorf("GetCFDomain err = %v, want ErrCFNotFound", err)
	}
}

func TestUpsertCFDomainIdempotent(t *testing.T) {
	db := newCFTestDB(t)
	if err := CreateCFProfile(db, CFProfile{ID: "p1", Label: "Main", AccountID: "acct-1"}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	d := CFDomain{Domain: "alpha.example", ProfileID: "p1", ZoneID: "zone-alpha", Enabled: true, Routing: true}
	if err := UpsertCFDomain(db, d); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := UpsertCFDomain(db, d); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	domains, err := ListCFDomains(db)
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	if len(domains) != 1 {
		t.Fatalf("upsert produced %d rows, want 1", len(domains))
	}

	// A conflicting upsert rewrites the whole row.
	if err := UpsertCFDomain(db, CFDomain{Domain: "alpha.example", ProfileID: "p1", ZoneID: "zone-alpha-2", Enabled: false, Routing: false}); err != nil {
		t.Fatalf("third upsert: %v", err)
	}
	got, err := GetCFDomain(db, "alpha.example")
	if err != nil {
		t.Fatalf("get domain: %v", err)
	}
	if got.ZoneID != "zone-alpha-2" {
		t.Errorf("zone_id = %q, want zone-alpha-2", got.ZoneID)
	}
	if got.Enabled || got.Routing {
		t.Errorf("enabled/routing = %v/%v, want false/false from struct", got.Enabled, got.Routing)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not parsed: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestCFDomainRoutingToggleAndDelete(t *testing.T) {
	db := newCFTestDB(t)
	if err := CreateCFProfile(db, CFProfile{ID: "p1", Label: "Main", AccountID: "acct-1"}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := UpsertCFDomain(db, CFDomain{Domain: "alpha.example", ProfileID: "p1", ZoneID: "z1", Enabled: true}); err != nil {
		t.Fatalf("upsert domain: %v", err)
	}

	if err := SetCFDomainRouting(db, "alpha.example", true); err != nil {
		t.Fatalf("enable routing: %v", err)
	}
	got, err := GetCFDomain(db, "alpha.example")
	if err != nil {
		t.Fatalf("get domain: %v", err)
	}
	if !got.Routing {
		t.Error("routing not enabled")
	}

	if err := SetCFDomainRouting(db, "alpha.example", false); err != nil {
		t.Fatalf("disable routing: %v", err)
	}
	got, err = GetCFDomain(db, "alpha.example")
	if err != nil {
		t.Fatalf("get domain after disable: %v", err)
	}
	if got.Routing {
		t.Error("routing not disabled")
	}

	if err := SetCFDomainRouting(db, "missing.example", true); !errors.Is(err, ErrCFNotFound) {
		t.Errorf("routing on missing domain err = %v, want ErrCFNotFound", err)
	}

	if err := DeleteCFDomain(db, "alpha.example"); err != nil {
		t.Fatalf("delete domain: %v", err)
	}
	if _, err := GetCFDomain(db, "alpha.example"); !errors.Is(err, ErrCFNotFound) {
		t.Errorf("get deleted domain err = %v, want ErrCFNotFound", err)
	}
	domains, err := ListCFDomains(db)
	if err != nil {
		t.Fatalf("list domains after delete: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("domains after delete = %+v, want none", domains)
	}
}

func TestCountCFDomainsForProfile(t *testing.T) {
	db := newCFTestDB(t)
	for _, id := range []string{"p1", "p2"} {
		if err := CreateCFProfile(db, CFProfile{ID: id, Label: id, AccountID: "acct-" + id}); err != nil {
			t.Fatalf("create profile %s: %v", id, err)
		}
	}
	for _, d := range []CFDomain{
		{Domain: "a.example", ProfileID: "p1"},
		{Domain: "b.example", ProfileID: "p1"},
		{Domain: "c.example", ProfileID: "p2"},
	} {
		if err := UpsertCFDomain(db, d); err != nil {
			t.Fatalf("upsert %s: %v", d.Domain, err)
		}
	}

	n, err := CountCFDomainsForProfile(db, "p1")
	if err != nil {
		t.Fatalf("count p1: %v", err)
	}
	if n != 2 {
		t.Errorf("count for p1 = %d, want 2", n)
	}
	n, err = CountCFDomainsForProfile(db, "p2")
	if err != nil {
		t.Fatalf("count p2: %v", err)
	}
	if n != 1 {
		t.Errorf("count for p2 = %d, want 1", n)
	}
	n, err = CountCFDomainsForProfile(db, "nobody")
	if err != nil {
		t.Fatalf("count unknown profile: %v", err)
	}
	if n != 0 {
		t.Errorf("count for unknown profile = %d, want 0", n)
	}
}

func TestListedDomainsFiltersAndSorts(t *testing.T) {
	db := newCFTestDB(t)
	if err := CreateCFProfile(db, CFProfile{ID: "p1", Label: "Main", AccountID: "acct-1"}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	rows := []CFDomain{
		{Domain: "zeta.example", ProfileID: "p1", Enabled: true, Routing: true},
		{Domain: "Alpha.example", ProfileID: "p1", Enabled: true, Routing: true},
		{Domain: "beta.example", ProfileID: "p1", Enabled: true, Routing: true},
		{Domain: "not-routed.example", ProfileID: "p1", Enabled: true, Routing: false},
		{Domain: "not-enabled.example", ProfileID: "p1", Enabled: false, Routing: true},
		{Domain: "off.example", ProfileID: "p1", Enabled: false, Routing: false},
	}
	for _, d := range rows {
		if err := UpsertCFDomain(db, d); err != nil {
			t.Fatalf("upsert %s: %v", d.Domain, err)
		}
	}

	listed, err := ListedDomains(db)
	if err != nil {
		t.Fatalf("listed domains: %v", err)
	}
	want := []string{"Alpha.example", "beta.example", "zeta.example"}
	if len(listed) != len(want) {
		t.Fatalf("listed = %v, want %v", listed, want)
	}
	for i, w := range want {
		if listed[i] != w {
			t.Fatalf("listed = %v, want %v", listed, want)
		}
	}

	if err := SetCFDomainRouting(db, "not-routed.example", true); err != nil {
		t.Fatalf("enable routing: %v", err)
	}
	listed, err = ListedDomains(db)
	if err != nil {
		t.Fatalf("listed domains after toggle: %v", err)
	}
	if len(listed) != 4 {
		t.Errorf("listed after toggle = %v, want 4 entries", listed)
	}
}
