package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/genai"

	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/tools"
)

const (
	MaxToolIterations = 5
	FallbackMessage   = "Mohon maaf kak, whatsapp kami sedang ada sedikit kendala🙏[NEXT]" +
		"Admin kami akan segera membantu ya kak 😊[HANDOVER]"
)

var (
	HariNama  = []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}
	BulanNama = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli",
		"Agustus", "September", "Oktober", "November", "Desember"}
)

type ImageData struct {
	Bytes    []byte
	MIMEType string
}

type Agent struct {
	client       *genai.Client
	model        string
	systemPrompt string
	tools        *tools.ToolService
	genConfig    *genai.GenerateContentConfig
}

func NewAgent(cfg *config.Config, ts *tools.ToolService, promptPath string) (*Agent, error) {
	sysPrompt, err := RenderSystemPrompt(promptPath, cfg.AIName, cfg.CompanyName)
	if err != nil {
		return nil, fmt.Errorf("failed to render system prompt: %w", err)
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize genai client: %w", err)
	}

	genConfig := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: sysPrompt}},
		},
		Tools: []*genai.Tool{
			{FunctionDeclarations: ts.GetFunctionDeclarations()},
		},
		Temperature: genai.Ptr(float64(0.7)),
	}

	return &Agent{
		client:       client,
		model:        cfg.GeminiModel,
		systemPrompt: sysPrompt,
		tools:        ts,
		genConfig:    genConfig,
	}, nil
}

func (a *Agent) RunAgent(ctx context.Context, history []*genai.Content, userMessage, phone string, images []ImageData) (string, []*genai.Content, error) {
	now := time.Now()
	weekday := int(now.Weekday()) - 1
	if weekday < 0 {
		weekday = 6
	}

	dateCtx := fmt.Sprintf("Hari ini %s, %d %s %d (%s), pukul %02d:%02d",
		HariNama[weekday], now.Day(), BulanNama[now.Month()-1], now.Year(),
		now.Format("2006-01-02"), now.Hour(), now.Minute())

	if userMessage == "" {
		userMessage = "(customer mengirim gambar tanpa teks)"
	}

	parts := []*genai.Part{
		{Text: fmt.Sprintf("[Konteks: %s. Nomor WhatsApp customer: %s]\n%s", dateCtx, phone, userMessage)},
	}
	for _, img := range images {
		parts = append(parts, &genai.Part{
			InlineData: &genai.Blob{
				MIMEType: img.MIMEType,
				Data:     img.Bytes,
			},
		})
	}

	contents := append([]*genai.Content{}, history...)
	contents = append(contents, &genai.Content{Role: "user", Parts: parts})

	for iter := 0; iter < MaxToolIterations; iter++ {
		resp, err := a.client.Models.GenerateContent(ctx, a.model, contents, a.genConfig)
		if err != nil {
			log.Printf("[error] Gemini API error: %v", err)
			return FallbackMessage, contents, err
		}

		if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
			return FallbackMessage, contents, nil
		}

		cand := resp.Candidates[0]
		contents = append(contents, cand.Content)

		functionCalls := resp.FunctionCalls()
		if len(functionCalls) == 0 {
			replyText, _ := resp.Text()
			if replyText == "" {
				replyText = FallbackMessage
			}
			return replyText, contents, nil
		}

		var resultParts []*genai.Part
		for _, fc := range functionCalls {
			log.Printf("  [tool] %s(%v)", fc.Name, fc.Args)
			result := a.tools.ExecuteTool(fc.Name, fc.Args)
			resultParts = append(resultParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: map[string]any{"result": result},
				},
			})
		}
		contents = append(contents, &genai.Content{Role: "user", Parts: resultParts})
	}

	return FallbackMessage, contents, nil
}
