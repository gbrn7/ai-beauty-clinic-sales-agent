package memory

import (
	"time"

	"google.golang.org/genai"
	"gorm.io/gorm"

	"github.com/raygbrn/ai-sales-agent-go/internal/model"
)

type Memory struct {
	db       *gorm.DB
	window   int
	ttlHours int
}

func NewMemory(db *gorm.DB, window, ttlHours int) *Memory {
	return &Memory{
		db:       db,
		window:   window,
		ttlHours: ttlHours,
	}
}

func (m *Memory) GetHistory(phone string) []*genai.Content {
	var rows []model.ConversationMessage
	m.db.Where("phone = ?", phone).Order("created_at DESC, id DESC").Limit(m.window).Find(&rows)

	if len(rows) == 0 {
		return []*genai.Content{}
	}

	// Reverse to chronological order (oldest first, newest last)
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	if m.ttlHours > 0 {
		lastMsg := rows[len(rows)-1]
		if time.Since(lastMsg.CreatedAt) > time.Duration(m.ttlHours)*time.Hour {
			return []*genai.Content{}
		}
	}

	var contents []*genai.Content
	for _, row := range rows {
		contents = append(contents, &genai.Content{
			Role: row.Role,
			Parts: []*genai.Part{
				{Text: row.Text},
			},
		})
	}
	return contents
}

func (m *Memory) Add(phone, role, text string) error {
	msg := model.ConversationMessage{
		Phone: phone,
		Role:  role,
		Text:  text,
	}
	return m.db.Create(&msg).Error
}

func (m *Memory) IsAIEnabled(phone string) bool {
	var contact model.Contact
	if err := m.db.Where("phone = ?", phone).First(&contact).Error; err != nil {
		return true
	}
	return contact.AiEnabled
}

func (m *Memory) SetAIEnabled(phone string, enabled bool) error {
	var contact model.Contact
	if err := m.db.Where("phone = ?", phone).First(&contact).Error; err == nil {
		return m.db.Model(&contact).Update("ai_enabled", enabled).Error
	}
	return m.db.Model(&model.Contact{}).Create(map[string]interface{}{
		"phone":      phone,
		"ai_enabled": enabled,
	}).Error
}

func (m *Memory) SetName(phone, name string) error {
	if name == "" {
		return nil
	}
	var contact model.Contact
	if err := m.db.Where("phone = ?", phone).First(&contact).Error; err == nil {
		if contact.Name == nil || *contact.Name != name {
			return m.db.Model(&contact).Update("name", name).Error
		}
		return nil
	}
	return m.db.Create(&model.Contact{
		Phone:     phone,
		Name:      &name,
		AiEnabled: true,
	}).Error
}

func (m *Memory) GetContactName(phone string) *string {
	var contact model.Contact
	if err := m.db.Where("phone = ?", phone).First(&contact).Error; err != nil {
		return nil
	}
	return contact.Name
}
