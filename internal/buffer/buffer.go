package buffer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"

	"github.com/raygbrn/ai-sales-agent-go/internal/agent"
	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/memory"
	"github.com/raygbrn/ai-sales-agent-go/internal/storage"
	"github.com/raygbrn/ai-sales-agent-go/internal/whatsapp"
)

// AgentRunner is an interface satisfied by *agent.Agent or custom test mocks.
type AgentRunner interface {
	RunAgent(ctx context.Context, history []*genai.Content, userMessage, phone string, images []agent.ImageData) (string, []*genai.Content, error)
}

// WhatsAppSender is an interface satisfied by *whatsapp.WhatsAppClient or custom test mocks.
type WhatsAppSender interface {
	SendTyping(phone string, duration time.Duration)
	SendText(phone, text string) error
	FetchMediaBase64(message map[string]any) ([]byte, string, error)
	SendReply(phone, reply string)
}

type DebounceBuffer struct {
	cfg         *config.Config
	mem         *memory.Memory
	agent       *agent.Agent
	agentRunner AgentRunner
	wa          *whatsapp.WhatsAppClient
	waSender    WhatsAppSender
	storage     storage.StorageService
	buffers     map[string][]*whatsapp.IncomingMessage
	timers      map[string]*time.Timer
	locks       map[string]*sync.Mutex
	mapMutex    sync.Mutex
}

func NewDebounceBuffer(
	cfg *config.Config,
	mem *memory.Memory,
	ag *agent.Agent,
	wa *whatsapp.WhatsAppClient,
	st storage.StorageService,
) *DebounceBuffer {
	return &DebounceBuffer{
		cfg:     cfg,
		mem:     mem,
		agent:   ag,
		wa:      wa,
		storage: st,
		buffers: make(map[string][]*whatsapp.IncomingMessage),
		timers:  make(map[string]*time.Timer),
		locks:   make(map[string]*sync.Mutex),
	}
}

// SetAgentRunner overrides the AgentRunner (useful for testing or custom agent loops).
func (db *DebounceBuffer) SetAgentRunner(ar AgentRunner) {
	db.agentRunner = ar
}

// SetWhatsAppSender overrides the WhatsAppSender (useful for testing or custom clients).
func (db *DebounceBuffer) SetWhatsAppSender(ws WhatsAppSender) {
	db.waSender = ws
}

func (db *DebounceBuffer) getLock(phone string) *sync.Mutex {
	db.mapMutex.Lock()
	defer db.mapMutex.Unlock()
	if _, ok := db.locks[phone]; !ok {
		db.locks[phone] = &sync.Mutex{}
	}
	return db.locks[phone]
}

func (db *DebounceBuffer) Add(msg *whatsapp.IncomingMessage) int {
	lock := db.getLock(msg.Phone)
	lock.Lock()
	defer lock.Unlock()

	db.mapMutex.Lock()
	db.buffers[msg.Phone] = append(db.buffers[msg.Phone], msg)
	count := len(db.buffers[msg.Phone])

	if t, exists := db.timers[msg.Phone]; exists {
		t.Stop()
	}

	phone := msg.Phone
	duration := time.Duration(db.cfg.BufferSeconds) * time.Second
	if duration <= 0 {
		duration = 8 * time.Second
	}
	db.timers[phone] = time.AfterFunc(duration, func() {
		db.processBufferedMessages(phone)
	})
	db.mapMutex.Unlock()

	return count
}

// Flush immediately cancels any scheduled debounce timer and processes buffered messages for the given phone.
func (db *DebounceBuffer) Flush(phone string) {
	db.mapMutex.Lock()
	if t, exists := db.timers[phone]; exists {
		t.Stop()
		delete(db.timers, phone)
	}
	db.mapMutex.Unlock()
	db.processBufferedMessages(phone)
}

func (db *DebounceBuffer) processBufferedMessages(phone string) {
	lock := db.getLock(phone)
	lock.Lock()
	defer lock.Unlock()

	db.mapMutex.Lock()
	pending := db.buffers[phone]
	delete(db.buffers, phone)
	delete(db.timers, phone)
	db.mapMutex.Unlock()

	if len(pending) == 0 {
		return
	}

	// Update pushName if present
	for i := len(pending) - 1; i >= 0; i-- {
		if pending[i].Name != "" {
			_ = db.mem.SetName(phone, pending[i].Name)
			break
		}
	}

	var texts []string
	var images []agent.ImageData
	var memoryLines []string

	for _, item := range pending {
		if item.Text != "" {
			texts = append(texts, item.Text)
		}
		if item.MediaType == "image" && item.RawData != nil {
			imgBytes, mime, err := db.fetchMediaBase64(item.RawData)
			if err == nil && len(imgBytes) > 0 {
				path, err := db.storage.SaveMedia(phone, imgBytes, mime)
				if err == nil {
					images = append(images, agent.ImageData{Bytes: imgBytes, MIMEType: mime})
					memoryLines = append(memoryLines, fmt.Sprintf("[image:%s]", path))
				}
			}
		}
	}

	combined := strings.Join(texts, "\n")
	memoryText := strings.TrimSpace(strings.Join(append([]string{combined}, memoryLines...), "\n"))
	if memoryText == "" {
		memoryText = "[customer mengirim gambar]"
	}

	log.Printf("[buffer] %s: %d pesan, %d gambar -> %q", phone, len(pending), len(images), memoryText)

	// Check human takeover switch
	if !db.mem.IsAIEnabled(phone) {
		log.Printf("[takeover] AI nonaktif untuk %s, pesan disimpan saja.", phone)
		_ = db.mem.Add(phone, "user", memoryText)
		return
	}

	history := db.mem.GetHistory(phone)
	ctx := context.Background()
	reply, _, err := db.runAgent(ctx, history, combined, phone, images)
	if err != nil {
		log.Printf("[error] gagal proses %s: %v", phone, err)
		return
	}

	_ = db.mem.Add(phone, "user", memoryText)
	_ = db.mem.Add(phone, "model", reply)

	if strings.Contains(reply, "[HANDOVER]") {
		_ = db.mem.SetAIEnabled(phone, false)
		log.Printf("[handover] AI dinonaktifkan untuk %s.", phone)
	}

	db.sendReply(phone, reply)
	log.Printf("[balas] %s: %q", phone, reply)
}

func (db *DebounceBuffer) runAgent(ctx context.Context, history []*genai.Content, userMessage, phone string, images []agent.ImageData) (string, []*genai.Content, error) {
	if db.agentRunner != nil {
		return db.agentRunner.RunAgent(ctx, history, userMessage, phone, images)
	}
	if db.agent != nil {
		return db.agent.RunAgent(ctx, history, userMessage, phone, images)
	}
	return "", nil, fmt.Errorf("no agent runner configured")
}

func (db *DebounceBuffer) fetchMediaBase64(message map[string]any) ([]byte, string, error) {
	if db.waSender != nil {
		return db.waSender.FetchMediaBase64(message)
	}
	if db.wa != nil {
		return db.wa.FetchMediaBase64(message)
	}
	return nil, "", fmt.Errorf("no whatsapp client configured")
}

func (db *DebounceBuffer) sendReply(phone, reply string) {
	if db.waSender != nil {
		db.waSender.SendReply(phone, reply)
		return
	}
	if db.wa != nil {
		db.wa.SendReply(phone, reply)
	}
}
