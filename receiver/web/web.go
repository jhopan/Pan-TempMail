package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"

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
func ConfigHandler(c *gin.Context) {
	domains := []string{}
	domainsStr := os.Getenv("DOMAINS")
	if domainsStr != "" {
		for _, d := range strings.Split(domainsStr, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				domains = append(domains, d)
			}
		}
	}
	if len(domains) == 0 {
		domains = []string{"example.com"}
	}

	siteName := os.Getenv("SITE_NAME")
	if siteName == "" {
		siteName = "TempMail"
	}

	c.JSON(http.StatusOK, gin.H{
		"domains":   domains,
		"site_name": siteName,
	})
}
