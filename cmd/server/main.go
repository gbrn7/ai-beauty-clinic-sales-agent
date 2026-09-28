package main

import (
	"fmt"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/raygbrn/ai-sales-agent-go/internal/agent"
	"github.com/raygbrn/ai-sales-agent-go/internal/buffer"
	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/db"
	"github.com/raygbrn/ai-sales-agent-go/internal/handler"
	"github.com/raygbrn/ai-sales-agent-go/internal/memory"
	"github.com/raygbrn/ai-sales-agent-go/internal/storage"
	"github.com/raygbrn/ai-sales-agent-go/internal/tools"
	"github.com/raygbrn/ai-sales-agent-go/internal/whatsapp"
)

func main() {
	cfg := config.Load()

	database, err := db.InitDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := db.SeedDB(database); err != nil {
		log.Fatalf("Failed to seed database: %v", err)
	}

	toolService := tools.NewToolService(database)
	mem := memory.NewMemory(database, cfg.MemoryWindow, cfg.SessionTTLHours)
	storageService := storage.NewStorageService(cfg, "media")
	waClient := whatsapp.NewWhatsAppClient(cfg)

	var ag *agent.Agent
	if cfg.GeminiAPIKey != "" {
		ag, err = agent.NewAgent(cfg, toolService, "prompts/system.md")
		if err != nil {
			log.Printf("[warning] Failed to init agent: %v. Webhook AI will not respond.", err)
		}
	} else {
		log.Println("[warning] GEMINI_API_KEY is empty. Provide API key in .env to enable AI agent.")
	}

	var debouncer *buffer.DebounceBuffer
	if ag != nil {
		debouncer = buffer.NewDebounceBuffer(cfg, mem, ag, waClient, storageService)
	}

	r := gin.Default()
	handler.RegisterRoutes(r, &handler.Dependencies{
		Config:   cfg,
		DB:       database,
		Buffer:   debouncer,
		Storage:  storageService,
		WhatsApp: waClient,
		Memory:   mem,
	})

	log.Printf("Glowria AI Sales Agent Server listening on port %s (Buffer debounce: %ds)", cfg.Port, cfg.BufferSeconds)
	if err := r.Run(fmt.Sprintf(":%s", cfg.Port)); err != nil {
		log.Fatalf("Server stopped: %v", err)
	}
}
