package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"receiver/cloudflare"
	"receiver/repository"
	"receiver/worker"

	"github.com/gin-gonic/gin"
)

// Issue levels of a domain check. "ok" is never emitted: the issue list only carries what the
// operator still has to look at.
const (
	cfLevelWarn = "warn"
	cfLevelBad  = "bad"
)

// Issue keys of a domain check.
const (
	cfIssueZone              = "zone"
	cfIssueProfile           = "profile"
	cfIssueWorker            = "worker"
	cfIssueRouting           = "routing"
	cfIssueEmailDNS          = "email_dns"
	cfIssueMXConflict        = "mx_conflict"
	cfIssueCNAME             = "cname"
	cfIssueWebHostConfigured = "web_host_configured"
)

// Repair keys of a domain fix.
const (
	cfFixWorker   = "worker"
	cfFixRouting  = "routing"
	cfFixEmailDNS = "email_dns"
	cfFixCNAME    = "cname"
)

// cfDNSRecordView is a DNS record as the panel renders it: no Cloudflare record id, and no
// priority for anything that is not an MX record.
type cfDNSRecordView struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Proxied  bool   `json:"proxied,omitempty"`
	Priority *int   `json:"priority,omitempty"`
}

type cfIssueView struct {
	Key     string `json:"key"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Fixable bool   `json:"fixable"`
}

type cfWorkerCheckView struct {
	Name     string `json:"name"`
	Deployed bool   `json:"deployed"`
}

type cfRoutingCheckView struct {
	Enabled bool   `json:"enabled"`
	Target  string `json:"target"`
	OK      bool   `json:"ok"`
}

type cfEmailDNSCheckView struct {
	Ready     bool              `json:"ready"`
	Missing   []cfDNSRecordView `json:"missing"`
	Conflicts []cfDNSRecordView `json:"conflicts"`
}

type cfWebHostCheckView struct {
	Configured bool              `json:"configured"`
	Host       string            `json:"host"`
	Expected   string            `json:"expected"`
	Zone       cloudflare.Zone   `json:"zone"`
	Records    []cfDNSRecordView `json:"records"`
	OK         bool              `json:"ok"`
}

type cfDomainCheckView struct {
	Domain   string              `json:"domain"`
	Profile  string              `json:"profile"`
	Zone     cloudflare.Zone     `json:"zone"`
	Worker   cfWorkerCheckView   `json:"worker"`
	Routing  cfRoutingCheckView  `json:"routing"`
	EmailDNS cfEmailDNSCheckView `json:"email_dns"`
	WebHost  cfWebHostCheckView  `json:"web_host"`
	Issues   []cfIssueView       `json:"issues"`
}

// cfFixStepView is one line of a fix report. OK is set on done entries only and Detail on
// failed entries only, so the three arrays keep their contracted shape.
type cfFixStepView struct {
	Key     string `json:"key"`
	OK      bool   `json:"ok,omitempty"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type cfDomainFixView struct {
	Domain  string          `json:"domain"`
	Done    []cfFixStepView `json:"done"`
	Skipped []cfFixStepView `json:"skipped"`
	Failed  []cfFixStepView `json:"failed"`
}

func cfDNSRecordViewOf(rec cloudflare.DNSRecord) cfDNSRecordView {
	return cfDNSRecordView{
		Type:     rec.Type,
		Name:     rec.Name,
		Content:  rec.Content,
		Proxied:  rec.Proxied,
		Priority: rec.Priority,
	}
}

func cfFixDone(report *cfDomainFixView, key, message string) {
	report.Done = append(report.Done, cfFixStepView{Key: key, OK: true, Message: message})
}

func cfFixSkipped(report *cfDomainFixView, key, message string) {
	report.Skipped = append(report.Skipped, cfFixStepView{Key: key, Message: message})
}

func cfFixFailed(report *cfDomainFixView, key, message string, cause error) {
	detail := ""
	if cause != nil {
		detail = cause.Error()
	}
	report.Failed = append(report.Failed, cfFixStepView{Key: key, Message: message, Detail: detail})
}

// cfHostSuffix reports whether host is suffix itself or sits below it on a label boundary.
func cfHostSuffix(host, suffix string) bool {
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

// cfRecordNameMatches compares a DNS record name with a host, ignoring a trailing dot and case.
func cfRecordNameMatches(recordName, host string) bool {
	return strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(recordName), "."), strings.TrimSpace(host))
}

