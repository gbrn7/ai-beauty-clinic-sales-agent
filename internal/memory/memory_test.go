package memory

import (
	"fmt"
	"testing"
	"time"

	"github.com/raygbrn/ai-sales-agent-go/internal/db"
	"github.com/raygbrn/ai-sales-agent-go/internal/model"
)

func TestMemorySlidingWindow(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 10, 24)

	phone := "628111"
	for i := 0; i < 25; i++ {
		role := "user"
		if i%2 == 1 {
			role = "model"
		}
		if err := mem.Add(phone, role, fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("failed to add message: %v", err)
		}
	}

	history := mem.GetHistory(phone)
	if len(history) != 10 {
		t.Fatalf("expected 10 messages in sliding window, got %d", len(history))
	}

	// Verify chronological order: oldest in window first (msg-15), newest last (msg-24)
	for i, h := range history {
		expectedIdx := 15 + i
		expectedText := fmt.Sprintf("msg-%d", expectedIdx)
		expectedRole := "user"
		if expectedIdx%2 == 1 {
			expectedRole = "model"
		}
		if h.Role != expectedRole {
			t.Errorf("history[%d] expected role %s, got %s", i, expectedRole, h.Role)
		}
		if len(h.Parts) == 0 || h.Parts[0].Text != expectedText {
			t.Errorf("history[%d] expected text %s, got %v", i, expectedText, h.Parts)
		}
	}
}

func TestMemoryTakeover(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 20, 24)

	phone := "628222"
	// Default when contact record doesn't exist
	if !mem.IsAIEnabled(phone) {
		t.Errorf("default AI enabled should be true")
	}

	// Set AI enabled false on brand new contact
	if err := mem.SetAIEnabled(phone, false); err != nil {
		t.Fatalf("failed to set AI enabled false: %v", err)
	}
	if mem.IsAIEnabled(phone) {
		t.Errorf("expected AI enabled false after handover on new contact")
	}

	// Verify in database directly that AiEnabled is false
	var contact model.Contact
	if err := database.Where("phone = ?", phone).First(&contact).Error; err != nil {
		t.Fatalf("failed to find contact: %v", err)
	}
	if contact.AiEnabled != false {
		t.Errorf("expected contact.AiEnabled in db to be false, got true")
	}

	// Set AI enabled true on existing contact
	if err := mem.SetAIEnabled(phone, true); err != nil {
		t.Fatalf("failed to set AI enabled true: %v", err)
	}
	if !mem.IsAIEnabled(phone) {
		t.Errorf("expected AI enabled true after admin enables")
	}

	// Set AI enabled false again on existing contact
	if err := mem.SetAIEnabled(phone, false); err != nil {
		t.Fatalf("failed to set AI enabled false again: %v", err)
	}
	if mem.IsAIEnabled(phone) {
		t.Errorf("expected AI enabled false after handover on existing contact")
	}
}

func TestMemorySessionTTL(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 20, 1) // 1 hour TTL

	phone := "628333"
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := database.Create(&model.ConversationMessage{
		Phone:     phone,
		Role:      "user",
		Text:      "old message",
		CreatedAt: oldTime,
	}).Error; err != nil {
		t.Fatalf("failed to create old message: %v", err)
	}

	history := mem.GetHistory(phone)
	if len(history) != 0 {
		t.Errorf("expected empty history due to TTL expiry, got %d", len(history))
	}

	// When fresh message arrives, history is returned
	if err := mem.Add(phone, "user", "fresh message"); err != nil {
		t.Fatalf("failed to add fresh message: %v", err)
	}
	history = mem.GetHistory(phone)
	if len(history) != 2 {
		t.Errorf("expected 2 messages (within window and last message is fresh), got %d", len(history))
	}
}

