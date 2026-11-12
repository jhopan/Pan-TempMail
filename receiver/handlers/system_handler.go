package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"receiver/database"
	"receiver/repository"
	"receiver/worker"

	"github.com/gin-gonic/gin"
)

// Root helper contract v1. The app never has root: it writes a request JSON into
// $HELPER_DIR/in/<id>.json, the systemd path unit runs the helper as root, and the helper
// answers in $HELPER_DIR/out/<id>.json. See docs/superpowers/plans/2026-11-12-root-helper.md.
const (
	defaultHelperDir    = "/opt/tempmail/data/helper"
	defaultHelperWait   = 25
	helperPollInterval  = 200 * time.Millisecond
	defaultRestartDelay = 2
	maxRestartDelay     = 10
	helperRequestMode   = 0o640
	helperDirMode       = 0o750
)

// processStart backs app.uptime_seconds; the panel started when this process loaded.
var processStart = time.Now()

// helperActions is the contract allowlist; anything else is refused before a file is written.
var helperActions = map[string]bool{
	"service-status":  true,
	"restart-service": true,
	"backup-now":      true,
	"logs":            true,
	"doctor":          true,
	"tunnel-status":   true,
	"tunnel-install":  true,
	"tunnel-restart":  true,
	"tunnel-logout":   true,
}

// helperFireAndForget actions cut the app or the tunnel, so no result can ever be awaited.
var helperFireAndForget = map[string]bool{
	"restart-service": true,
	"tunnel-logout":   true,
}

var (
	// RE2 caps repeat counts at 1000, so the contract's {20,4096} is a char class plus a length check.
	helperTokenPattern             = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
	helperTokenMin, helperTokenMax = 20, 4096
)

// helperRequest is the exact contract payload; the helper rejects unknown fields.
type helperRequest struct {
	ID          string `json:"id"`
	Action      string `json:"action"`
	Lines       int    `json:"lines"`
	Token       string `json:"token"`
	Confirm     string `json:"confirm"`
	Delay       int    `json:"delay"`
	RequestedBy string `json:"requested_by"`
}

type systemAction struct {
	Action  string `json:"action"`
	Lines   int    `json:"lines"`
	Token   string `json:"token"`
	Confirm string `json:"confirm"`
	Delay   int    `json:"delay"`
}

func helperDir() string {
	if dir := os.Getenv("HELPER_DIR"); dir != "" {
		return dir
	}
	return defaultHelperDir
}

func helperWait() time.Duration {
	seconds := defaultHelperWait
	if raw := os.Getenv("HELPER_WAIT_SECONDS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			seconds = n
		}
	}
	return time.Duration(seconds) * time.Second
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// countQuery returns 0 instead of an error: one missing table must not break the whole panel.
func countQuery(db *sql.DB, query string) int {
	if db == nil {
		return 0
	}
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		return 0
	}
	return n
}

// rssBytes reports the resident set size. Only Linux exposes /proc/self/statm, so other
// systems report 0 rather than a made-up number.
func rssBytes() uint64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return pages * uint64(os.Getpagesize())
}

func systemStatusHelper() gin.H {
	dir := helperDir()
	inDir := filepath.Join(dir, "in")
	installed := false
	pending := 0
	if info, err := os.Stat(inDir); err == nil && info.IsDir() {
		// syscall.Access does not exist on Windows; the owner write bit is the portable signal.
		installed = info.Mode().Perm()&0o200 != 0
		if files, err := filepath.Glob(filepath.Join(inDir, "*.json")); err == nil {
			pending = len(files)
		}
	}
	return gin.H{"installed": installed, "dir": dir, "pending": pending}
}

// statusSeriesHours and statusSeriesCap drive the dashboard's hourly activity chart: 24 bars
// counted from the last 24 hours, with a row cap so a busy instance stays cheap to read.
const (
	statusSeriesHours = 24
	statusSeriesCap   = 5000
)

func systemStatusDB() gin.H {
	db := database.Conn()
	path := envOr("DB_PATH", "data/tempmail.db")
	var size int64
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	series, capped, err := repository.MessageSeries(db, time.Now().UTC(), statusSeriesHours, statusSeriesCap)
	if err != nil {
		// A broken query must not break the panel: the chart stays valid but empty.
		log.Printf("system status: message series: %v", err)
		series = make([]int, statusSeriesHours)
	}
	peak := 0
	for _, n := range series {
		if n > peak {
			peak = n
		}
	}
	return gin.H{
		"path":                path,
		"bytes":               size,
		"inboxes":             countQuery(db, "SELECT COUNT(*) FROM inboxes"),
		"messages":            countQuery(db, "SELECT COUNT(*) FROM messages"),
		"attachments":         countQuery(db, "SELECT COUNT(*) FROM attachments"),
		"series":              series,
		"series_peak":         peak,
		"series_window_hours": statusSeriesHours,
		"series_capped":       capped,
	}
}

func systemStatusApp() gin.H {
	return gin.H{
		"uptime_seconds": int64(time.Since(processStart).Seconds()),
		"rss_bytes":      rssBytes(),
		"go_version":     runtime.Version(),
		"site_name":      envOr("SITE_NAME", "TempMail"),
		"port":           envOr("PORT", "8080"),
	}
}

