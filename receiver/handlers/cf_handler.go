package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"

	"receiver/cloudflare"
	"receiver/database"
	"receiver/repository"
	"receiver/worker"

	"github.com/gin-gonic/gin"
)

var cfProfileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

type cfProfileView struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	AccountID      string `json:"account_id"`
	Active         bool   `json:"active"`
	HasToken       bool   `json:"has_token"`
	WorkerDeployed bool   `json:"worker_deployed"`
	DomainCount    int    `json:"domain_count"`
}

type cfDomainView struct {
	Domain  string `json:"domain"`
	Profile string `json:"profile"`
	ZoneID  string `json:"zone_id"`
	Enabled bool   `json:"enabled"`
	Routing bool   `json:"routing"`
}

type cfTunnelView struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	WebHost     string `json:"web_host"`
	Status      string `json:"status"`
	Connections int    `json:"connections"`
	AccountID   string `json:"account_id"`
	Source      string `json:"source"`
	Error       string `json:"error"`
}

// settingsKey prefixes are declared in the repository package so menu and web read the same keys.
const settingTunnelID = "cf_tunnel_id"

func cfAPIBase() string {
	base := os.Getenv("CLOUDFLARE_API_BASE")
	if base == "" {
		return "https://api.cloudflare.com/client/v4"
	}
	return strings.TrimRight(base, "/")
}

// cfFail reports an error to the client. The raw Cloudflare message goes to "detail" so the
// operator sees why a call was rejected instead of a generic failure.
func cfFail(c *gin.Context, status int, msg string, err error) {
	body := gin.H{"error": msg}
	if err != nil {
		body["detail"] = err.Error()
	}
	c.JSON(status, body)
}

func cfDB() *sql.DB { return database.Conn() }

func cfActiveProfile(db *sql.DB) string {
	return repository.GetSetting(db, repository.SettingActiveProfile)
}

// cfResolve selects the requested profile, or the active one when id is empty.
func cfResolve(db *sql.DB, id string) (repository.CFProfile, *cloudflare.Client, error) {
	if id == "" {
		id = cfActiveProfile(db)
	}
	if id == "" {
		return repository.CFProfile{}, nil, errors.New("belum ada profil Cloudflare aktif")
	}
	p, err := repository.GetCFProfile(db, id)
	if err != nil {
		return repository.CFProfile{}, nil, err
	}
	sealed, err := repository.GetCFProfileToken(db, id)
	if err != nil {
		return repository.CFProfile{}, nil, err
	}
	token, err := cloudflare.OpenSecret(sealed)
	if err != nil {
		return repository.CFProfile{}, nil, err
	}
	if token == "" {
		return repository.CFProfile{}, nil, errors.New("profil " + id + " belum punya API token")
	}
	return p, cloudflare.NewClient(token), nil
}

func cfProfileViewFor(db *sql.DB, p repository.CFProfile, active string) cfProfileView {
	count, err := repository.CountCFDomainsForProfile(db, p.ID)
	if err != nil {
		count = 0
	}
	sealed, _ := repository.GetCFProfileToken(db, p.ID)
	return cfProfileView{
		ID:             p.ID,
		Label:          p.Label,
		AccountID:      p.AccountID,
		Active:         p.ID == active,
		HasToken:       sealed != "",
		WorkerDeployed: p.WorkerDeployed,
		DomainCount:    count,
	}
}

func cfDomainViewOf(d repository.CFDomain) cfDomainView {
	return cfDomainView{Domain: d.Domain, Profile: d.ProfileID, ZoneID: d.ZoneID, Enabled: d.Enabled, Routing: d.Routing}
}

