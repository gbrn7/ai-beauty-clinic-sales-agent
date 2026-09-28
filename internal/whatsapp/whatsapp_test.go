package whatsapp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/raygbrn/ai-sales-agent-go/internal/config"
)

func TestExtractIncomingText(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"remoteJid": "628123456789@s.whatsapp.net",
				"fromMe":    false,
			},
			"pushName": "Budi",
			"message": map[string]any{
				"conversation": "Halo apa kabar?",
			},
		},
	}

	msg := ExtractIncoming(payload)
	if msg == nil {
		t.Fatalf("expected message parsed, got nil")
	}
	if msg.Phone != "628123456789" {
		t.Errorf("expected phone 628123456789, got %s", msg.Phone)
	}
	if msg.Text != "Halo apa kabar?" {
		t.Errorf("expected text 'Halo apa kabar?', got %s", msg.Text)
	}
	if msg.Name != "Budi" {
		t.Errorf("expected name Budi, got %s", msg.Name)
	}
	if msg.MediaType != "" {
		t.Errorf("expected empty MediaType, got %s", msg.MediaType)
	}
}

func TestExtractIncomingExtendedText(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"remoteJid": "628987654321@s.whatsapp.net",
				"fromMe":    false,
			},
			"pushName": "Siti",
			"message": map[string]any{
				"extendedTextMessage": map[string]any{
					"text": "Pesan panjang dari link preview",
				},
			},
		},
	}

	msg := ExtractIncoming(payload)
	if msg == nil {
		t.Fatalf("expected message parsed, got nil")
	}
	if msg.Phone != "628987654321" {
		t.Errorf("expected phone 628987654321, got %s", msg.Phone)
	}
	if msg.Text != "Pesan panjang dari link preview" {
		t.Errorf("expected text 'Pesan panjang dari link preview', got %s", msg.Text)
	}
	if msg.Name != "Siti" {
		t.Errorf("expected name Siti, got %s", msg.Name)
	}
}

func TestExtractIncomingImage(t *testing.T) {
	payload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"remoteJid": "628111222333@s.whatsapp.net",
				"fromMe":    false,
			},
			"pushName": "Ani",
			"message": map[string]any{
				"imageMessage": map[string]any{
					"caption":  "Lihat jerawat ini kak",
					"mimetype": "image/png",
				},
			},
		},
	}

	msg := ExtractIncoming(payload)
	if msg == nil {
		t.Fatalf("expected message parsed, got nil")
	}
	if msg.Phone != "628111222333" {
		t.Errorf("expected phone 628111222333, got %s", msg.Phone)
	}
	if msg.Text != "Lihat jerawat ini kak" {
		t.Errorf("expected caption 'Lihat jerawat ini kak', got %s", msg.Text)
	}
	if msg.MediaType != "image" {
		t.Errorf("expected MediaType 'image', got %s", msg.MediaType)
	}
	if msg.Mime != "image/png" {
		t.Errorf("expected Mime 'image/png', got %s", msg.Mime)
	}
}

func TestExtractIncomingIgnoreSelfOrGroup(t *testing.T) {
	// From me
	selfPayload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"remoteJid": "628123456789@s.whatsapp.net",
				"fromMe":    true,
			},
		},
	}
	if ExtractIncoming(selfPayload) != nil {
		t.Errorf("expected nil for fromMe=true")
	}

	// Group message (@g.us)
	groupPayload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"remoteJid": "123456789@g.us",
				"fromMe":    false,
			},
		},
	}
	if ExtractIncoming(groupPayload) != nil {
		t.Errorf("expected nil for group message")
	}

	// Non-WhatsApp remoteJid format
	broadcastPayload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"remoteJid": "status@broadcast",
				"fromMe":    false,
			},
		},
	}
	if ExtractIncoming(broadcastPayload) != nil {
		t.Errorf("expected nil for broadcast message")
	}

	// Empty data
	if ExtractIncoming(map[string]any{}) != nil {
		t.Errorf("expected nil for empty payload")
	}
}

