// Package config menyiapkan nilai yang harus ada sebelum server melayani permintaan.
//
// TempMail bisa dijalankan tanpa file .env: JWT_SECRET dan EMAIL_API_KEY dibuat sekali
// secara acak lalu disimpan di berkas 0600. Aturan lama "JWT_SECRET wajib" tetap berlaku
// dalam arti tidak ada nilai bawaan yang bisa ditebak; yang hilang hanyalah keharusan
// mengetik rahasia sebelum aplikasi bisa dinyalakan.
package config

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManagedSecrets adalah nama lingkungan yang dibuat otomatis saat kosong.
var ManagedSecrets = []string{"JWT_SECRET", "EMAIL_API_KEY"}

// DefaultSecretsPath menentukan lokasi berkas rahasia: SECRETS_FILE, atau di samping DB,
// atau ./data/secrets.env kalau keduanya tidak diset.
func DefaultSecretsPath() string {
	if path := strings.TrimSpace(os.Getenv("SECRETS_FILE")); path != "" {
		return path
	}
	if dbPath := strings.TrimSpace(os.Getenv("DB_PATH")); dbPath != "" {
		return filepath.Join(filepath.Dir(dbPath), "secrets.env")
	}
	return filepath.Join("data", "secrets.env")
}

// EnsureSecrets mengisi rahasia yang kosong: nilai dari environment selalu menang, nilai
// yang dibuat hanya ditulis untuk kunci yang memang belum ada nilainya.
//
// getenv dan setenv dipisah supaya bisa diuji tanpa menyentuh environment proses.
// Kembaliannya adalah daftar nama rahasia yang baru dibuat (kosong berarti tidak ada).
func EnsureSecrets(getenv func(string) string, setenv func(string, string) error, path string) ([]string, error) {
	stored, err := readSecrets(path)
	if err != nil {
		return nil, err
	}

	created := []string{}
	changed := false
	for _, key := range ManagedSecrets {
		if strings.TrimSpace(getenv(key)) != "" {
			// Operator sudah menyetel lewat environment: jangan menulis nilai itu ke berkas.
			continue
		}
		if value := strings.TrimSpace(stored[key]); value != "" {
			if err := setenv(key, value); err != nil {
				return created, fmt.Errorf("set %s dari %s: %w", key, path, err)
			}
			continue
		}
		value, err := generateSecret(key)
		if err != nil {
			return created, err
		}
		stored[key] = value
		changed = true
		created = append(created, key)
		if err := setenv(key, value); err != nil {
			return created, fmt.Errorf("set %s: %w", key, err)
		}
	}

	if !changed {
		return created, nil
	}
	if err := writeSecrets(path, stored); err != nil {
		// Gagal menyimpan berarti kunci akan berubah setiap restart: sesi panel mati dan
		// binding EMAIL_API_KEY di Worker tidak lagi cocok. Lebih baik berhenti dengan
		// pesan yang jelas daripada jalan setengah rusak.
		return created, fmt.Errorf("simpan rahasia ke %s: %w (set JWT_SECRET dan EMAIL_API_KEY lewat environment, atau perbaiki izin direktori)", path, err)
	}
	return created, nil
}

// generateSecret membuat nilai acak: kunci JWT 48 byte (base64url) dan API key 32 byte (hex).
func generateSecret(key string) (string, error) {
	size := 32
	switch key {
	case "JWT_SECRET":
		size = 48
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("acak %s: %w", key, err)
	}
	if key == "JWT_SECRET" {
		return base64.RawURLEncoding.EncodeToString(buf), nil
	}
	return hex.EncodeToString(buf), nil
}

// readSecrets membaca berkas KEY=value sederhana. Berkas yang belum ada bukan error.
func readSecrets(path string) (map[string]string, error) {
	out := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("baca %s: %w", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("baca %s: %w", path, err)
	}
	return out, nil
}

// writeSecrets menulis berkas 0600, dengan kunci terurut supaya berkasnya stabil.
func writeSecrets(path string, values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	builder.WriteString("# Dibuat otomatis oleh TempMail saat rahasia belum diset.\n")
	builder.WriteString("# Jangan dibagikan atau dimasukkan ke git. Boleh diedit; environment menang.\n")
	for _, key := range keys {
		builder.WriteString(key + "=" + values[key] + "\n")
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o600); err != nil {
		return err
	}
	// WriteFile hanya memakai mode itu saat membuat berkas baru; berkas lama bisa punya izin
	// lebih longgar, jadi izinnya ditegakkan lagi setiap kali.
	return os.Chmod(path, 0o600)
}
