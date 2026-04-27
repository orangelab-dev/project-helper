package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type DeepSeekConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

type Config struct {
	Addr         string
	DatabasePath string
	ReposDir     string
	ReportsDir   string
	LogPath      string
	FrontendURL  string
	DeepSeek     DeepSeekConfig
}

func Load(envPath string) (Config, error) {
	_ = loadDotEnv(envPath)

	cfg := Config{
		Addr:         envOrDefault("SERVER_ADDR", ":8080"),
		DatabasePath: envOrDefault("DATABASE_PATH", filepath.Join("data", "project-helper.db")),
		ReposDir:     envOrDefault("REPOS_DIR", filepath.Join("data", "repos")),
		ReportsDir:   envOrDefault("REPORTS_DIR", filepath.Join("data", "reports")),
		LogPath:      envOrDefault("LOG_PATH", filepath.Join("data", "logs", "server.log")),
		FrontendURL:  envOrDefault("FRONTEND_URL", "http://localhost:5173"),
		DeepSeek: DeepSeekConfig{
			APIKey:  os.Getenv("DEEPSEEK_API_KEY"),
			Model:   envOrDefault("DEEPSEEK_MODEL", "deepseek-v4-pro"),
			BaseURL: strings.TrimRight(envOrDefault("DEEPSEEK_BASE_URL", "https://api.deepseek.com"), "/"),
		},
	}
	return cfg, nil
}

func loadDotEnv(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
	return scanner.Err()
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