func cfConfiguredCopies(db *sql.DB) []string {
	raw := repository.GetSetting(db, repository.SettingCopies)
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// configuredDomains returns the domains offered by the UI: routing-enabled domains first,
// then the DOMAINS env fallback, so the panel works before any Cloudflare profile exists.
func configuredDomains() []string {
	if db := cfDB(); db != nil {
		if listed, err := repository.ListedDomains(db); err == nil && len(listed) > 0 {
			return listed
		}
	}
	var out []string
	for _, part := range strings.Split(os.Getenv("DOMAINS"), ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func cfTunnelSettings(db *sql.DB) (name, id, webHost string) {
	name = repository.GetSetting(db, repository.SettingTunnelName)
	if name == "" {
		name = os.Getenv("CF_TUNNEL_NAME")
	}
	if name == "" {
		name = "tempmailbyjhopanstore"
	}
	return name, repository.GetSetting(db, settingTunnelID), repository.GetSetting(db, repository.SettingTunnelWebHost)
}

func cfTunnelViewFor(db *sql.DB) cfTunnelView {
	name, id, webHost := cfTunnelSettings(db)
	view := cfTunnelView{Name: name, ID: id, WebHost: webHost, Status: "unknown", Source: "config"}
	profile, client, err := cfResolve(db, "")
	if err != nil {
		view.Source = "none"
		view.Error = err.Error()
		return view
	}
	view.AccountID = profile.AccountID
	tunnels, err := client.ListTunnels(profile.AccountID)
	if err != nil {
		view.Error = err.Error()
		return view
	}
	view.Source = "api"
	for _, t := range tunnels {
		if t.Name != name {
			continue
		}
		view.ID = t.ID
		view.Status = t.Status
		view.Connections = t.Connections
		// The connections array on the tunnel list is deprecated, so ask the dedicated endpoint.
		if count, err := client.TunnelConnections(profile.AccountID, t.ID); err == nil {
			view.Connections = count
		}
		return view
	}
	view.Status = "not-found"
	return view
}

// CFStatus is the single call the menu and the panel use to render current state.
func CFStatus(c *gin.Context) {
	db := cfDB()
	active := cfActiveProfile(db)
	profiles, err := repository.ListCFProfiles(db)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal membaca profil", err)
		return
	}
	views := make([]cfProfileView, 0, len(profiles))
	for _, p := range profiles {
		views = append(views, cfProfileViewFor(db, p, active))
	}
	domains, err := repository.ListCFDomains(db)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal membaca domain", err)
		return
	}
	domainViews := make([]cfDomainView, 0, len(domains))
	for _, d := range domains {
		domainViews = append(domainViews, cfDomainViewOf(d))
	}
	c.JSON(http.StatusOK, gin.H{
		"profiles":           views,
		"active_profile":     active,
		"domains":            domainViews,
		"tunnel":             cfTunnelViewFor(db),
		"copies":             cfConfiguredCopies(db),
		"worker_name":        worker.Name(),
		"worker_compat_date": worker.CompatDate(),
		"env_domains":        configuredDomains(),
		"api_base":           cfAPIBase(),
	})
}

func CFListProfiles(c *gin.Context) {
	db := cfDB()
	active := cfActiveProfile(db)
	profiles, err := repository.ListCFProfiles(db)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal membaca profil", err)
		return
	}
	views := make([]cfProfileView, 0, len(profiles))
	for _, p := range profiles {
		views = append(views, cfProfileViewFor(db, p, active))
	}
	c.JSON(http.StatusOK, views)
}

