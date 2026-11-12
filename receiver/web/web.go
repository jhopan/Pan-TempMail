package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"receiver/database"
	"receiver/repository"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFiles embed.FS

// StaticFS returns the embedded static files as an http.FileSystem.
func StaticFS() http.FileSystem {
	sub, _ := fs.Sub(staticFiles, "static")
	return http.FS(sub)
}

// ConfigHandler returns public configuration (domains, site name) for the frontend.
// Domains come from the Cloudflare records managed by the admin panel, so adding a domain
// shows up without restarting the service; DOMAINS from the env file is the fallback.
func ConfigHandler(c *gin.Context) {
	domains := []string{}
	if db := database.Conn(); db != nil {
		if listed, err := repository.ListedDomains(db); err == nil {
			domains = listed
		}
	}
	if len(domains) == 0 {
		domainsStr := os.Getenv("DOMAINS")
		if domainsStr != "" {
			for _, d := range strings.Split(domainsStr, ",") {
				d = strings.TrimSpace(d)
				if d != "" {
					domains = append(domains, d)
				}
			}
		}
	}
	if len(domains) == 0 {
		domains = []string{"example.com"}
	}

	siteName := os.Getenv("SITE_NAME")
	if siteName == "" {
		siteName = "Pan-TempMail"
	}

	c.JSON(http.StatusOK, gin.H{
		"domains":   domains,
		"site_name": siteName,
	})
}
