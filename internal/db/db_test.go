package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/raygbrn/ai-sales-agent-go/internal/model"
)

func TestInitDBAndSeed(t *testing.T) {
	database, err := InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	if err := SeedDB(database); err != nil {
		t.Fatalf("failed to seed db: %v", err)
	}

	var count int64
	database.Model(&model.Treatment{}).Count(&count)
	if count != 10 {
		t.Errorf("expected 10 treatments seeded, got %d", count)
	}

	var hoursCount int64
	database.Model(&model.ClinicHours{}).Count(&hoursCount)
	if hoursCount != 7 {
		t.Errorf("expected 7 days clinic hours seeded, got %d", hoursCount)
	}

	var doctorCount int64
	database.Model(&model.DoctorSchedule{}).Count(&doctorCount)
	if doctorCount != 7 {
		t.Errorf("expected 7 doctor schedule entries, got %d", doctorCount)
	}

	var specialCount int64
	database.Model(&model.SpecialSchedule{}).Count(&specialCount)
	if specialCount != 5 {
		t.Errorf("expected 5 special schedule entries, got %d", specialCount)
	}
}

func TestSeedDB_Idempotency(t *testing.T) {
	database, err := InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	// First seeding
	if err := SeedDB(database); err != nil {
		t.Fatalf("failed to seed db first time: %v", err)
	}

	// Second seeding should not duplicate records
	if err := SeedDB(database); err != nil {
		t.Fatalf("failed to seed db second time: %v", err)
	}

	var count int64
	database.Model(&model.Treatment{}).Count(&count)
	if count != 10 {
		t.Errorf("expected 10 treatments after idempotent seed, got %d", count)
	}

	var hoursCount int64
	database.Model(&model.ClinicHours{}).Count(&hoursCount)
	if hoursCount != 7 {
		t.Errorf("expected 7 days clinic hours after idempotent seed, got %d", hoursCount)
	}
}

func TestInitDB_FilePragmas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_pragma.db")

	database, err := InitDB("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("failed to init file db: %v", err)
	}

	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
	}
	defer sqlDB.Close()

	var journalMode string
	row := sqlDB.QueryRow("PRAGMA journal_mode;")
	if err := row.Scan(&journalMode); err != nil {
		t.Fatalf("failed to read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal_mode 'wal', got %q", journalMode)
	}

	var busyTimeout int
	row = sqlDB.QueryRow("PRAGMA busy_timeout;")
	if err := row.Scan(&busyTimeout); err != nil {
		t.Fatalf("failed to read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("expected busy_timeout 5000, got %d", busyTimeout)
	}
}

func TestInitDB_DefaultPath(t *testing.T) {
	// sqlitePath == "" should default to clinic.db
	// Run in temp working directory so clinic.db doesn't pollute repo
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to tmpDir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	database, err := InitDB("sqlite://")
	if err != nil {
		t.Fatalf("failed to init db with empty sqlite path: %v", err)
	}
	sqlDB, _ := database.DB()
	if sqlDB != nil {
		_ = sqlDB.Close()
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "clinic.db")); os.IsNotExist(err) {
		t.Errorf("expected clinic.db to be created in working directory")
	}
}

func TestInitDB_ThreeSlashes(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to tmpDir: %v", err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	database, err := InitDB("sqlite:///clinic.db")
	if err != nil {
		t.Fatalf("failed to init db with sqlite:///clinic.db: %v", err)
	}
	sqlDB, _ := database.DB()
	if sqlDB != nil {
		_ = sqlDB.Close()
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "clinic.db")); os.IsNotExist(err) {
		t.Errorf("expected clinic.db created from sqlite:///clinic.db")
	}
}

func TestModels_CRUD(t *testing.T) {
	database, err := InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	if err := SeedDB(database); err != nil {
		t.Fatalf("failed to seed db: %v", err)
	}

	// 1. Create Contact
	name := "Alice"
	contact := model.Contact{
		Phone:       "628123456789",
		Name:        &name,
		AiEnabled:   true,
		IsReturning: false,
	}
	if err := database.Create(&contact).Error; err != nil {
		t.Fatalf("failed to create contact: %v", err)
	}

	var fetchedContact model.Contact
	if err := database.First(&fetchedContact, "phone = ?", "628123456789").Error; err != nil {
		t.Fatalf("failed to find contact: %v", err)
	}
	if *fetchedContact.Name != "Alice" || !fetchedContact.AiEnabled {
		t.Errorf("contact data mismatch: %+v", fetchedContact)
	}

	// 2. Create ConversationMessage
	msg := model.ConversationMessage{
		Phone: "628123456789",
		Role:  "user",
		Text:  "Halo, mau tanya promo treatment acne",
	}
	if err := database.Create(&msg).Error; err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	var fetchedMsg model.ConversationMessage
	if err := database.First(&fetchedMsg, "phone = ?", "628123456789").Error; err != nil {
		t.Fatalf("failed to find message: %v", err)
	}
	if fetchedMsg.Text != msg.Text || fetchedMsg.Role != "user" {
		t.Errorf("message data mismatch: %+v", fetchedMsg)
	}

	// 3. Create Booking
	var treatment model.Treatment
	if err := database.First(&treatment, "category = ?", "acne").Error; err != nil {
		t.Fatalf("failed to find treatment: %v", err)
	}

	booking := model.Booking{
		Code:            "GLW-20260623-ABCD",
		CustomerName:    "Alice",
		Phone:           "628123456789",
		TreatmentID:     treatment.ID,
		BookingDate:     "2026-06-23",
		BookingTime:     "14:00",
		DurationMinutes: model.TreatmentDurationMinutes,
		Status:          "pending",
		CreatedAt:       time.Now(),
	}
	if err := database.Create(&booking).Error; err != nil {
		t.Fatalf("failed to create booking: %v", err)
	}

	var fetchedBooking model.Booking
	if err := database.Preload("Treatment").First(&fetchedBooking, "code = ?", "GLW-20260623-ABCD").Error; err != nil {
		t.Fatalf("failed to fetch booking with preload: %v", err)
	}
	if fetchedBooking.Treatment.ID != treatment.ID {
		t.Errorf("expected treatment ID %d, got %d", treatment.ID, fetchedBooking.Treatment.ID)
	}
	if fetchedBooking.DurationMinutes != 90 {
		t.Errorf("expected duration minutes 90, got %d", fetchedBooking.DurationMinutes)
	}
}
