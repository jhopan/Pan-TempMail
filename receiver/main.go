package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"receiver/api"
	"receiver/cleanup"
	"receiver/config"
	"receiver/database"
	"receiver/handlers"
	"receiver/smtp"

	"github.com/joho/godotenv"
)

// version diisi saat build: -ldflags "-X main.version=v1.2.3". Dipakai untuk memastikan
// pembaruan benar-benar mendarat dan untuk laporan di panel.
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "cetak versi lalu keluar")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	// Load .env if present (optional — env vars can be set directly)
	_ = godotenv.Load()

	// Rahasia bisa dibuat otomatis: aplikasi tetap bisa dinyalakan tanpa .env, dan
	// nilainya disimpan 0600 supaya sesi panel serta binding Worker tidak berubah
	// setiap restart. Environment selalu menang.
	secretsPath := config.DefaultSecretsPath()
	generated, err := config.EnsureSecrets(os.Getenv, os.Setenv, secretsPath)
	if err != nil {
		log.Fatalf("secrets: %v", err)
	}
	if len(generated) > 0 {
		log.Printf("Rahasia dibuat otomatis (%s), disimpan di %s dengan izin 0600. Set lewat environment untuk menimpa.",
			strings.Join(generated, ", "), secretsPath)
	}

	// Panel melaporkan versi biner ini supaya pembaruan bisa diverifikasi dari browser.
	handlers.SetVersion(version)

	// Startup banner
	siteName := os.Getenv("SITE_NAME")
	if siteName == "" {
		siteName = "Pan-TempMail"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	banner := "\n==============================================\n  " + siteName + " " + version + "\n        Credit JhopanStore\n==============================================\n"
	log.Println(banner)

	// Connect to the SQLite database (one file, WAL)
	if err := database.Connect(); err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	// Start cleanup worker for expired inboxes and old messages
	cleanup.StartCleanupWorker(database.Conn())

	// Start built-in SMTP server (replaces Maddy)
	smtp.Start()

	// Setup HTTP API + static frontend
	r := api.SetupRouter()

	// Graceful shutdown
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE needs no write timeout
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("HTTP server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	// Wait for interrupt signal (SIGINT or SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Give 10 seconds for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("http: forced shutdown: %v", err)
	}

	log.Println("Server exited")
}
