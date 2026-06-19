package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsDotEnvFromParentDirectory(t *testing.T) {
	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	if err := os.Mkdir(backendDir, 0o755); err != nil {
		t.Fatalf("mkdir backend: %v", err)
	}
	envContent := []byte(`
DATABASE_URL=postgres://mosaic:mosaic@localhost:15432/mosaic?sslmode=disable
LLM_API_KEY=env-file-key
AUTH_JWT_SECRET=env-file-secret-at-least-32-chars
STORAGE_ACCESS_KEY=mosaic
STORAGE_SECRET_KEY=mosaic_secret
`)
	if err := os.WriteFile(filepath.Join(root, ".env"), envContent, 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	if err := os.Chdir(backendDir); err != nil {
		t.Fatalf("chdir backend: %v", err)
	}

	t.Setenv("DATABASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("AUTH_JWT_SECRET", "")
	t.Setenv("STORAGE_ACCESS_KEY", "")
	t.Setenv("STORAGE_SECRET_KEY", "")

	cfg := Load()

	if cfg.LLM.APIKey != "env-file-key" {
		t.Fatalf("expected LLM_API_KEY from .env, got %q", cfg.LLM.APIKey)
	}
	if cfg.Database.URL == "" || cfg.Database.URL == "MISSING_DATABASE_URL" {
		t.Fatalf("expected DATABASE_URL from .env, got %q", cfg.Database.URL)
	}
}

func TestExistingEnvironmentWinsOverDotEnv(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("LLM_API_KEY=env-file-key\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir root: %v", err)
	}

	t.Setenv("LLM_API_KEY", "process-env-key")
	cfg := Load()

	if cfg.LLM.APIKey != "process-env-key" {
		t.Fatalf("expected process environment to win, got %q", cfg.LLM.APIKey)
	}
}
