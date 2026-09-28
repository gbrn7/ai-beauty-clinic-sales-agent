package buffer

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/genai"

	"github.com/raygbrn/ai-sales-agent-go/internal/agent"
	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/db"
	"github.com/raygbrn/ai-sales-agent-go/internal/memory"
	"github.com/raygbrn/ai-sales-agent-go/internal/whatsapp"
)

type mockAgentRunner struct {
	mu           sync.Mutex
	calls        int
	userMessages []string
	phones       []string
	images       [][]agent.ImageData
	returnReply  string
	returnErr    error
}

func (m *mockAgentRunner) RunAgent(ctx context.Context, history []*genai.Content, userMessage, phone string, images []agent.ImageData) (string, []*genai.Content, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	m.userMessages = append(m.userMessages, userMessage)
	m.phones = append(m.phones, phone)
	m.images = append(m.images, images)
	return m.returnReply, history, m.returnErr
}

type mockWhatsAppSender struct {
	mu            sync.Mutex
	typingDur     map[string]time.Duration
	textsSent     map[string][]string
	replies       map[string][]string
	mediaToReturn []byte
	mimeToReturn  string
	mediaErr      error
}

func newMockWhatsAppSender() *mockWhatsAppSender {
	return &mockWhatsAppSender{
		typingDur: make(map[string]time.Duration),
		textsSent: make(map[string][]string),
		replies:   make(map[string][]string),
	}
}

func (m *mockWhatsAppSender) SendTyping(phone string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typingDur[phone] = duration
}

func (m *mockWhatsAppSender) SendText(phone, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.textsSent[phone] = append(m.textsSent[phone], text)
	return nil
}

func (m *mockWhatsAppSender) FetchMediaBase64(message map[string]any) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mediaToReturn, m.mimeToReturn, m.mediaErr
}

func (m *mockWhatsAppSender) SendReply(phone, reply string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.replies[phone] = append(m.replies[phone], reply)
}

type mockStorageService struct {
	mu         sync.Mutex
	saved      map[string][]byte
	mimes      map[string]string
	returnPath string
}

func newMockStorageService() *mockStorageService {
	return &mockStorageService{
		saved:      make(map[string][]byte),
		mimes:      make(map[string]string),
		returnPath: "media/test_image.jpg",
	}
}

func (m *mockStorageService) SaveMedia(phone string, data []byte, mime string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saved[phone] = data
	m.mimes[phone] = mime
	return m.returnPath, nil
}

func (m *mockStorageService) GetMedia(fname string) ([]byte, string, error) {
	return nil, "", nil
}

func setupTestDB(t *testing.T) *memory.Memory {
	database, err := db.InitDB("sqlite://file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	return memory.NewMemory(database, 20, 24)
}

func TestDebounceBuffer_AddAndDebounce(t *testing.T) {
	mem := setupTestDB(t)
	cfg := &config.Config{
		BufferSeconds: 1, // 1 second debounce
	}
	ag := &mockAgentRunner{returnReply: "Halo kak, ada yang bisa dibantu?"}
	wa := newMockWhatsAppSender()
	st := newMockStorageService()

	buf := NewDebounceBuffer(cfg, mem, nil, nil, st)
	buf.SetAgentRunner(ag)
	buf.SetWhatsAppSender(wa)

	phone := "6281200000001"
	c1 := buf.Add(&whatsapp.IncomingMessage{
		Phone: phone,
		Name:  "Budi",
		Text:  "Halo",
	})
	if c1 != 1 {
		t.Errorf("expected count 1, got %d", c1)
	}

	time.Sleep(200 * time.Millisecond)

	c2 := buf.Add(&whatsapp.IncomingMessage{
		Phone: phone,
		Name:  "Budi Santoso",
		Text:  "Mau tanya promo facial",
	})
	if c2 != 2 {
		t.Errorf("expected count 2, got %d", c2)
	}

	// Wait for debounce timer to fire (1s + margin)
	time.Sleep(1200 * time.Millisecond)

	ag.mu.Lock()
	calls := ag.calls
	userMsgs := append([]string{}, ag.userMessages...)
	ag.mu.Unlock()

	if calls != 1 {
		t.Fatalf("expected exactly 1 agent call, got %d", calls)
	}

	expectedCombined := "Halo\nMau tanya promo facial"
	if userMsgs[0] != expectedCombined {
		t.Errorf("expected combined user message %q, got %q", expectedCombined, userMsgs[0])
	}

	// Verify reply sent to WhatsApp
	wa.mu.Lock()
	replies := wa.replies[phone]
	wa.mu.Unlock()

	if len(replies) != 1 || replies[0] != "Halo kak, ada yang bisa dibantu?" {
		t.Errorf("expected 1 reply sent to WA, got %v", replies)
	}

	// Verify name was updated in memory
	name := mem.GetContactName(phone)
	if name == nil || *name != "Budi Santoso" {
		t.Errorf("expected contact name 'Budi Santoso', got %v", name)
	}

	// Verify conversation saved to memory
	history := mem.GetHistory(phone)
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries (user + model), got %d", len(history))
	}
	if history[0].Role != "user" || history[0].Parts[0].Text != expectedCombined {
		t.Errorf("expected user history %q, got %q", expectedCombined, history[0].Parts[0].Text)
	}
	if history[1].Role != "model" || history[1].Parts[0].Text != "Halo kak, ada yang bisa dibantu?" {
		t.Errorf("expected model history 'Halo kak, ada yang bisa dibantu?', got %q", history[1].Parts[0].Text)
	}
}