func TestMemorySessionTTLDisabled(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 20, 0) // TTL 0 = disabled

	phone := "628444"
	oldTime := time.Now().Add(-100 * time.Hour)
	if err := database.Create(&model.ConversationMessage{
		Phone:     phone,
		Role:      "user",
		Text:      "very old message",
		CreatedAt: oldTime,
	}).Error; err != nil {
		t.Fatalf("failed to create old message: %v", err)
	}

	history := mem.GetHistory(phone)
	if len(history) != 1 {
		t.Errorf("expected 1 message when TTL is disabled, got %d", len(history))
	}
}

func TestMemoryContactName(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 10, 24)

	phone := "628555"

	// Initially nil
	if name := mem.GetContactName(phone); name != nil {
		t.Errorf("expected nil name for non-existent contact, got %v", *name)
	}

	// Empty name should be a no-op
	if err := mem.SetName(phone, ""); err != nil {
		t.Fatalf("unexpected error setting empty name: %v", err)
	}
	if name := mem.GetContactName(phone); name != nil {
		t.Errorf("expected nil name after setting empty name, got %v", *name)
	}

	// Set name on new contact
	if err := mem.SetName(phone, "Siti"); err != nil {
		t.Fatalf("failed to set name: %v", err)
	}
	name := mem.GetContactName(phone)
	if name == nil || *name != "Siti" {
		t.Errorf("expected name Siti, got %v", name)
	}

	// Update name on existing contact
	if err := mem.SetName(phone, "Siti Rahma"); err != nil {
		t.Fatalf("failed to update name: %v", err)
	}
	name = mem.GetContactName(phone)
	if name == nil || *name != "Siti Rahma" {
		t.Errorf("expected updated name Siti Rahma, got %v", name)
	}

	// Setting identical name is a no-op
	if err := mem.SetName(phone, "Siti Rahma"); err != nil {
		t.Fatalf("failed setting identical name: %v", err)
	}
	name = mem.GetContactName(phone)
	if name == nil || *name != "Siti Rahma" {
		t.Errorf("expected name Siti Rahma, got %v", name)
	}
}

func TestMemorySetNamePreservesAIEnabled(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 10, 24)

	phone := "628666"

	// 1. Disable AI on new contact
	if err := mem.SetAIEnabled(phone, false); err != nil {
		t.Fatalf("failed to disable AI: %v", err)
	}
	if mem.IsAIEnabled(phone) {
		t.Fatalf("expected AI disabled")
	}

	// 2. Set Name afterwards
	if err := mem.SetName(phone, "Budi"); err != nil {
		t.Fatalf("failed to set name: %v", err)
	}

	// 3. Name should be Budi AND AI should still be disabled!
	name := mem.GetContactName(phone)
	if name == nil || *name != "Budi" {
		t.Errorf("expected name Budi, got %v", name)
	}
	if mem.IsAIEnabled(phone) {
		t.Errorf("expected AI to remain disabled after SetName, but was enabled")
	}
}

func TestMemoryEmptyHistory(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 10, 24)

	history := mem.GetHistory("628999")
	if history == nil {
		t.Errorf("expected non-nil empty slice")
	}
	if len(history) != 0 {
		t.Errorf("expected 0 items, got %d", len(history))
	}
}

func TestMemoryFewerThanWindow(t *testing.T) {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	mem := NewMemory(database, 10, 24)

	phone := "628777"
	_ = mem.Add(phone, "user", "halo")
	_ = mem.Add(phone, "model", "halo juga")
	_ = mem.Add(phone, "user", "mau tanya harga")

	history := mem.GetHistory(phone)
	if len(history) != 3 {
		t.Fatalf("expected 3 items, got %d", len(history))
	}

	if history[0].Parts[0].Text != "halo" {
		t.Errorf("expected 1st message 'halo', got '%s'", history[0].Parts[0].Text)
	}
	if history[1].Parts[0].Text != "halo juga" {
		t.Errorf("expected 2nd message 'halo juga', got '%s'", history[1].Parts[0].Text)
	}
	if history[2].Parts[0].Text != "mau tanya harga" {
		t.Errorf("expected 3rd message 'mau tanya harga', got '%s'", history[2].Parts[0].Text)
	}
}
