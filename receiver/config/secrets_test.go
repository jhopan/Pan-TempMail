package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeEnv(initial map[string]string) (func(string) string, func(string, string) error, map[string]string) {
	env := map[string]string{}
	for key, value := range initial {
		env[key] = value
	}
	get := func(key string) string { return env[key] }
	set := func(key, value string) error { env[key] = value; return nil }
	return get, set, env
}

func TestEnsureSecretsGeneratesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.env")
	get, set, env := fakeEnv(nil)

	created, err := EnsureSecrets(get, set, path)
	if err != nil {
		t.Fatalf("ensure secrets: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("created = %v, ingin dua rahasia", created)
	}
	for _, key := range ManagedSecrets {
		if strings.TrimSpace(env[key]) == "" {
			t.Fatalf("%s tidak diset di environment", key)
		}
	}
	if len(env["JWT_SECRET"]) != 64 {
		t.Errorf("panjang JWT_SECRET = %d, ingin 64 (48 byte base64url)", len(env["JWT_SECRET"]))
	}
	if len(env["EMAIL_API_KEY"]) != 64 {
		t.Errorf("panjang EMAIL_API_KEY = %d, ingin 64 (32 byte hex)", len(env["EMAIL_API_KEY"]))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat rahasia: %v", err)
	}
	if runtime.GOOS != "windows" {
		// Windows melaporkan mode sintetis (0666), jadi izin 0600 hanya diperiksa di
		// sistem tempat izin itu benar-benar berlaku.
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("izin berkas = %o, ingin 600", perm)
		}
	}

	// Panggilan kedua memakai berkas yang sama dan tidak membuat ulang.
	get2, set2, env2 := fakeEnv(nil)
	created2, err := EnsureSecrets(get2, set2, path)
	if err != nil {
		t.Fatalf("ensure kedua: %v", err)
	}
	if len(created2) != 0 {
		t.Errorf("created kedua = %v, ingin kosong", created2)
	}
	if env2["JWT_SECRET"] != env["JWT_SECRET"] || env2["EMAIL_API_KEY"] != env["EMAIL_API_KEY"] {
		t.Errorf("nilai berubah antar pemanggilan: %v vs %v", env2, env)
	}
}

func TestEnsureSecretsPrefersEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.env")
	get, set, env := fakeEnv(map[string]string{"JWT_SECRET": "dari-environment"})

	created, err := EnsureSecrets(get, set, path)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(created) != 1 || created[0] != "EMAIL_API_KEY" {
		t.Fatalf("created = %v, ingin hanya EMAIL_API_KEY", created)
	}
	if env["JWT_SECRET"] != "dari-environment" {
		t.Errorf("JWT_SECRET ditimpa: %q", env["JWT_SECRET"])
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("baca berkas: %v", err)
	}
	if strings.Contains(string(body), "dari-environment") {
		t.Errorf("nilai dari environment tidak boleh ditulis ke berkas: %s", body)
	}
	if !strings.Contains(string(body), "EMAIL_API_KEY=") {
		t.Errorf("EMAIML_API_KEY tidak ada di berkas: %s", body)
	}
}

func TestEnsureSecretsKeepsManualValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.env")
	if err := os.WriteFile(path, []byte("JWT_SECRET=manual-dari-berkas\n"), 0o600); err != nil {
		t.Fatalf("tulis berkas: %v", err)
	}
	get, set, env := fakeEnv(nil)

	created, err := EnsureSecrets(get, set, path)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(created) != 1 || created[0] != "EMAIL_API_KEY" {
		t.Fatalf("created = %v, ingin hanya EMAIL_API_KEY", created)
	}
	if env["JWT_SECRET"] != "manual-dari-berkas" {
		t.Errorf("JWT_SECRET dari berkas tidak dipakai: %q", env["JWT_SECRET"])
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("baca berkas: %v", err)
	}
	if !strings.Contains(string(body), "JWT_SECRET=manual-dari-berkas") {
		t.Errorf("nilai manual hilang: %s", body)
	}
}

func TestEnsureSecretsReportsUnwritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tidak-ada", "secrets.env")
	get, set, _ := fakeEnv(nil)
	// Direktori dibuat sendiri, jadi paksa gagal dengan menaruh berkas di path yang
	// sebenarnya sebuah direktori.
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("siapkan direktori: %v", err)
	}
	if _, err := EnsureSecrets(get, set, path); err == nil {
		t.Fatal("ingin error saat berkas tidak bisa ditulis")
	}
}