// cfProfileForHost returns the profile owning the longest registered domain that is a suffix of
// host, so a web host living under a registered domain is managed from that domain's account.
func cfProfileForHost(db *sql.DB, host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	domains, err := repository.ListCFDomains(db)
	if err != nil {
		return ""
	}
	best := ""
	bestLabels := 0
	for _, d := range domains {
		name := strings.ToLower(strings.TrimSpace(d.Domain))
		if name == "" || d.ProfileID == "" || !cfHostSuffix(host, name) {
			continue
		}
		if labels := strings.Count(name, ".") + 1; labels > bestLabels {
			best, bestLabels = d.ProfileID, labels
		}
	}
	return best
}

// cfClientForHost returns the client that can manage the zone of host: the profile owning the
// host when the host sits under a registered domain, otherwise the fallback profile, otherwise
// the active profile.
func cfClientForHost(db *sql.DB, host, fallbackID string) (*cloudflare.Client, error) {
	candidates := make([]string, 0, 3)
	if owner := cfProfileForHost(db, host); owner != "" {
		candidates = append(candidates, owner)
	}
	if fallbackID != "" {
		candidates = append(candidates, fallbackID)
	}
	candidates = append(candidates, "")
	var firstErr error
	for _, id := range candidates {
		_, client, err := cfResolve(db, id)
		if err == nil {
			return client, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// cfWorkerDeployed reads the stored deploy state; a profile that cannot be read counts as
// undeployed rather than as a hard failure of the check.
func cfWorkerDeployed(db *sql.DB, profileID string) bool {
	if profileID == "" {
		return false
	}
	profile, err := repository.GetCFProfile(db, profileID)
	if err != nil {
		return false
	}
	return profile.WorkerDeployed
}

// cfZoneIDForFix resolves the zone a repair writes to: the stored zone id wins, a domain
// imported without one falls back to a zone lookup.
func cfZoneIDForFix(record repository.CFDomain, client *cloudflare.Client) (string, error) {
	if record.ZoneID != "" {
		return record.ZoneID, nil
	}
	zone, err := client.ZoneStatus(record.Domain)
	if err != nil {
		return "", err
	}
	if zone.ID == "" {
		return "", errors.New("Cloudflare tidak mengembalikan ID zone untuk " + record.Domain)
	}
	return zone.ID, nil
}

// cfMissingDNSRecords lists the records Cloudflare requires for Email Routing that the zone
// does not have yet. The comparison ignores record ids, so a repaired zone stops being missing.
func cfMissingDNSRecords(required, existing []cloudflare.DNSRecord) []cloudflare.DNSRecord {
	missing := []cloudflare.DNSRecord{}
	for _, want := range required {
		found := false
		for _, have := range existing {
			sameName := strings.EqualFold(strings.TrimSuffix(have.Name, "."), strings.TrimSuffix(want.Name, "."))
			if strings.EqualFold(have.Type, want.Type) && sameName && strings.EqualFold(have.Content, want.Content) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, want)
		}
	}
	return missing
}

// cfMXConflicts lists existing MX records that are not one of the MX records Cloudflare wants.
// A foreign MX is reported, never rewritten: mail delivery is the operator's decision.
func cfMXConflicts(required, existing []cloudflare.DNSRecord) []cloudflare.DNSRecord {
	wanted := map[string]bool{}
	for _, rec := range required {
		if strings.EqualFold(rec.Type, "MX") {
			wanted[strings.ToLower(strings.TrimSpace(rec.Content))] = true
		}
	}
	conflicts := []cloudflare.DNSRecord{}
	for _, rec := range existing {
		if !strings.EqualFold(rec.Type, "MX") {
			continue
		}
		if wanted[strings.ToLower(strings.TrimSpace(rec.Content))] {
			continue
		}
		conflicts = append(conflicts, rec)
	}
	return conflicts
}

// cfWorkerBindings builds the secret bindings and their names for a Worker deploy. Both
// CFDeployWorker and the repair path use it, so a deploy can never drift between them.
func cfWorkerBindings(db *sql.DB, webHost string) ([]cloudflare.Binding, []string, error) {
	apiKey := strings.TrimSpace(os.Getenv("EMAIL_API_KEY"))
	if apiKey == "" {
		return nil, nil, errors.New("EMAIL_API_KEY belum diset di server")
	}
	if webHost == "" {
		return nil, nil, errors.New("Web host tunnel belum diset, isi dulu di bagian Tunnel")
	}
	bindings := []cloudflare.Binding{
		{Name: "TEMPMAIL_SERVER_URL", Type: "secret_text", Text: "https://" + webHost},
		{Name: "TEMPMAIL_API_KEY", Type: "secret_text", Text: apiKey},
		{Name: "FORWARD_LIST", Type: "secret_text", Text: strings.Join(cfConfiguredCopies(db), ",")},
	}
	secrets := []string{"TEMPMAIL_SERVER_URL", "TEMPMAIL_API_KEY", "FORWARD_LIST"}
	if signingSecret := strings.TrimSpace(os.Getenv("WORKER_SIGNING_SECRET")); signingSecret != "" {
		bindings = append(bindings, cloudflare.Binding{Name: "TEMPMAIL_SIGNING_SECRET", Type: "secret_text", Text: signingSecret})
		secrets = append(secrets, "TEMPMAIL_SIGNING_SECRET")
	}
	return bindings, secrets, nil
}

// cfDeployWorkerFor uploads the shared Worker script for one profile and records the deploy
// state, reusing the same bindings builder as CFDeployWorker.
func cfDeployWorkerFor(db *sql.DB, profile repository.CFProfile, client *cloudflare.Client) error {
	_, _, webHost := cfTunnelSettings(db)
	bindings, _, err := cfWorkerBindings(db, webHost)
	if err != nil {
		return err
	}
	if _, err := client.DeployWorker(profile.AccountID, worker.Name(), worker.CompatDate(), worker.Script(), bindings); err != nil {
		return err
	}
	if err := client.SetWorkerSubdomain(profile.AccountID, worker.Name(), false, false); err != nil {
		return err
	}
	return repository.SetCFProfileWorkerDeployed(db, profile.ID, true)
}

// CFTunnelCNAME publishes the web host on the tunnel: it reports the zone that owns the host
// and creates the proxied CNAME when the host has none yet. An existing record for the host is
// reported as-is and never rewritten here, because this endpoint carries no overwrite switch.
func CFTunnelCNAME(c *gin.Context) {
	db := cfDB()
	_, tunnelID, webHost := cfTunnelSettings(db)
	if webHost == "" {
		cfFail(c, http.StatusBadRequest, "Web host tunnel belum diset, isi dulu di bagian Tunnel", nil)
		return
	}
	if tunnelID == "" {
		cfFail(c, http.StatusBadRequest, "ID tunnel belum ada, buat atau isi ID tunnel dulu di bagian Tunnel", nil)
		return
	}
	client, err := cfClientForHost(db, webHost, "")
	if err != nil {
		cfFail(c, http.StatusBadRequest, "Profil Cloudflare untuk zone "+webHost+" tidak siap dipakai (butuh API token)", err)
		return
	}
	expected := tunnelID + ".cfargotunnel.com"
	zone, err := client.FindZoneForHost(webHost)
	if err != nil {
		cfFail(c, http.StatusBadRequest, "Zone "+webHost+" tidak ada di akun Cloudflare profil ini", err)
		return
	}
	records, err := client.ListDNSRecords(zone.ID, "CNAME", webHost)
	if err != nil {
		cfFail(c, http.StatusBadGateway, "Cloudflare menolak pembacaan DNS zone "+zone.Name, err)
		return
	}
	existing := false
	for _, rec := range records {
		if strings.EqualFold(rec.Type, "CNAME") && cfRecordNameMatches(rec.Name, webHost) {
			existing = true
			break
		}
	}
	created := false
	if !existing {
		record := cloudflare.DNSRecord{Type: "CNAME", Name: webHost, Content: expected, Proxied: true, TTL: 1}
		if _, err := client.CreateDNSRecord(zone.ID, record); err != nil {
			cfFail(c, http.StatusBadGateway, "Cloudflare menolak pembuatan CNAME "+webHost, err)
			return
		}
		created = true
	}
	c.JSON(http.StatusOK, gin.H{
		"web_host": webHost,
		"expected": expected,
		"created":  created,
		"existing": existing,
		"zone":     gin.H{"id": zone.ID, "name": zone.Name},
	})
}

// CFDomainCheck reports everything that keeps one registered domain from serving mail: its
// zone, the Worker deploy state, the catch-all routing rule, the Email Routing DNS records and
// the tunnel CNAME that publishes the web panel. It never writes to Cloudflare or to the
// database, and a failing Cloudflare call becomes a "bad" issue instead of a 5xx, so the panel
// can still render the state that could be read.
func CFDomainCheck(c *gin.Context) {
	db := cfDB()
	domain := strings.ToLower(strings.TrimSpace(c.Param("domain")))
	record, err := repository.GetCFDomain(db, domain)
	if err != nil {
		cfFail(c, http.StatusNotFound, "Domain tidak terdaftar", err)
		return
	}
	view := cfDomainCheckView{
		Domain:  record.Domain,
		Profile: record.ProfileID,
		Worker:  cfWorkerCheckView{Name: worker.Name()},
		EmailDNS: cfEmailDNSCheckView{
			Missing:   []cfDNSRecordView{},
			Conflicts: []cfDNSRecordView{},
		},
		WebHost: cfWebHostCheckView{Records: []cfDNSRecordView{}},
		Issues:  []cfIssueView{},
	}
	view.Worker.Deployed = cfWorkerDeployed(db, record.ProfileID)

	profile, client, resolveErr := cfResolve(db, record.ProfileID)
	if resolveErr != nil {
		// Without a working token every Cloudflare check below is unreadable; the missing
		// credential is the one thing the operator has to fix first.
		view.Issues = append(view.Issues, cfIssueView{
			Key:     cfIssueProfile,
			Level:   cfLevelBad,
			Fixable: false,
			Message: "Profil " + record.ProfileID + " tidak siap dipakai: " + resolveErr.Error(),
		})
	} else {
		view.Profile = profile.ID
		cfProbeWorker(profile, client, &view)
		view.Issues = append(view.Issues, cfZoneIssues(record, client, &view)...)
	}
	view.Issues = append(view.Issues, cfWebHostIssues(db, record, &view)...)
	c.JSON(http.StatusOK, view)
}

// cfProbeWorker replaces the stored deploy flag with the live answer from Cloudflare, so a Worker
// deleted outside the panel can no longer be reported as ready. A failed probe keeps the stored
// value and is reported: a token without Workers Scripts read access cannot see the script.
func cfProbeWorker(profile repository.CFProfile, client *cloudflare.Client, view *cfDomainCheckView) {
	stored := view.Worker.Deployed
	exists, err := client.WorkerScript(profile.AccountID, worker.Name())
	if err != nil {
		view.Issues = append(view.Issues, cfIssueView{
			Key:     cfIssueWorker,
			Level:   cfLevelWarn,
			Fixable: false,
			Message: "Keberadaan Worker di Cloudflare tidak bisa dibaca, status dari database dipakai: " + err.Error(),
		})
		return
	}
	view.Worker.Deployed = exists
	if exists && !stored {
		view.Issues = append(view.Issues, cfIssueView{
			Key:     cfIssueWorker,
			Level:   cfLevelWarn,
			Fixable: true,
			Message: "Worker " + worker.Name() + " sudah ada di Cloudflare tetapi belum tercatat di database, jalankan Rapikan untuk menyamakan",
		})
	}
}

// cfZoneIssues fills the zone, Worker, routing and Email Routing DNS parts of a check report
// and returns the issues they carry.
func cfZoneIssues(record repository.CFDomain, client *cloudflare.Client, view *cfDomainCheckView) []cfIssueView {
	issues := []cfIssueView{}
	zoneID := record.ZoneID
	if zone, err := client.ZoneStatus(record.Domain); err != nil {
		issues = append(issues, cfIssueView{
			Key:     cfIssueZone,
			Level:   cfLevelBad,
			Fixable: false,
			Message: "Zone " + record.Domain + " tidak bisa dibaca dari Cloudflare: " + err.Error(),
		})
	} else {
		view.Zone = zone
		if zone.ID != "" {
			zoneID = zone.ID
		}
	}
	if !view.Worker.Deployed {
		issues = append(issues, cfIssueView{
			Key:     cfIssueWorker,
			Level:   cfLevelBad,
			Fixable: true,
			Message: "Worker " + worker.Name() + " belum di-deploy untuk profil ini",
		})
	}
	if zoneID == "" {
		issues = append(issues, cfIssueView{
			Key:     cfIssueZone,
			Level:   cfLevelBad,
			Fixable: false,
			Message: "Zone " + record.Domain + " belum punya ID zone di profil ini, daftarkan ulang domainnya",
		})
		return issues
	}

	rule, err := client.GetCatchAll(zoneID)
	if err != nil {
		issues = append(issues, cfIssueView{
			Key:     cfIssueRouting,
			Level:   cfLevelBad,
			Fixable: true,
			Message: "Catch-all Email Routing tidak bisa dibaca: " + err.Error(),
		})
	} else {
		view.Routing = cfRoutingCheckView{Enabled: rule.Enabled, Target: rule.Target}
		view.Routing.OK = rule.Enabled && rule.Target == worker.Name()
		if !view.Routing.OK {
			message := "Catch-all Email Routing belum aktif"
			switch {
			case rule.Enabled && rule.Target == "":
				message = "Catch-all Email Routing aktif tapi tidak mengarah ke Worker"
			case rule.Enabled:
				message = "Catch-all Email Routing mengarah ke " + rule.Target + ", bukan ke Worker " + worker.Name()
			}
			issues = append(issues, cfIssueView{Key: cfIssueRouting, Level: cfLevelBad, Fixable: true, Message: message})
		}
	}

	required, requiredErr := client.EmailRoutingDNS(zoneID)
	existing, existingErr := client.ListDNSRecords(zoneID, "", "")
	if requiredErr != nil || existingErr != nil {
		cause := requiredErr
		if cause == nil {
			cause = existingErr
		}
		issues = append(issues, cfIssueView{
			Key:     cfIssueEmailDNS,
			Level:   cfLevelBad,
			Fixable: true,
			Message: "Record Email Routing tidak bisa dibaca: " + cause.Error(),
		})
		return issues
	}
	missing := cfMissingDNSRecords(required, existing)
	conflicts := cfMXConflicts(required, existing)
	for _, rec := range missing {
		view.EmailDNS.Missing = append(view.EmailDNS.Missing, cfDNSRecordViewOf(rec))
	}
	for _, rec := range conflicts {
		view.EmailDNS.Conflicts = append(view.EmailDNS.Conflicts, cfDNSRecordViewOf(rec))
	}
	view.EmailDNS.Ready = len(missing) == 0
	if !view.EmailDNS.Ready {
		issues = append(issues, cfIssueView{
			Key:     cfIssueEmailDNS,
			Level:   cfLevelBad,
			Fixable: true,
			Message: fmt.Sprintf("%d record Email Routing belum dibuat", len(missing)),
		})
	}
	if len(conflicts) > 0 {
		contents := make([]string, 0, len(conflicts))
		for _, rec := range conflicts {
			contents = append(contents, rec.Content)
		}
		issues = append(issues, cfIssueView{
			Key:     cfIssueMXConflict,
			Level:   cfLevelWarn,
			Fixable: false,
			Message: "MX di luar Cloudflare: " + strings.Join(contents, ", ") + " - periksa manual, panel tidak menghapusnya",
		})
	}
	return issues
}

// cfWebHostIssues fills the web host part of a check report: the tunnel CNAME that publishes
// the panel, resolved in the zone that owns the web host.
func cfWebHostIssues(db *sql.DB, record repository.CFDomain, view *cfDomainCheckView) []cfIssueView {
	issues := []cfIssueView{}
	_, tunnelID, webHost := cfTunnelSettings(db)
	view.WebHost.Host = webHost
	if webHost == "" {
		issues = append(issues, cfIssueView{
			Key:     cfIssueWebHostConfigured,
			Level:   cfLevelBad,
			Fixable: false,
			Message: "Web host tunnel belum diset, isi dulu di bagian Tunnel",
		})
		return issues
	}
	view.WebHost.Configured = true
	if tunnelID != "" {
		view.WebHost.Expected = tunnelID + ".cfargotunnel.com"
	}

	client, err := cfClientForHost(db, webHost, record.ProfileID)
	if err != nil {
		issues = append(issues, cfIssueView{
			Key:     cfIssueCNAME,
			Level:   cfLevelBad,
			Fixable: false,
			Message: "Profil Cloudflare untuk zone " + webHost + " tidak siap dipakai: " + err.Error(),
		})
		return issues
	}
	zone, err := client.FindZoneForHost(webHost)
	if err != nil {
		issues = append(issues, cfIssueView{
			Key:     cfIssueCNAME,
			Level:   cfLevelBad,
			Fixable: false,
			Message: "Zone untuk web host " + webHost + " tidak ditemukan: " + err.Error(),
		})
		return issues
	}
	view.WebHost.Zone = zone

	records, err := client.ListDNSRecords(zone.ID, "CNAME", webHost)
	if err != nil {
		issues = append(issues, cfIssueView{
			Key:     cfIssueCNAME,
			Level:   cfLevelBad,
			Fixable: false,
			Message: "DNS zone " + zone.Name + " tidak bisa dibaca: " + err.Error(),
		})
		return issues
	}
	matching := make([]cloudflare.DNSRecord, 0, len(records))
	correct := 0
	for _, rec := range records {
		if !strings.EqualFold(rec.Type, "CNAME") || !cfRecordNameMatches(rec.Name, webHost) {
			continue
		}
		matching = append(matching, rec)
		view.WebHost.Records = append(view.WebHost.Records, cfDNSRecordViewOf(rec))
		if view.WebHost.Expected != "" && rec.Content == view.WebHost.Expected && rec.Proxied {
			correct++
		}
	}
	// The panel is only reachable through the tunnel when exactly one proxied CNAME for the
	// host points at the tunnel.
	view.WebHost.OK = correct == 1
	if view.WebHost.OK {
		return issues
	}
	issue := cfIssueView{Key: cfIssueCNAME, Level: cfLevelWarn, Fixable: true}
	switch {
	case view.WebHost.Expected == "":
		issue.Fixable = false
		issue.Message = "ID tunnel belum ada, CNAME web host " + webHost + " belum bisa dibuat"
	case len(matching) == 0:
		issue.Message = "CNAME " + webHost + " -> " + view.WebHost.Expected + " (proxied) belum ada"
	default:
		contents := make([]string, 0, len(matching))
		for _, rec := range matching {
			contents = append(contents, rec.Content)
		}
		issue.Message = "CNAME " + webHost + " sudah ada (" + strings.Join(contents, ", ") + ") tapi tidak mengarah ke " + view.WebHost.Expected
	}
	return append(issues, issue)
}

// CFDomainFix runs the selected repairs for one registered domain. Every selected key is
// reported exactly once as done, skipped or failed, unselected keys never run, and nothing is
// deleted except the web host's own conflicting CNAME records when overwrite_cname asks for it.
func CFDomainFix(c *gin.Context) {
	var req struct {
		CNAME          bool `json:"cname"`
		Routing        bool `json:"routing"`
		Worker         bool `json:"worker"`
		EmailDNS       bool `json:"email_dns"`
		OverwriteCNAME bool `json:"overwrite_cname"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	db := cfDB()
	domain := strings.ToLower(strings.TrimSpace(c.Param("domain")))
	record, err := repository.GetCFDomain(db, domain)
	if err != nil {
		cfFail(c, http.StatusNotFound, "Domain tidak terdaftar", err)
		return
	}
	report := cfDomainFixView{
		Domain:  record.Domain,
		Done:    []cfFixStepView{},
		Skipped: []cfFixStepView{},
		Failed:  []cfFixStepView{},
	}

	needsProfile := req.Worker || req.Routing || req.EmailDNS
	if needsProfile {
		profile, client, profileErr := cfResolve(db, record.ProfileID)
		if profileErr != nil {
			message := "Profil " + record.ProfileID + " tidak siap dipakai"
			if req.Worker {
				cfFixFailed(&report, cfFixWorker, message, profileErr)
			}
			if req.Routing {
				cfFixFailed(&report, cfFixRouting, message, profileErr)
			}
			if req.EmailDNS {
				cfFixFailed(&report, cfFixEmailDNS, message, profileErr)
			}
		} else {
			if req.Worker {
				if err := cfDeployWorkerFor(db, profile, client); err != nil {
					cfFixFailed(&report, cfFixWorker, "Worker "+worker.Name()+" gagal di-deploy", err)
				} else {
					cfFixDone(&report, cfFixWorker, "Worker "+worker.Name()+" ter-deploy untuk profil "+profile.ID)
				}
			}
			if req.Routing {
				cfFixDomainRouting(db, record, client, &report)
			}
			if req.EmailDNS {
				cfFixDomainEmailDNS(record, client, &report)
			}
		}
	}
	if req.CNAME {
		cfFixWebHostCNAME(db, record, req.OverwriteCNAME, &report)
	}
	c.JSON(http.StatusOK, report)
}

// cfFixDomainRouting enables Email Routing, points the catch-all at the Worker and stores the
// new state. Routing is enabled first because a catch-all needs it.
func cfFixDomainRouting(db *sql.DB, record repository.CFDomain, client *cloudflare.Client, report *cfDomainFixView) {
	zoneID, err := cfZoneIDForFix(record, client)
	if err != nil {
		cfFixFailed(report, cfFixRouting, "Zone "+record.Domain+" tidak bisa dipakai untuk perbaikan routing", err)
		return
	}
	if err := client.EnableEmailRouting(zoneID); err != nil {
		cfFixFailed(report, cfFixRouting, "Cloudflare menolak mengaktifkan Email Routing", err)
		return
	}
	if err := client.SetCatchAll(zoneID, worker.Name(), true); err != nil {
		cfFixFailed(report, cfFixRouting, "Cloudflare menolak mengarahkan catch-all ke Worker "+worker.Name(), err)
		return
	}
	record.ZoneID = zoneID
	record.Routing = true
	record.Enabled = true
	if err := repository.UpsertCFDomain(db, record); err != nil {
		cfFixFailed(report, cfFixRouting, "Cloudflare sudah diubah, gagal menyimpan status domain", err)
		return
	}
	cfFixDone(report, cfFixRouting, "Email Routing aktif dan catch-all mengarah ke Worker "+worker.Name())
}

// cfFixDomainEmailDNS creates only the Email Routing records the zone is missing. Foreign MX
// records are left alone: they are reported by the check endpoint, not repaired here.
func cfFixDomainEmailDNS(record repository.CFDomain, client *cloudflare.Client, report *cfDomainFixView) {
	zoneID, err := cfZoneIDForFix(record, client)
	if err != nil {
		cfFixFailed(report, cfFixEmailDNS, "Zone "+record.Domain+" tidak bisa dipakai untuk perbaikan DNS", err)
		return
	}
	required, err := client.EmailRoutingDNS(zoneID)
	if err != nil {
		cfFixFailed(report, cfFixEmailDNS, "Gagal membaca record Email Routing yang dibutuhkan", err)
		return
	}
	existing, err := client.ListDNSRecords(zoneID, "", "")
	if err != nil {
		cfFixFailed(report, cfFixEmailDNS, "Gagal membaca DNS zone "+record.Domain, err)
		return
	}
	missing := cfMissingDNSRecords(required, existing)
	if len(missing) == 0 {
		cfFixSkipped(report, cfFixEmailDNS, "Semua record Email Routing sudah ada")
		return
	}
	created := 0
	failures := []string{}
	for _, rec := range missing {
		// Required records are reported with their Cloudflare id; a create must not echo it.
		rec.ID = ""
		if rec.TTL == 0 {
			rec.TTL = 1
		}
		if _, err := client.CreateDNSRecord(zoneID, rec); err != nil {
			failures = append(failures, rec.Type+" "+rec.Name+": "+err.Error())
			continue
		}
		created++
	}
	if len(failures) > 0 {
		cfFixFailed(report, cfFixEmailDNS,
			fmt.Sprintf("Hanya %d dari %d record Email Routing yang dibuat", created, len(missing)),
			errors.New(strings.Join(failures, "; ")))
		return
	}
	cfFixDone(report, cfFixEmailDNS, fmt.Sprintf("%d record Email Routing dibuat", created))
}

// cfFixWebHostCNAME writes the tunnel CNAME for the web host. A host that already has a CNAME
// is left alone unless overwrite_cname is set, and even then only that host's own CNAME records
// are deleted before the record is recreated with the tunnel target.
func cfFixWebHostCNAME(db *sql.DB, record repository.CFDomain, overwrite bool, report *cfDomainFixView) {
	_, tunnelID, webHost := cfTunnelSettings(db)
	if webHost == "" {
		cfFixSkipped(report, cfFixCNAME, "Web host tunnel belum diset, isi dulu di bagian Tunnel")
		return
	}
	if tunnelID == "" {
		cfFixSkipped(report, cfFixCNAME, "ID tunnel belum ada, buat atau isi ID tunnel dulu di bagian Tunnel")
		return
	}
	expected := tunnelID + ".cfargotunnel.com"
	client, err := cfClientForHost(db, webHost, record.ProfileID)
	if err != nil {
		cfFixFailed(report, cfFixCNAME, "Profil Cloudflare untuk zone "+webHost+" tidak siap dipakai", err)
		return
	}
	zone, err := client.FindZoneForHost(webHost)
	if err != nil {
		if errors.Is(err, cloudflare.ErrZoneNotFound) {
			cfFixSkipped(report, cfFixCNAME, "Zone "+webHost+" tidak ada di akun Cloudflare profil ini, daftarkan domain web host dulu")
			return
		}
		cfFixFailed(report, cfFixCNAME, "Zone untuk web host "+webHost+" tidak bisa dibaca", err)
		return
	}
	records, err := client.ListDNSRecords(zone.ID, "CNAME", webHost)
	if err != nil {
		cfFixFailed(report, cfFixCNAME, "Gagal membaca DNS zone "+zone.Name, err)
		return
	}
	matching := []cloudflare.DNSRecord{}
	correct := false
	for _, rec := range records {
		if !strings.EqualFold(rec.Type, "CNAME") || !cfRecordNameMatches(rec.Name, webHost) {
			continue
		}
		matching = append(matching, rec)
		if rec.Content == expected && rec.Proxied {
			correct = true
		}
	}
	if correct {
		if len(matching) > 1 {
			cfFixSkipped(report, cfFixCNAME, fmt.Sprintf(
				"CNAME %s -> %s sudah ada, tapi ada %d record untuk host ini, periksa manual", webHost, expected, len(matching)))
			return
		}
		cfFixSkipped(report, cfFixCNAME, "CNAME "+webHost+" -> "+expected+" sudah benar")
		return
	}
	if len(matching) > 0 && !overwrite {
		contents := make([]string, 0, len(matching))
		for _, rec := range matching {
			contents = append(contents, rec.Content)
		}
		cfFixSkipped(report, cfFixCNAME, "CNAME "+webHost+" sudah ada dengan target "+
			strings.Join(contents, ", ")+", tidak diubah (kirim overwrite_cname untuk menggantinya)")
		return
	}
	for _, rec := range matching {
		if err := client.DeleteDNSRecord(zone.ID, rec.ID); err != nil {
			cfFixFailed(report, cfFixCNAME, "CNAME lama "+webHost+" gagal dihapus, record baru tidak dibuat", err)
			return
		}
	}
	if _, err := client.CreateDNSRecord(zone.ID, cloudflare.DNSRecord{
		Type: "CNAME", Name: webHost, Content: expected, Proxied: true, TTL: 1,
	}); err != nil {
		cfFixFailed(report, cfFixCNAME, "Cloudflare menolak pembuatan CNAME "+webHost, err)
		return
	}
	message := "CNAME " + webHost + " -> " + expected + " dibuat"
	if len(matching) > 0 {
		message = "CNAME " + webHost + " diganti ke " + expected
	}
	cfFixDone(report, cfFixCNAME, message)
}
