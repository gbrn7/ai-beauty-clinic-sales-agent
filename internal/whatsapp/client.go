package whatsapp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/raygbrn/ai-sales-agent-go/internal/config"
)

const (
	TypingSecondsPerChar = 0.03
	TypingMinSeconds     = 1.0
	TypingMaxSeconds     = 5.0
)

type WhatsAppClient struct {
	apiURL   string
	apiKey   string
	instance string
	http     *http.Client
	sleepFn  func(time.Duration)
}

func NewWhatsAppClient(cfg *config.Config) *WhatsAppClient {
	return &WhatsAppClient{
		apiURL:   strings.TrimRight(cfg.EvolutionAPIURL, "/"),
		apiKey:   cfg.EvolutionAPIKey,
		instance: cfg.EvolutionInstance,
		http:     &http.Client{Timeout: 30 * time.Second},
		sleepFn:  time.Sleep,
	}
}

// NewClient is an alias for NewWhatsAppClient.
func NewClient(cfg *config.Config) *WhatsAppClient {
	return NewWhatsAppClient(cfg)
}

// SetSleepFn overrides the sleep duration function (useful for fast unit tests).
func (c *WhatsAppClient) SetSleepFn(fn func(time.Duration)) {
	c.sleepFn = fn
}

func calculateTypingDuration(text string) time.Duration {
	secs := float64(len(text)) * TypingSecondsPerChar
	if secs < TypingMinSeconds {
		secs = TypingMinSeconds
	}
	if secs > TypingMaxSeconds {
		secs = TypingMaxSeconds
	}
	return time.Duration(secs * float64(time.Second))
}

func (c *WhatsAppClient) SendTyping(phone string, duration time.Duration) {
	url := fmt.Sprintf("%s/chat/sendPresence/%s", c.apiURL, c.instance)
	payload := map[string]any{
		"number":   phone,
		"presence": "composing",
		"delay":    int(duration.Milliseconds()),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return
	}
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func (c *WhatsAppClient) SendText(phone, text string) error {
	url := fmt.Sprintf("%s/message/sendText/%s", c.apiURL, c.instance)
	payload := map[string]any{
		"number": phone,
		"text":   text,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("evolution api returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *WhatsAppClient) FetchMediaBase64(message map[string]any) ([]byte, string, error) {
	url := fmt.Sprintf("%s/chat/getBase64FromMediaMessage/%s", c.apiURL, c.instance)
	payload := map[string]any{
		"message":      message,
		"convertToMp4": false,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("evolution api returned status %d", resp.StatusCode)
	}

	var body struct {
		Base64   string `json:"base64"`
		MimeType string `json:"mimetype"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(body.Base64)
	if err != nil {
		return nil, "", err
	}
	mime := body.MimeType
	if mime == "" {
		mime = "image/jpeg"
	}
	return decoded, mime, nil
}

func (c *WhatsAppClient) SendReply(phone, reply string) {
	bubbles := SplitBubbles(reply)
	for _, bubble := range bubbles {
		dur := calculateTypingDuration(bubble)
		c.SendTyping(phone, dur)
		if c.sleepFn != nil {
			c.sleepFn(dur)
		} else {
			time.Sleep(dur)
		}
		if err := c.SendText(phone, bubble); err != nil {
			log.Printf("[whatsapp] failed to send text to %s: %v", phone, err)
		}
	}
}

// SendReply package function as specified in interfaces.
func SendReply(client *WhatsAppClient, phone, reply string) error {
	client.SendReply(phone, reply)
	return nil
}