func CFCreateProfile(c *gin.Context) {
	var req struct {
		ID        string `json:"id"`
		Label     string `json:"label"`
		AccountID string `json:"account_id"`
		Token     string `json:"token"`
		Activate  bool   `json:"activate"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.AccountID = strings.TrimSpace(req.AccountID)
	req.Token = strings.TrimSpace(req.Token)
	if !cfProfileIDPattern.MatchString(req.ID) {
		cfFail(c, http.StatusBadRequest, "ID profil hanya boleh huruf, angka, -, _ (maks 32)", nil)
		return
	}
	if req.AccountID == "" || req.Token == "" {
		cfFail(c, http.StatusBadRequest, "Account ID dan API token wajib diisi", nil)
		return
	}
	db := cfDB()
	if _, err := repository.GetCFProfile(db, req.ID); err == nil {
		cfFail(c, http.StatusConflict, "Profil "+req.ID+" sudah ada", nil)
		return
	}
	account, err := cloudflare.NewClient(req.Token).Account(req.AccountID)
	if err != nil {
		cfFail(c, http.StatusBadRequest, "API token atau Account ID ditolak Cloudflare", err)
		return
	}
	sealed, err := cloudflare.SealSecret(req.Token)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal mengenkripsi token", err)
		return
	}
	label := req.Label
	if label == "" {
		label = account.Name
	}
	if label == "" {
		label = req.ID
	}
	if err := repository.CreateCFProfile(db, repository.CFProfile{ID: req.ID, Label: label, AccountID: account.ID}); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menyimpan profil", err)
		return
	}
	if err := repository.SetCFProfileToken(db, req.ID, sealed); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menyimpan token", err)
		return
	}
	if req.Activate || cfActiveProfile(db) == "" {
		if err := repository.SetSetting(db, repository.SettingActiveProfile, req.ID); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan profil aktif", err)
			return
		}
	}
	p, err := repository.GetCFProfile(db, req.ID)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal membaca profil tersimpan", err)
		return
	}
	c.JSON(http.StatusCreated, cfProfileViewFor(db, p, cfActiveProfile(db)))
}

func CFUpdateProfile(c *gin.Context) {
	var req struct {
		Label     *string `json:"label"`
		AccountID *string `json:"account_id"`
		Token     *string `json:"token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	db := cfDB()
	profile, err := repository.GetCFProfile(db, c.Param("id"))
	if err != nil {
		cfFail(c, http.StatusNotFound, "Profil tidak ditemukan", err)
		return
	}
	accountID := profile.AccountID
	tokenSealed, _ := repository.GetCFProfileToken(db, profile.ID)
	token, err := cloudflare.OpenSecret(tokenSealed)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal membuka token tersimpan", err)
		return
	}
	if req.AccountID != nil && strings.TrimSpace(*req.AccountID) != "" {
		accountID = strings.TrimSpace(*req.AccountID)
	}
	if req.Token != nil && strings.TrimSpace(*req.Token) != "" {
		token = strings.TrimSpace(*req.Token)
	}
	if _, err := cloudflare.NewClient(token).Account(accountID); err != nil {
		cfFail(c, http.StatusBadRequest, "API token atau Account ID ditolak Cloudflare", err)
		return
	}
	label := profile.Label
	if req.Label != nil && strings.TrimSpace(*req.Label) != "" {
		label = strings.TrimSpace(*req.Label)
	}
	if err := repository.UpdateCFProfileMeta(db, profile.ID, label, accountID); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal memperbarui profil", err)
		return
	}
	if req.Token != nil && strings.TrimSpace(*req.Token) != "" {
		sealed, err := cloudflare.SealSecret(token)
		if err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal mengenkripsi token", err)
			return
		}
		if err := repository.SetCFProfileToken(db, profile.ID, sealed); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan token", err)
			return
		}
	}
	updated, err := repository.GetCFProfile(db, profile.ID)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal membaca profil", err)
		return
	}
	c.JSON(http.StatusOK, cfProfileViewFor(db, updated, cfActiveProfile(db)))
}