func systemStatusCloudflare(db *sql.DB) gin.H {
	profiles, domains, active := 0, 0, ""
	if db != nil {
		if list, err := repository.ListCFProfiles(db); err == nil {
			profiles = len(list)
		}
		if list, err := repository.ListCFDomains(db); err == nil {
			domains = len(list)
		}
		active = repository.GetSetting(db, repository.SettingActiveProfile)
	}
	return gin.H{
		"profiles":       profiles,
		"domains":        domains,
		"active_profile": active,
		"worker_name":    worker.Name(),
	}
}

// SystemStatus answers with facts this process already knows. It never talks to the helper, so
// the panel still renders when the helper is missing or busy.
func SystemStatus(c *gin.Context) {
	db := database.Conn()
	c.JSON(http.StatusOK, gin.H{
		"helper":     systemStatusHelper(),
		"db":         systemStatusDB(),
		"app":        systemStatusApp(),
		"cloudflare": systemStatusCloudflare(db),
	})
}

// SystemAction validates the request against the contract allowlist, publishes it for the root
// helper and, unless the action is fire-and-forget, waits for the helper answer.
func SystemAction(c *gin.Context) {
	var req systemAction
	if err := c.ShouldBindJSON(&req); err != nil {
		cfFail(c, http.StatusBadRequest, "body JSON tidak valid", err)
		return
	}
	if !helperActions[req.Action] {
		cfFail(c, http.StatusBadRequest, "aksi tidak didukung: "+req.Action, nil)
		return
	}
	payload, err := helperActionPayload(req)
	if err != nil {
		cfFail(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	dir := helperDir()
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "root helper belum dipasang di server ini"})
		return
	}
	inDir, outDir := filepath.Join(dir, "in"), filepath.Join(dir, "out")
	for _, sub := range []string{inDir, outDir} {
		if err := os.MkdirAll(sub, helperDirMode); err != nil {
			cfFail(c, http.StatusInternalServerError, "gagal menyiapkan direktori helper", err)
			return
		}
	}
	id, err := helperRequestID()
	if err != nil {
		cfFail(c, http.StatusInternalServerError, "gagal membuat id request", err)
		return
	}
	payload.ID = id
	payload.RequestedBy = c.GetString("email")
	if err := writeHelperRequest(inDir, id, payload); err != nil {
		cfFail(c, http.StatusInternalServerError, "gagal menulis request helper", err)
		return
	}
	if helperFireAndForget[req.Action] {
		c.JSON(http.StatusOK, gin.H{"id": id, "action": req.Action, "status": "accepted", "ok": true})
		return
	}
	result, ok := waitHelperResult(c.Request.Context(), outDir, id)
	if !ok {
		// The request file stays in in/ so the helper still performs the action.
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "helper tidak menjawab", "id": id})
		return
	}
	c.JSON(http.StatusOK, result)
}

// helperActionPayload returns the validated contract fields for one action. An invalid parameter
// is refused here so no request file is ever written for it.
func helperActionPayload(req systemAction) (helperRequest, error) {
	payload := helperRequest{Action: req.Action}
	switch req.Action {
	case "logs":
		if req.Lines < 1 || req.Lines > 500 {
			return payload, fmt.Errorf("lines harus antara 1 dan 500")
		}
		payload.Lines = req.Lines
	case "tunnel-install":
		if len(req.Token) < helperTokenMin || len(req.Token) > helperTokenMax || !helperTokenPattern.MatchString(req.Token) {
			return payload, errors.New("token tunnel tidak valid")
		}
		payload.Token = req.Token
	case "tunnel-logout":
		if req.Confirm != "RESET" {
			return payload, errors.New("konfirmasi tunnel-logout harus RESET")
		}
		payload.Confirm = req.Confirm
	case "restart-service":
		// Zero means "not sent" and would restart instantly, so it falls back to the contract default.
		delay := defaultRestartDelay
		if req.Delay != 0 {
			delay = req.Delay
		}
		payload.Delay = min(max(delay, 0), maxRestartDelay)
	}
	return payload, nil
}

func helperRequestID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// writeHelperRequest publishes the request by renaming a temp file, so the helper never reads a
// half-written request.
func writeHelperRequest(inDir, id string, payload helperRequest) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	tmp := filepath.Join(inDir, "."+id+".tmp")
	if err := os.WriteFile(tmp, data, helperRequestMode); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(inDir, id+".json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// waitHelperResult polls for the helper answer, then removes it. A silent helper and a cancelled
// request both report no answer: the request file stays in in/ for the helper to process.
func waitHelperResult(ctx context.Context, outDir, id string) (map[string]any, bool) {
	path := filepath.Join(outDir, id+".json")
	deadline := time.NewTimer(helperWait())
	defer deadline.Stop()
	ticker := time.NewTicker(helperPollInterval)
	defer ticker.Stop()
	for {
		if result, ok := readHelperResult(path); ok {
			return result, true
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-deadline.C:
			return nil, false
		case <-ticker.C:
		}
	}
}

func readHelperResult(path string) (map[string]any, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false
	}
	_ = os.Remove(path)
	return result, true
}
