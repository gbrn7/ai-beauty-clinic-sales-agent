package config

import (
	"os"
	"testing"
)

func TestConfigLoadDefaults(t *testing.T) {
	os.Clearenv()
	cfg := Load()
	if cfg.Port != "8000" {
		t.Errorf("expected default Port 8000, got %s", cfg.Port)
	}
	if cfg.BufferSeconds != 8 {
		t.Errorf("expected default BufferSeconds 8, got %d", cfg.BufferSeconds)
	}
	if cfg.MemoryWindow != 20 {
		t.Errorf("expected default MemoryWindow 20, got %d", cfg.MemoryWindow)
	}
	if cfg.AIName != "Gita" {
		t.Errorf("expected default AIName Gita, got %s", cfg.AIName)
	}
	if cfg.GeminiModel != "gemini-2.5-flash" {
		t.Errorf("expected default GeminiModel gemini-2.5-flash, got %s", cfg.GeminiModel)
	}
	if cfg.GeminiThinking != "low" {
		t.Errorf("expected default GeminiThinking low, got %s", cfg.GeminiThinking)
	}
	if cfg.EvolutionAPIURL != "http://localhost:8080" {
		t.Errorf("expected default EvolutionAPIURL http://localhost:8080, got %s", cfg.EvolutionAPIURL)
	}
	if cfg.EvolutionInstance != "glowria" {
		t.Errorf("expected default EvolutionInstance glowria, got %s", cfg.EvolutionInstance)
	}
	if cfg.DatabaseURL != "sqlite://clinic.db" {
		t.Errorf("expected default DatabaseURL sqlite://clinic.db, got %s", cfg.DatabaseURL)
	}
	if cfg.CompanyName != "Glowria Aesthetic Clinic" {
		t.Errorf("expected default CompanyName Glowria Aesthetic Clinic, got %s", cfg.CompanyName)
	}
	if cfg.SessionTTLHours != 24 {
		t.Errorf("expected default SessionTTLHours 24, got %d", cfg.SessionTTLHours)
	}
	if cfg.DashboardAPIKey != "ganti-dengan-string-acak-panjang" {
		t.Errorf("expected default DashboardAPIKey ganti-dengan-string-acak-panjang, got %s", cfg.DashboardAPIKey)
	}
	if cfg.IsR2Enabled() {
		t.Errorf("expected IsR2Enabled to be false with defaults, got true")
	}
}

func TestConfigLoadCustomEnv(t *testing.T) {
	os.Clearenv()
	os.Setenv("PORT", "9000")
	os.Setenv("GEMINI_API_KEY", "test-gemini-key")
	os.Setenv("GEMINI_MODEL", "gemini-1.5-pro")
	os.Setenv("GEMINI_THINKING_LEVEL", "high")
	os.Setenv("EVOLUTION_API_URL", "https://evo.example.com")
	os.Setenv("EVOLUTION_API_KEY", "test-evo-key")
	os.Setenv("EVOLUTION_INSTANCE", "clinic-inst")
	os.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/clinic")
	os.Setenv("AI_NAME", "Maya")
	os.Setenv("COMPANY_NAME", "Test Clinic")
	os.Setenv("BUFFER_SECONDS", "15")
	os.Setenv("MEMORY_WINDOW", "30")
	os.Setenv("SESSION_TTL_HOURS", "48")
	os.Setenv("DASHBOARD_API_KEY", "secret-dashboard-key")
	os.Setenv("R2_ENDPOINT", "https://r2.cloudflarestorage.com")
	os.Setenv("R2_ACCESS_KEY_ID", "r2-key")
	os.Setenv("R2_SECRET_ACCESS_KEY", "r2-secret")
	os.Setenv("R2_BUCKET", "media-bucket")
	os.Setenv("R2_PUBLIC_URL", "https://pub.example.com")

	cfg := Load()
	if cfg.Port != "9000" {
		t.Errorf("expected Port 9000, got %s", cfg.Port)
	}
	if cfg.GeminiAPIKey != "test-gemini-key" {
		t.Errorf("expected GeminiAPIKey test-gemini-key, got %s", cfg.GeminiAPIKey)
	}
	if cfg.GeminiModel != "gemini-1.5-pro" {
		t.Errorf("expected GeminiModel gemini-1.5-pro, got %s", cfg.GeminiModel)
	}
	if cfg.GeminiThinking != "high" {
		t.Errorf("expected GeminiThinking high, got %s", cfg.GeminiThinking)
	}
	if cfg.EvolutionAPIURL != "https://evo.example.com" {
		t.Errorf("expected EvolutionAPIURL https://evo.example.com, got %s", cfg.EvolutionAPIURL)
	}
	if cfg.EvolutionAPIKey != "test-evo-key" {
		t.Errorf("expected EvolutionAPIKey test-evo-key, got %s", cfg.EvolutionAPIKey)
	}
	if cfg.EvolutionInstance != "clinic-inst" {
		t.Errorf("expected EvolutionInstance clinic-inst, got %s", cfg.EvolutionInstance)
	}
	if cfg.DatabaseURL != "postgres://user:pass@localhost:5432/clinic" {
		t.Errorf("expected DatabaseURL postgres://..., got %s", cfg.DatabaseURL)
	}
	if cfg.AIName != "Maya" {
		t.Errorf("expected AIName Maya, got %s", cfg.AIName)
	}
	if cfg.CompanyName != "Test Clinic" {
		t.Errorf("expected CompanyName Test Clinic, got %s", cfg.CompanyName)
	}
	if cfg.BufferSeconds != 15 {
		t.Errorf("expected BufferSeconds 15, got %d", cfg.BufferSeconds)
	}
	if cfg.MemoryWindow != 30 {
		t.Errorf("expected MemoryWindow 30, got %d", cfg.MemoryWindow)
	}
	if cfg.SessionTTLHours != 48 {
		t.Errorf("expected SessionTTLHours 48, got %d", cfg.SessionTTLHours)
	}
	if cfg.DashboardAPIKey != "secret-dashboard-key" {
		t.Errorf("expected DashboardAPIKey secret-dashboard-key, got %s", cfg.DashboardAPIKey)
	}
	if !cfg.IsR2Enabled() {
		t.Errorf("expected IsR2Enabled to be true, got false")
	}
}

func TestConfigIsR2EnabledPartial(t *testing.T) {
	cfg := &Config{
		R2Endpoint:    "https://r2.cloudflarestorage.com",
		R2AccessKeyID: "key",
		// missing secret, bucket, publicURL
	}
	if cfg.IsR2Enabled() {
		t.Errorf("expected IsR2Enabled to be false when partially configured, got true")
	}
}
