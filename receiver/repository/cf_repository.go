package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrCFNotFound is returned when a Cloudflare profile or domain row does not exist.
var ErrCFNotFound = errors.New("cf record not found")

// Settings keys used by the Cloudflare control panel.
const (
	SettingActiveProfile = "cf_active_profile"
	SettingTunnelWebHost = "cf_tunnel_web_host"
	SettingTunnelName    = "cf_tunnel_name"
	SettingCopies        = "cf_copies"
)

// CFProfile is a stored Cloudflare account credential plus worker state.
type CFProfile struct {
	ID             string
	Label          string
	AccountID      string
	WorkerDeployed bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CFDomain is a stored Cloudflare zone the temp-mail instance serves.
type CFDomain struct {
	Domain    string
	ProfileID string
	ZoneID    string
	Enabled   bool
	Routing   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

const cfProfileColumns = `id, label, account_id, worker_deployed, created_at, updated_at`
const cfDomainColumns = `domain, profile_id, zone_id, enabled, routing, created_at, updated_at`

type cfScanner interface {
	Scan(dest ...any) error
}

func cfNow() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func parseCFTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanCFProfile(sc cfScanner) (CFProfile, error) {
	var p CFProfile
	var deployed int
	var created, updated string
	if err := sc.Scan(&p.ID, &p.Label, &p.AccountID, &deployed, &created, &updated); err != nil {
		return CFProfile{}, err
	}
	p.WorkerDeployed = deployed != 0
	p.CreatedAt = parseCFTime(created)
	p.UpdatedAt = parseCFTime(updated)
	return p, nil
}

func scanCFDomain(sc cfScanner) (CFDomain, error) {
	var d CFDomain
	var enabled, routing int
	var created, updated string
	if err := sc.Scan(&d.Domain, &d.ProfileID, &d.ZoneID, &enabled, &routing, &created, &updated); err != nil {
		return CFDomain{}, err
	}
	d.Enabled = enabled != 0
	d.Routing = routing != 0
	d.CreatedAt = parseCFTime(created)
	d.UpdatedAt = parseCFTime(updated)
	return d, nil
}

// cfAffected turns an update that matched no row into ErrCFNotFound so callers can answer 404.
func cfAffected(res sql.Result, err error, op string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if n == 0 {
		return fmt.Errorf("%s: %w", op, ErrCFNotFound)
	}
	return nil
}

// ListCFProfiles returns every stored Cloudflare profile.
func ListCFProfiles(db *sql.DB) ([]CFProfile, error) {
	rows, err := db.QueryContext(context.Background(), placeholder(`SELECT `+cfProfileColumns+` FROM cf_profiles ORDER BY created_at, id`))
	if err != nil {
		return nil, fmt.Errorf("list cf profiles: %w", err)
	}
	defer rows.Close()

	profiles := []CFProfile{}
	for rows.Next() {
		p, err := scanCFProfile(rows)
		if err != nil {
			return nil, fmt.Errorf("list cf profiles: %w", err)
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cf profiles: %w", err)
	}
	return profiles, nil
}

// GetCFProfile returns one profile by ID.
func GetCFProfile(db *sql.DB, id string) (CFProfile, error) {
	row := db.QueryRowContext(context.Background(), placeholder(`SELECT `+cfProfileColumns+` FROM cf_profiles WHERE id = $1`), id)
	p, err := scanCFProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CFProfile{}, ErrCFNotFound
	}
	if err != nil {
		return CFProfile{}, fmt.Errorf("get cf profile: %w", err)
	}
	return p, nil
}

// CreateCFProfile inserts a profile with an empty token.
func CreateCFProfile(db *sql.DB, p CFProfile) error {
	now := cfNow()
	_, err := db.ExecContext(context.Background(), placeholder(`INSERT INTO cf_profiles (id, label, account_id, token, worker_deployed, created_at, updated_at)
		VALUES ($1, $2, $3, '', $4, $5, $6)`),
		p.ID, p.Label, p.AccountID, boolToInt(p.WorkerDeployed), now, now)
	if err != nil {
		return fmt.Errorf("create cf profile: %w", err)
	}
	return nil
}

// UpdateCFProfileMeta updates the label and account id of a profile.
func UpdateCFProfileMeta(db *sql.DB, id, label, accountID string) error {
	res, err := db.ExecContext(context.Background(), placeholder(`UPDATE cf_profiles SET label = $1, account_id = $2, updated_at = $3 WHERE id = $4`),
		label, accountID, cfNow(), id)
	return cfAffected(res, err, "update cf profile meta")
}

// SetCFProfileToken stores the sealed API token for a profile.
func SetCFProfileToken(db *sql.DB, id, sealed string) error {
	res, err := db.ExecContext(context.Background(), placeholder(`UPDATE cf_profiles SET token = $1, updated_at = $2 WHERE id = $3`),
		sealed, cfNow(), id)
	return cfAffected(res, err, "set cf profile token")
}

// GetCFProfileToken returns the sealed API token for a profile.
func GetCFProfileToken(db *sql.DB, id string) (string, error) {
	var token string
	err := db.QueryRowContext(context.Background(), placeholder(`SELECT token FROM cf_profiles WHERE id = $1`), id).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrCFNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get cf profile token: %w", err)
	}
	return token, nil
}

// SetCFProfileWorkerDeployed records whether the worker script is deployed for a profile.
func SetCFProfileWorkerDeployed(db *sql.DB, id string, deployed bool) error {
	res, err := db.ExecContext(context.Background(), placeholder(`UPDATE cf_profiles SET worker_deployed = $1, updated_at = $2 WHERE id = $3`),
		boolToInt(deployed), cfNow(), id)
	return cfAffected(res, err, "set cf profile worker deployed")
}

// DeleteCFProfile removes a profile; deleting an absent profile is not an error.
func DeleteCFProfile(db *sql.DB, id string) error {
	if _, err := db.ExecContext(context.Background(), placeholder(`DELETE FROM cf_profiles WHERE id = $1`), id); err != nil {
		return fmt.Errorf("delete cf profile: %w", err)
	}
	return nil
}

// CountCFDomainsForProfile counts the domains attached to a profile.
func CountCFDomainsForProfile(db *sql.DB, id string) (int, error) {
	var n int
	if err := db.QueryRowContext(context.Background(), placeholder(`SELECT COUNT(*) FROM cf_domains WHERE profile_id = $1`), id).Scan(&n); err != nil {
		return 0, fmt.Errorf("count cf domains: %w", err)
	}
	return n, nil
}

// ListCFDomains returns every stored domain.
func ListCFDomains(db *sql.DB) ([]CFDomain, error) {
	rows, err := db.QueryContext(context.Background(), placeholder(`SELECT `+cfDomainColumns+` FROM cf_domains ORDER BY domain`))
	if err != nil {
		return nil, fmt.Errorf("list cf domains: %w", err)
	}
	defer rows.Close()

	domains := []CFDomain{}
	for rows.Next() {
		d, err := scanCFDomain(rows)
		if err != nil {
			return nil, fmt.Errorf("list cf domains: %w", err)
		}
		domains = append(domains, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list cf domains: %w", err)
	}
	return domains, nil
}

// GetCFDomain returns one domain by name.
func GetCFDomain(db *sql.DB, domain string) (CFDomain, error) {
	row := db.QueryRowContext(context.Background(), placeholder(`SELECT `+cfDomainColumns+` FROM cf_domains WHERE domain = $1`), domain)
	d, err := scanCFDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CFDomain{}, ErrCFNotFound
	}
	if err != nil {
		return CFDomain{}, fmt.Errorf("get cf domain: %w", err)
	}
	return d, nil
}

// UpsertCFDomain writes profile_id, zone_id, enabled and routing exactly as passed.
func UpsertCFDomain(db *sql.DB, d CFDomain) error {
	now := cfNow()
	_, err := db.ExecContext(context.Background(), placeholder(`INSERT INTO cf_domains (domain, profile_id, zone_id, enabled, routing, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT(domain) DO UPDATE SET
			profile_id = excluded.profile_id,
			zone_id = excluded.zone_id,
			enabled = excluded.enabled,
			routing = excluded.routing,
			updated_at = excluded.updated_at`),
		d.Domain, d.ProfileID, d.ZoneID, boolToInt(d.Enabled), boolToInt(d.Routing), now, now)
	if err != nil {
		return fmt.Errorf("upsert cf domain: %w", err)
	}
	return nil
}

// SetCFDomainRouting toggles email routing for a domain.
func SetCFDomainRouting(db *sql.DB, domain string, routing bool) error {
	res, err := db.ExecContext(context.Background(), placeholder(`UPDATE cf_domains SET routing = $1, updated_at = $2 WHERE domain = $3`),
		boolToInt(routing), cfNow(), domain)
	return cfAffected(res, err, "set cf domain routing")
}

// DeleteCFDomain removes a domain; deleting an absent domain is not an error.
func DeleteCFDomain(db *sql.DB, domain string) error {
	if _, err := db.ExecContext(context.Background(), placeholder(`DELETE FROM cf_domains WHERE domain = $1`), domain); err != nil {
		return fmt.Errorf("delete cf domain: %w", err)
	}
	return nil
}

// ListedDomains returns the domains served by the mail worker, alphabetically.
func ListedDomains(db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(context.Background(), placeholder(`SELECT domain FROM cf_domains WHERE enabled = 1 AND routing = 1 ORDER BY domain`))
	if err != nil {
		return nil, fmt.Errorf("listed domains: %w", err)
	}
	defer rows.Close()

	domains := []string{}
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, fmt.Errorf("listed domains: %w", err)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listed domains: %w", err)
	}
	return domains, nil
}
