package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/genai"

	"github.com/raygbrn/ai-sales-agent-go/internal/agent"
	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/db"
	"github.com/raygbrn/ai-sales-agent-go/internal/tools"
	"github.com/raygbrn/ai-sales-agent-go/internal/whatsapp"
)

const DummyPhone = "6281234567890"

func main() {
	cfg := config.Load()
	if cfg.GeminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY belum diisi. Salin .env.example ke .env dan isi API key dari https://aistudio.google.com")
	}

	database, err := db.InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := db.SeedDB(database); err != nil {
		log.Fatalf("Failed to seed database: %v", err)
	}

	toolService := tools.NewToolService(database)
	ag, err := agent.NewAgent(cfg, toolService, "prompts/system.md")
	if err != nil {
		log.Fatalf("Failed to initialize agent: %v", err)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  Glowria Aesthetic Clinic — AI Sales Agent (model: %s)\n", cfg.GeminiModel)
	fmt.Println("  Ketik pesan seperti customer WhatsApp. 'exit' untuk keluar.")
	fmt.Println(strings.Repeat("=", 60))

	reader := bufio.NewReader(os.Stdin)
	var history []*genai.Content
	ctx := context.Background()

	for {
		fmt.Print("\nCustomer > ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		input = strings.TrimSpace(input)
		if input == "" || strings.EqualFold(input, "exit") || strings.EqualFold(input, "quit") {
			break
		}

		reply, newHistory, err := ag.RunAgent(ctx, history, input, DummyPhone, nil)
		if err != nil {
			fmt.Printf("\n[Error]: %v\n", err)
			continue
		}
		history = newHistory

		bubbles := whatsapp.SplitBubbles(reply)
		for _, b := range bubbles {
			if strings.Contains(reply, "[HANDOVER]") {
				fmt.Printf("\nGita    > %s\n  ⚠ [HANDOVER terdeteksi]\n", b)
			} else {
				fmt.Printf("\nGita    > %s\n", b)
			}
		}
	}

	fmt.Println("\nSampai jumpa!")
}