func CFDeleteProfile(c *gin.Context) {
	db := cfDB()
	id := c.Param("id")
	if _, err := repository.GetCFProfile(db, id); err != nil {
		cfFail(c, http.StatusNotFound, "Profil tidak ditemukan", err)
		return
	}
	count, err := repository.CountCFDomainsForProfile(db, id)
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menghitung domain profil", err)
		return
	}
	if count > 0 {
		cfFail(c, http.StatusConflict, "Masih ada domain memakai profil ini, hapus atau pindahkan dulu", nil)
		return
	}
	if err := repository.DeleteCFProfile(db, id); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menghapus profil", err)
		return
	}
	if cfActiveProfile(db) == id {
		if err := repository.SetSetting(db, repository.SettingActiveProfile, ""); err != nil {
			cfFail(c, http.StatusInternalServerError, "Profil terhapus, gagal mengosongkan profil aktif", err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func CFActivateProfile(c *gin.Context) {
	db := cfDB()
	id := c.Param("id")
	if _, err := repository.GetCFProfile(db, id); err != nil {
		cfFail(c, http.StatusNotFound, "Profil tidak ditemukan", err)
		return
	}
	if err := repository.SetSetting(db, repository.SettingActiveProfile, id); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menyimpan profil aktif", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"active_profile": id})
}

func CFDeployWorker(c *gin.Context) {
	db := cfDB()
	profile, client, err := cfResolve(db, c.Param("id"))
	if err != nil {
		cfFail(c, http.StatusBadRequest, "Profil tidak siap dipakai", err)
		return
	}
	_, _, webHost := cfTunnelSettings(db)
	bindings, secrets, err := cfWorkerBindings(db, webHost)
	if err != nil {
		cfFail(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	deployment, err := client.DeployWorker(profile.AccountID, worker.Name(), worker.CompatDate(), worker.Script(), bindings)
	if err != nil {
		cfFail(c, http.StatusBadGateway, "Cloudflare menolak deploy Worker", err)
		return
	}
	if err := client.SetWorkerSubdomain(profile.AccountID, worker.Name(), false, false); err != nil {
		cfFail(c, http.StatusBadGateway, "Worker terunggah, gagal mematikan workers.dev", err)
		return
	}
	if err := repository.SetCFProfileWorkerDeployed(db, profile.ID, true); err != nil {
		cfFail(c, http.StatusInternalServerError, "Worker terunggah, gagal menyimpan status profil", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"profile":     profile.ID,
		"worker":      worker.Name(),
		"modified_on": deployment.ModifiedOn,
		"secrets":     secrets,
	})
}

func CFDeleteWorker(c *gin.Context) {
	db := cfDB()
	profile, client, err := cfResolve(db, c.Param("id"))
	if err != nil {
		cfFail(c, http.StatusBadRequest, "Profil tidak siap dipakai", err)
		return
	}
	if err := client.DeleteWorker(profile.AccountID, worker.Name()); err != nil {
		cfFail(c, http.StatusBadGateway, "Cloudflare menolak hapus Worker", err)
		return
	}
	if err := repository.SetCFProfileWorkerDeployed(db, profile.ID, false); err != nil {
		cfFail(c, http.StatusInternalServerError, "Worker terhapus, gagal menyimpan status profil", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "worker": worker.Name()})
}

func CFListDomains(c *gin.Context) {
	domains, err := repository.ListCFDomains(cfDB())
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal membaca domain", err)
		return
	}
	views := make([]cfDomainView, 0, len(domains))
	for _, d := range domains {
		views = append(views, cfDomainViewOf(d))
	}
	c.JSON(http.StatusOK, views)
}

func CFAddDomain(c *gin.Context) {
	var req struct {
		Domain  string `json:"domain"`
		Profile string `json:"profile"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if req.Domain == "" || !strings.Contains(req.Domain, ".") {
		cfFail(c, http.StatusBadRequest, "Domain tidak valid", nil)
		return
	}
	db := cfDB()
	if _, err := repository.GetCFDomain(db, req.Domain); err == nil {
		cfFail(c, http.StatusConflict, "Domain "+req.Domain+" sudah terdaftar", nil)
		return
	}
	profile, client, err := cfResolve(db, strings.TrimSpace(req.Profile))
	if err != nil {
		cfFail(c, http.StatusBadRequest, "Profil tidak siap dipakai", err)
		return
	}
	zoneID, err := client.ZoneID(req.Domain)
	if err != nil {
		if errors.Is(err, cloudflare.ErrZoneNotFound) {
			cfFail(c, http.StatusBadRequest, "Zone "+req.Domain+" tidak ada di akun "+profile.ID+" atau belum aktif", err)
			return
		}
		cfFail(c, http.StatusBadGateway, "Cloudflare menolak pencarian zone", err)
		return
	}
	domain := repository.CFDomain{Domain: req.Domain, ProfileID: profile.ID, ZoneID: zoneID}
	if err := repository.UpsertCFDomain(db, domain); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menyimpan domain", err)
		return
	}
	c.JSON(http.StatusCreated, cfDomainViewOf(domain))
}

func CFDeleteDomain(c *gin.Context) {
	db := cfDB()
	domain := strings.ToLower(strings.TrimSpace(c.Param("domain")))
	if _, err := repository.GetCFDomain(db, domain); err != nil {
		cfFail(c, http.StatusNotFound, "Domain tidak terdaftar", err)
		return
	}
	if err := repository.DeleteCFDomain(db, domain); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menghapus domain", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "cloudflare_untouched": true})
}

func CFSetDomainRouting(c *gin.Context) {
	var req struct {
		Routing   bool `json:"routing"`
		UnlockDNS bool `json:"unlock_dns"`
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
	_, client, err := cfResolve(db, record.ProfileID)
	if err != nil {
		cfFail(c, http.StatusBadRequest, "Profil domain ini tidak siap dipakai", err)
		return
	}
	// Domains imported from the legacy state file carry no zone id, so resolve it once here.
	if record.ZoneID == "" {
		resolved, err := client.ZoneID(record.Domain)
		if err != nil {
			cfFail(c, http.StatusBadRequest, "Zone "+record.Domain+" tidak ditemukan di profil "+record.ProfileID, err)
			return
		}
		record.ZoneID = resolved
	}
	dnsUnlocked := false
	// Email Routing must be enabled before a catch-all can reference the Worker.
	if req.Routing {
		if err := client.EnableEmailRouting(record.ZoneID); err != nil {
			cfFail(c, http.StatusBadGateway, "Cloudflare menolak mengaktifkan Email Routing", err)
			return
		}
		if req.UnlockDNS {
			if err := client.UnlockEmailRoutingDNS(record.ZoneID); err == nil {
				dnsUnlocked = true
			}
		}
	}
	if err := client.SetCatchAll(record.ZoneID, worker.Name(), req.Routing); err != nil {
		cfFail(c, http.StatusBadGateway, "Cloudflare menolak perubahan catch-all", err)
		return
	}
	record.Routing = req.Routing
	record.Enabled = req.Routing
	if err := repository.UpsertCFDomain(db, record); err != nil {
		cfFail(c, http.StatusInternalServerError, "Cloudflare sudah diubah, gagal menyimpan status domain", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"domain":       record.Domain,
		"profile":      record.ProfileID,
		"zone_id":      record.ZoneID,
		"enabled":      record.Enabled,
		"routing":      record.Routing,
		"dns_unlocked": dnsUnlocked,
	})
}

func CFTunnelStatus(c *gin.Context) {
	c.JSON(http.StatusOK, cfTunnelViewFor(cfDB()))
}

func CFSetTunnel(c *gin.Context) {
	var req struct {
		WebHost string `json:"web_host"`
		Name    string `json:"name"`
		ID      string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	db := cfDB()
	if id := strings.TrimSpace(req.ID); id != "" {
		if err := repository.SetSetting(db, settingTunnelID, id); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan ID tunnel", err)
			return
		}
	}
	if host := strings.ToLower(strings.TrimSpace(req.WebHost)); host != "" {
		if err := repository.SetSetting(db, repository.SettingTunnelWebHost, host); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan web host", err)
			return
		}
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		if err := repository.SetSetting(db, repository.SettingTunnelName, name); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan nama tunnel", err)
			return
		}
	}
	c.JSON(http.StatusOK, cfTunnelViewFor(db))
}

func CFCreateTunnel(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		WebHost string `json:"web_host"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	db := cfDB()
	profile, client, err := cfResolve(db, "")
	if err != nil {
		cfFail(c, http.StatusBadRequest, "Profil tidak siap dipakai", err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name, _, _ = cfTunnelSettings(db)
	}
	tunnel, err := client.CreateTunnel(profile.AccountID, name)
	if err != nil {
		cfFail(c, http.StatusBadGateway, "Cloudflare menolak pembuatan tunnel", err)
		return
	}
	token, err := client.TunnelToken(profile.AccountID, tunnel.ID)
	if err != nil {
		cfFail(c, http.StatusBadGateway, "Tunnel dibuat, gagal mengambil token konektor", err)
		return
	}
	if err := repository.SetSetting(db, repository.SettingTunnelName, name); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menyimpan nama tunnel", err)
		return
	}
	if err := repository.SetSetting(db, settingTunnelID, tunnel.ID); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menyimpan ID tunnel", err)
		return
	}
	webHost := strings.ToLower(strings.TrimSpace(req.WebHost))
	if webHost != "" {
		if err := repository.SetSetting(db, repository.SettingTunnelWebHost, webHost); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan web host", err)
			return
		}
	}
	commands := []string{
		"sudo cloudflared service install " + token,
		"sudo systemctl enable --now cloudflared",
		"sudo systemctl status cloudflared --no-pager",
	}
	if webHost != "" {
		commands = append(commands,
			"# DNS: buat CNAME "+webHost+" -> "+tunnel.ID+".cfargotunnel.com (proxied) di zone web host")
	}
	c.JSON(http.StatusOK, gin.H{
		"id":       tunnel.ID,
		"name":     name,
		"token":    token,
		"commands": commands,
	})
}

func CFGetCopies(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"destinations": cfConfiguredCopies(cfDB())})
}

func CFSetCopies(c *gin.Context) {
	var req struct {
		Destinations []string `json:"destinations"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	cleaned := []string{}
	for _, dest := range req.Destinations {
		trimmed := strings.TrimSpace(dest)
		if trimmed == "" {
			continue
		}
		if !strings.Contains(trimmed, "@") || !strings.Contains(trimmed, ".") {
			cfFail(c, http.StatusBadRequest, "Alamat tujuan tidak valid: "+trimmed, nil)
			return
		}
		cleaned = append(cleaned, trimmed)
	}
	db := cfDB()
	if err := repository.SetSetting(db, repository.SettingCopies, strings.Join(cleaned, ",")); err != nil {
		cfFail(c, http.StatusInternalServerError, "Gagal menyimpan tujuan salinan", err)
		return
	}
	appliedTo := []string{}
	profiles, err := repository.ListCFProfiles(db)
	if err == nil {
		for _, p := range profiles {
			if p.WorkerDeployed {
				appliedTo = append(appliedTo, p.ID)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"destinations": cleaned, "applied_to": appliedTo})
}

// CFImport loads an existing cloudflare-state.json written by the bash menu into the database.
// Existing profiles are left untouched so a re-run cannot overwrite a rotated token.
func CFImport(c *gin.Context) {
	var req struct {
		State struct {
			ActiveProfile string `json:"active_profile"`
			Profiles      map[string]struct {
				Label          string `json:"label"`
				AccountID      string `json:"account_id"`
				Token          string `json:"token"`
				WorkerDeployed bool   `json:"worker_deployed"`
			} `json:"profiles"`
			Domains map[string]struct {
				Profile string `json:"profile"`
				ZoneID  string `json:"zone_id"`
				Enabled bool   `json:"enabled"`
				Routing bool   `json:"routing"`
			} `json:"domains"`
			Tunnel struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				WebHost string `json:"web_host"`
			} `json:"tunnel"`
			CopyDestinations string `json:"copy_destinations"`
		} `json:"state"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "Body tidak valid", err)
		return
	}
	db := cfDB()
	importedProfiles := 0
	for id, p := range req.State.Profiles {
		if !cfProfileIDPattern.MatchString(id) {
			continue
		}
		_, lookupErr := repository.GetCFProfile(db, id)
		if lookupErr != nil {
			label := p.Label
			if label == "" {
				label = id
			}
			if err := repository.CreateCFProfile(db, repository.CFProfile{ID: id, Label: label, AccountID: p.AccountID}); err != nil {
				cfFail(c, http.StatusInternalServerError, "Gagal mengimpor profil "+id, err)
				return
			}
			importedProfiles++
		}
		// An existing profile keeps its token unless it has none yet, so re-running the import
		// never overwrites a rotated credential.
		sealed, _ := repository.GetCFProfileToken(db, id)
		if p.Token != "" && sealed == "" {
			encrypted, err := cloudflare.SealSecret(p.Token)
			if err != nil {
				cfFail(c, http.StatusInternalServerError, "Gagal mengenkripsi token profil "+id, err)
				return
			}
			if err := repository.SetCFProfileToken(db, id, encrypted); err != nil {
				cfFail(c, http.StatusInternalServerError, "Gagal menyimpan token profil "+id, err)
				return
			}
		}
		if p.WorkerDeployed {
			if err := repository.SetCFProfileWorkerDeployed(db, id, true); err != nil {
				cfFail(c, http.StatusInternalServerError, "Gagal menyimpan status Worker profil "+id, err)
				return
			}
		}
	}
	importedDomains := 0
	for name, d := range req.State.Domains {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		record := repository.CFDomain{Domain: name, ProfileID: d.Profile, ZoneID: d.ZoneID, Enabled: d.Enabled, Routing: d.Routing}
		if err := repository.UpsertCFDomain(db, record); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal mengimpor domain "+name, err)
			return
		}
		importedDomains++
	}
	if req.State.ActiveProfile != "" && cfActiveProfile(db) == "" {
		if err := repository.SetSetting(db, repository.SettingActiveProfile, req.State.ActiveProfile); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan profil aktif", err)
			return
		}
	}
	tunnelImported := false
	if req.State.Tunnel.ID != "" {
		if err := repository.SetSetting(db, settingTunnelID, req.State.Tunnel.ID); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan ID tunnel", err)
			return
		}
		tunnelImported = true
	}
	if req.State.Tunnel.Name != "" {
		if err := repository.SetSetting(db, repository.SettingTunnelName, req.State.Tunnel.Name); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan nama tunnel", err)
			return
		}
		tunnelImported = true
	}
	if req.State.Tunnel.WebHost != "" {
		if err := repository.SetSetting(db, repository.SettingTunnelWebHost, strings.ToLower(req.State.Tunnel.WebHost)); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan web host", err)
			return
		}
		tunnelImported = true
	}
	if strings.TrimSpace(req.State.CopyDestinations) != "" {
		if err := repository.SetSetting(db, repository.SettingCopies, strings.TrimSpace(req.State.CopyDestinations)); err != nil {
			cfFail(c, http.StatusInternalServerError, "Gagal menyimpan tujuan salinan", err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"imported": gin.H{
			"profiles":       importedProfiles,
			"domains":        importedDomains,
			"tunnel":         tunnelImported,
			"active_profile": cfActiveProfile(db),
		},
	})
}
