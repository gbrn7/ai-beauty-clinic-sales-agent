package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port              string
	GeminiAPIKey      string
	GeminiModel       string
	GeminiThinking    string
	EvolutionAPIURL   string
	EvolutionAPIKey   string
	EvolutionInstance string
	DatabaseURL       string
	AIName            string
	CompanyName       string
	BufferSeconds     int
	MemoryWindow      int
	SessionTTLHours   int
	DashboardAPIKey   string
	R2Endpoint        string
	R2AccessKeyID     string
	R2SecretAccessKey string
	R2Bucket          string
	R2PublicURL       string
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		Port:              getEnv("PORT", "8000"),
		GeminiAPIKey:      os.Getenv("GEMINI_API_KEY"),
		GeminiModel:       getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		GeminiThinking:    getEnv("GEMINI_THINKING_LEVEL", "low"),
		EvolutionAPIURL:   getEnv("EVOLUTION_API_URL", "http://localhost:8080"),
		EvolutionAPIKey:   os.Getenv("EVOLUTION_API_KEY"),
		EvolutionInstance: getEnv("EVOLUTION_INSTANCE", "glowria"),
		DatabaseURL:       getEnv("DATABASE_URL", "sqlite://clinic.db"),
		AIName:            getEnv("AI_NAME", "Gita"),
		CompanyName:       getEnv("COMPANY_NAME", "Glowria Aesthetic Clinic"),
		BufferSeconds:     getEnvInt("BUFFER_SECONDS", 8),
		MemoryWindow:      getEnvInt("MEMORY_WINDOW", 20),
		SessionTTLHours:   getEnvInt("SESSION_TTL_HOURS", 24),
		DashboardAPIKey:   getEnv("DASHBOARD_API_KEY", "ganti-dengan-string-acak-panjang"),
		R2Endpoint:        os.Getenv("R2_ENDPOINT"),
		R2AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:          os.Getenv("R2_BUCKET"),
		R2PublicURL:       os.Getenv("R2_PUBLIC_URL"),
	}
}

func (c *Config) IsR2Enabled() bool {
	return c.R2Endpoint != "" && c.R2AccessKeyID != "" &&
		c.R2SecretAccessKey != "" && c.R2Bucket != "" && c.R2PublicURL != ""
}
