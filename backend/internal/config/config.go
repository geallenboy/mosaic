package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Storage  StorageConfig
	LLM      LLMConfig
	Auth     AuthConfig
}

type ServerConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type StorageConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type LLMConfig struct {
	Provider   string
	APIKey     string
	Model      string
	MaxTokens  int
	TimeoutSec int
}

type AuthConfig struct {
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
}

func Load() *Config {
	loadDotEnv()

	return &Config{
		Server: ServerConfig{
			Addr:         getEnv("SERVER_ADDR", ":8080"),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 120 * time.Second,
		},
		Database: DatabaseConfig{
			URL:             mustEnv("DATABASE_URL"),
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 5 * time.Minute,
		},
		Storage: StorageConfig{
			Endpoint:  getEnv("STORAGE_ENDPOINT", "http://localhost:9000"),
			AccessKey: mustEnv("STORAGE_ACCESS_KEY"),
			SecretKey: mustEnv("STORAGE_SECRET_KEY"),
			Bucket:    getEnv("STORAGE_BUCKET", "mosaic-dev"),
			UseSSL:    getEnv("STORAGE_USE_SSL", "false") == "true",
		},
		LLM: LLMConfig{
			Provider:   getEnv("LLM_PROVIDER", "openai"),
			APIKey:     mustEnv("LLM_API_KEY"),
			Model:      getEnv("LLM_MODEL", "gpt-4o"),
			MaxTokens:  4096,
			TimeoutSec: 120,
		},
		Auth: AuthConfig{
			JWTSecret:          mustEnv("AUTH_JWT_SECRET"),
			AccessTokenExpiry:  15 * time.Minute,
			RefreshTokenExpiry: 7 * 24 * time.Hour,
		},
	}
}

func loadDotEnv() {
	path, ok := findDotEnv()
	if !ok {
		return
	}
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseEnvLine(scanner.Text())
		if !ok {
			continue
		}
		if os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func findDotEnv() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(dir, ".env")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func parseEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	value = strings.Trim(value, `"'`)
	return key, value, true
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// mustEnv 在本地开发时允许空值（使用 .env.example 中的占位符）
// 生产环境应通过密钥管理服务确保所有必填变量已设置
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		// 开发阶段返回占位符，不 panic，便于冷启动
		return "MISSING_" + key
	}
	return v
}
