package agent

import (
	"os"
	"strings"

	"github.com/raygbrn/ai-sales-agent-go/prompts"
)

func RenderSystemPrompt(path, aiName, companyName string) (string, error) {
	var raw string
	if path != "" {
		bytes, err := os.ReadFile(path)
		if err == nil {
			raw = string(bytes)
		} else if path == "prompts/system.md" || path == "system.md" {
			raw = prompts.SystemPrompt
		} else {
			return "", err
		}
	} else {
		raw = prompts.SystemPrompt
	}

	replacements := map[string]string{
		"{{aiName}}":      aiName,
		"{{companyName}}": companyName,
		"{{currentDateContext}}": "Tanggal dan jam SAAT INI selalu diberikan di awal setiap pesan customer " +
			"(dalam tanda [Konteks: ...]). Jadikan itu satu-satunya acuan waktu untuk " +
			"menghitung 'hari ini', 'besok', 'lusa', dll. JANGAN PERNAH menawarkan atau " +
			"menyetujui booking untuk jam yang sudah lewat.",
		"{{bookingContext}}": "<booking_context>Gunakan tool check_available_schedule dan " +
			"book_treatment untuk status slot real-time. Jangan pernah berasumsi soal ketersediaan tanpa hasil tool.</booking_context>",
		"{{leadContext}}":         "Lihat info customer di awal percakapan.",
		"{{conversationSummary}}": "Lihat history percakapan di contents.",
		"{{knowledgeBase}}": "Gunakan tool search_treatments untuk data treatment dan harga. " +
			"Info klinik statis ada di <clinic_info>.",
		"{{additionalContext}}": "",
	}

	for k, v := range replacements {
		raw = strings.ReplaceAll(raw, k, v)
	}
	return raw, nil
}
