package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScriptMatchesRepoWorker(t *testing.T) {
	repoScript, err := os.ReadFile(filepath.Join("..", "..", "cloudflare-worker", "src", "index.js"))
	if err != nil {
		t.Fatalf("read repo worker script: %v", err)
	}
	if Script() == "" {
		t.Fatal("Script() is empty")
	}
	if Script() != string(repoScript) {
		t.Fatalf("Script() differs from cloudflare-worker/src/index.js (%d vs %d bytes)", len(Script()), len(repoScript))
	}
}

func TestNameDefaults(t *testing.T) {
	t.Setenv("CF_WORKER_NAME", "")
	if Name() != "tempmailbyjhopanstore" {
		t.Fatalf("Name() = %q", Name())
	}

	t.Setenv("CF_WORKER_NAME", "custom-worker")
	if Name() != "custom-worker" {
		t.Fatalf("Name() = %q", Name())
	}
}

func TestCompatDateDefaults(t *testing.T) {
	t.Setenv("CF_WORKER_COMPAT_DATE", "")
	if CompatDate() != "2024-01-01" {
		t.Fatalf("CompatDate() = %q", CompatDate())
	}

	t.Setenv("CF_WORKER_COMPAT_DATE", "2025-06-30")
	if CompatDate() != "2025-06-30" {
		t.Fatalf("CompatDate() = %q", CompatDate())
	}
}