func TestDebounceBuffer_HumanTakeover(t *testing.T) {
	mem := setupTestDB(t)
	cfg := &config.Config{
		BufferSeconds: 1,
	}
	ag := &mockAgentRunner{returnReply: "Ini tidak boleh terkirim"}
	wa := newMockWhatsAppSender()
	st := newMockStorageService()

	buf := NewDebounceBuffer(cfg, mem, nil, nil, st)
	buf.SetAgentRunner(ag)
	buf.SetWhatsAppSender(wa)

	phone := "6281200000002"
	// Disable AI (human takeover active)
	_ = mem.SetAIEnabled(phone, false)

	buf.Add(&whatsapp.IncomingMessage{
		Phone: phone,
		Name:  "Customer",
		Text:  "Tolong admin balas",
	})

	time.Sleep(1200 * time.Millisecond)

	ag.mu.Lock()
	calls := ag.calls
	ag.mu.Unlock()

	if calls != 0 {
		t.Fatalf("expected 0 agent calls when takeover active, got %d", calls)
	}

	wa.mu.Lock()
	replies := wa.replies[phone]
	wa.mu.Unlock()

	if len(replies) != 0 {
		t.Fatalf("expected 0 WA replies when takeover active, got %v", replies)
	}

	// Message should still be saved to memory for admin to see
	history := mem.GetHistory(phone)
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry (user message saved), got %d", len(history))
	}
	if history[0].Parts[0].Text != "Tolong admin balas" {
		t.Errorf("expected user message saved, got %q", history[0].Parts[0].Text)
	}
}

func TestDebounceBuffer_HandoverMarker(t *testing.T) {
	mem := setupTestDB(t)
	cfg := &config.Config{
		BufferSeconds: 1,
	}
	ag := &mockAgentRunner{
		returnReply: "Baik kak, admin kami akan segera membantu ya 🙏 [WAIT][HANDOVER]",
	}
	wa := newMockWhatsAppSender()
	st := newMockStorageService()

	buf := NewDebounceBuffer(cfg, mem, nil, nil, st)
	buf.SetAgentRunner(ag)
	buf.SetWhatsAppSender(wa)

	phone := "6281200000003"
	// AI initially enabled
	if !mem.IsAIEnabled(phone) {
		t.Fatalf("expected AI enabled initially")
	}

	buf.Add(&whatsapp.IncomingMessage{
		Phone: phone,
		Name:  "Customer Handover",
		Text:  "Mau komplain dong",
	})

	time.Sleep(1200 * time.Millisecond)

	// AI should now be disabled after [HANDOVER] marker
	if mem.IsAIEnabled(phone) {
		t.Errorf("expected AI to be disabled after [HANDOVER] reply")
	}

	// Reply should still be sent to customer
	wa.mu.Lock()
	replies := wa.replies[phone]
	wa.mu.Unlock()

	if len(replies) != 1 {
		t.Fatalf("expected reply sent to customer, got %v", replies)
	}
}

func TestDebounceBuffer_ImageHandling(t *testing.T) {
	mem := setupTestDB(t)
	cfg := &config.Config{
		BufferSeconds: 1,
	}
	ag := &mockAgentRunner{returnReply: "Terima kasih fotonya kak"}
	wa := newMockWhatsAppSender()
	wa.mediaToReturn = []byte("binary-image-data")
	wa.mimeToReturn = "image/jpeg"
	st := newMockStorageService()

	buf := NewDebounceBuffer(cfg, mem, nil, nil, st)
	buf.SetAgentRunner(ag)
	buf.SetWhatsAppSender(wa)

	phone := "6281200000004"
	buf.Add(&whatsapp.IncomingMessage{
		Phone:     phone,
		Name:      "Customer Image",
		Text:      "Ini foto keluhan saya",
		MediaType: "image",
		Mime:      "image/jpeg",
		RawData:   map[string]any{"id": "msg-img-123"},
	})

	time.Sleep(1200 * time.Millisecond)

	ag.mu.Lock()
	imagesPassed := ag.images
	ag.mu.Unlock()

	if len(imagesPassed) != 1 || len(imagesPassed[0]) != 1 {
		t.Fatalf("expected 1 image passed to agent, got %v", imagesPassed)
	}
	if string(imagesPassed[0][0].Bytes) != "binary-image-data" {
		t.Errorf("expected image data 'binary-image-data', got %q", string(imagesPassed[0][0].Bytes))
	}
	if imagesPassed[0][0].MIMEType != "image/jpeg" {
		t.Errorf("expected mime 'image/jpeg', got %q", imagesPassed[0][0].MIMEType)
	}

	// Check storage SaveMedia was called
	st.mu.Lock()
	savedData := st.saved[phone]
	st.mu.Unlock()
	if string(savedData) != "binary-image-data" {
		t.Errorf("expected storage saved data 'binary-image-data', got %q", string(savedData))
	}

	// Check memory text includes [image:media/test_image.jpg]
	history := mem.GetHistory(phone)
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	expectedUserMem := "Ini foto keluhan saya\n[image:media/test_image.jpg]"
	if history[0].Parts[0].Text != expectedUserMem {
		t.Errorf("expected user memory %q, got %q", expectedUserMem, history[0].Parts[0].Text)
	}
}