func TestSplitBubbles(t *testing.T) {
	text := "Halo kak! [NEXT] Ada yang bisa dibantu?\n\nSilakan cek menu kami.[WAIT][HANDOVER]"
	bubbles := SplitBubbles(text)
	if len(bubbles) != 3 {
		t.Fatalf("expected 3 bubbles, got %d: %v", len(bubbles), bubbles)
	}
	if bubbles[0] != "Halo kak!" {
		t.Errorf("bubble 0 expected 'Halo kak!', got %q", bubbles[0])
	}
	if bubbles[1] != "Ada yang bisa dibantu?" {
		t.Errorf("bubble 1 expected 'Ada yang bisa dibantu?', got %q", bubbles[1])
	}
	if bubbles[2] != "Silakan cek menu kami." {
		t.Errorf("bubble 2 expected 'Silakan cek menu kami.', got %q", bubbles[2])
	}
}

func TestWhatsAppClient_SendText(t *testing.T) {
	var receivedNumber, receivedText, receivedKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("apikey")
		if !strings.HasSuffix(r.URL.Path, "/message/sendText/glowria") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receivedNumber = body["number"]
		receivedText = body["text"]
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		EvolutionAPIURL:   ts.URL,
		EvolutionAPIKey:   "test-key",
		EvolutionInstance: "glowria",
	}

	client := NewWhatsAppClient(cfg)
	err := client.SendText("628123456789", "Halo dari bot")
	if err != nil {
		t.Fatalf("SendText failed: %v", err)
	}

	if receivedKey != "test-key" {
		t.Errorf("expected apikey 'test-key', got %q", receivedKey)
	}
	if receivedNumber != "628123456789" {
		t.Errorf("expected number '628123456789', got %q", receivedNumber)
	}
	if receivedText != "Halo dari bot" {
		t.Errorf("expected text 'Halo dari bot', got %q", receivedText)
	}
}

func TestWhatsAppClient_SendTyping(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat/sendPresence/glowria") {
			called = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &config.Config{
		EvolutionAPIURL:   ts.URL,
		EvolutionAPIKey:   "test-key",
		EvolutionInstance: "glowria",
	}

	client := NewWhatsAppClient(cfg)
	client.SendTyping("628123456789", 500*time.Millisecond)

	if !called {
		t.Errorf("expected sendPresence to be called")
	}
}

func TestWhatsAppClient_FetchMediaBase64(t *testing.T) {
	rawBytes := []byte("fake-image-bytes-data")
	encoded := base64.StdEncoding.EncodeToString(rawBytes)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/chat/getBase64FromMediaMessage/glowria") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"base64":   encoded,
				"mimetype": "image/jpeg",
			})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &config.Config{
		EvolutionAPIURL:   ts.URL,
		EvolutionAPIKey:   "test-key",
		EvolutionInstance: "glowria",
	}

	client := NewWhatsAppClient(cfg)
	data, mime, err := client.FetchMediaBase64(map[string]any{"id": "msg-1"})
	if err != nil {
		t.Fatalf("FetchMediaBase64 failed: %v", err)
	}
	if string(data) != string(rawBytes) {
		t.Errorf("expected data %q, got %q", string(rawBytes), string(data))
	}
	if mime != "image/jpeg" {
		t.Errorf("expected mime 'image/jpeg', got %q", mime)
	}
}

func TestWhatsAppClient_SendReply(t *testing.T) {
	var sentTexts []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/message/sendText/glowria") {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			sentTexts = append(sentTexts, body["text"])
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/chat/sendPresence/glowria") {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &config.Config{
		EvolutionAPIURL:   ts.URL,
		EvolutionAPIKey:   "test-key",
		EvolutionInstance: "glowria",
	}

	client := NewWhatsAppClient(cfg)
	client.SetSleepFn(func(time.Duration) {}) // no sleep in test

	reply := "Bubble 1 [NEXT] Bubble 2\n\nBubble 3 [WAIT] [HANDOVER]"
	client.SendReply("628123456789", reply)

	if len(sentTexts) != 3 {
		t.Fatalf("expected 3 sent texts, got %d: %v", len(sentTexts), sentTexts)
	}
	if sentTexts[0] != "Bubble 1" {
		t.Errorf("expected sentTexts[0] 'Bubble 1', got %q", sentTexts[0])
	}
	if sentTexts[1] != "Bubble 2" {
		t.Errorf("expected sentTexts[1] 'Bubble 2', got %q", sentTexts[1])
	}
	if sentTexts[2] != "Bubble 3" {
		t.Errorf("expected sentTexts[2] 'Bubble 3', got %q", sentTexts[2])
	}
}

