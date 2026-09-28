package whatsapp

import (
	"regexp"
	"strings"
)

type IncomingMessage struct {
	Phone     string
	Name      string
	Text      string
	MediaType string // "image" or empty
	Mime      string
	RawData   map[string]any
}

func ExtractIncoming(payload map[string]any) *IncomingMessage {
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		return nil
	}
	key, _ := data["key"].(map[string]any)
	if key == nil {
		return nil
	}
	if fromMe, _ := key["fromMe"].(bool); fromMe {
		return nil
	}

	remoteJid, _ := key["remoteJid"].(string)
	if strings.Contains(remoteJid, "@g.us") || !strings.HasSuffix(remoteJid, "@s.whatsapp.net") {
		return nil
	}

	phone := strings.Split(remoteJid, "@")[0]
	name, _ := data["pushName"].(string)
	message, _ := data["message"].(map[string]any)
	if message == nil {
		return nil
	}

	// Check image message
	if imgMsg, ok := message["imageMessage"].(map[string]any); ok {
		caption, _ := imgMsg["caption"].(string)
		mime, _ := imgMsg["mimetype"].(string)
		if mime == "" {
			mime = "image/jpeg"
		}
		return &IncomingMessage{
			Phone:     phone,
			Name:      name,
			Text:      caption,
			MediaType: "image",
			Mime:      mime,
			RawData:   data,
		}
	}

	// Check regular text message
	text, _ := message["conversation"].(string)
	if text == "" {
		if ext, ok := message["extendedTextMessage"].(map[string]any); ok {
			text, _ = ext["text"].(string)
		}
	}

	if text == "" {
		return nil
	}

	return &IncomingMessage{
		Phone:   phone,
		Name:    name,
		Text:    text,
		RawData: data,
	}
}

var splitRegex = regexp.MustCompile(`\[NEXT\]|\n\s*\n`)

func SplitBubbles(reply string) []string {
	clean := strings.ReplaceAll(reply, "[WAIT]", "")
	clean = strings.ReplaceAll(clean, "[HANDOVER]", "")

	rawBubbles := splitRegex.Split(clean, -1)
	var bubbles []string
	for _, b := range rawBubbles {
		t := strings.TrimSpace(b)
		if t != "" {
			bubbles = append(bubbles, t)
		}
	}
	return bubbles
}
