package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvExportSyntax(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("DEEPSEEK_MODEL", "")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("SERVER_PORT", "")
	if err := os.WriteFile(envPath, []byte("export DEEPSEEK_API_KEY=test-key\nexport DEEPSEEK_MODEL=deepseek-v4-pro\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DeepSeek.APIKey != "test-key" {
		t.Fatalf("API key = %q", cfg.DeepSeek.APIKey)
	}
	if cfg.DeepSeek.Model != "deepseek-v4-pro" {
		t.Fatalf("model = %q", cfg.DeepSeek.Model)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("addr = %q", cfg.Addr)
	}
}

func TestLoadUsesServerPort(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("SERVER_PORT", "")
	if err := os.WriteFile(envPath, []byte("export SERVER_PORT=9090\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9090" {
		t.Fatalf("addr = %q", cfg.Addr)
	}
}

func TestLoadServerAddrOverridesServerPort(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	t.Setenv("SERVER_ADDR", "")
	t.Setenv("SERVER_PORT", "")
	if err := os.WriteFile(envPath, []byte("export SERVER_PORT=9090\nexport SERVER_ADDR=127.0.0.1:7070\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:7070" {
		t.Fatalf("addr = %q", cfg.Addr)
	}
}
