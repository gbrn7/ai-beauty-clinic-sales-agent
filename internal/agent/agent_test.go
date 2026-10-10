package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/genai"

	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/db"
	"github.com/raygbrn/ai-sales-agent-go/internal/tools"
)

func setupTestToolService(t *testing.T) *tools.ToolService {
	database, err := db.InitDB("sqlite://file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("db init failed: %v", err)
	}
	if err := db.SeedDB(database); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	return tools.NewToolService(database)
}

func TestNewAgent_FileNotFound(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "test-api-key",
		GeminiModel:  "gemini-2.5-flash",
		AIName:       "Gita",
		CompanyName:  "Glowria",
	}
	ts := tools.NewToolService(nil)
	_, err := NewAgent(cfg, ts, "non-existent-path.md")
	if err == nil {
		t.Fatal("expected error when prompt file does not exist, got nil")
	}
}

func TestNewAgent_Success(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey: "test-api-key",
		GeminiModel:  "gemini-2.5-flash",
		AIName:       "Gita",
		CompanyName:  "Glowria Aesthetic Clinic",
	}
	ts := tools.NewToolService(nil)
	promptPath := "../../prompts/system.md"

	ag, err := NewAgent(cfg, ts, promptPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ag == nil {
		t.Fatal("expected agent to be non-nil")
	}
	if ag.model != "gemini-2.5-flash" {
		t.Errorf("expected model gemini-2.5-flash, got %s", ag.model)
	}
	if !strings.Contains(ag.systemPrompt, "Gita") {
		t.Errorf("expected systemPrompt to contain Gita")
	}
	if ag.genConfig == nil {
		t.Fatal("expected genConfig to be non-nil")
	}
	if ag.genConfig.Temperature == nil || *ag.genConfig.Temperature != float32(0.7) {
		t.Errorf("expected Temperature 0.7, got %v", ag.genConfig.Temperature)
	}
	if len(ag.genConfig.Tools) == 0 || len(ag.genConfig.Tools[0].FunctionDeclarations) == 0 {
		t.Error("expected function declarations to be set in Tools")
	}
}

func TestNewAgent_ThinkingConfig_Gemini3(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey:   "test-api-key",
		GeminiModel:    "gemini-3.1-flash-lite",
		GeminiThinking: "low",
		AIName:         "Gita",
		CompanyName:    "Glowria Aesthetic Clinic",
	}
	ts := tools.NewToolService(nil)
	promptPath := "../../prompts/system.md"

	ag, err := NewAgent(cfg, ts, promptPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ag.genConfig.ThinkingConfig == nil {
		t.Fatal("expected ThinkingConfig to be set")
	}
	if ag.genConfig.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelLow {
		t.Errorf("expected ThinkingLevelLow, got %v", ag.genConfig.ThinkingConfig.ThinkingLevel)
	}
}

func TestNewAgent_ThinkingConfig_Disabled(t *testing.T) {
	cfg := &config.Config{
		GeminiAPIKey:   "test-api-key",
		GeminiModel:    "gemini-2.5-flash",
		GeminiThinking: "off",
		AIName:         "Gita",
		CompanyName:    "Glowria Aesthetic Clinic",
	}
	ts := tools.NewToolService(nil)
	promptPath := "../../prompts/system.md"

	ag, err := NewAgent(cfg, ts, promptPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ag.genConfig.ThinkingConfig == nil {
		t.Fatal("expected ThinkingConfig to be set")
	}
	if ag.genConfig.ThinkingConfig.ThinkingBudget == nil || *ag.genConfig.ThinkingConfig.ThinkingBudget != 0 {
		t.Errorf("expected ThinkingBudget 0, got %v", ag.genConfig.ThinkingConfig.ThinkingBudget)
	}
}

func TestRunAgent_SingleTurnText(t *testing.T) {
	var requestedPath string
	var reqBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&reqBody)

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Halo kak! Ada yang bisa Gita bantu?"},
						},
					},
					"finishReason": "STOP",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  "test-key",
		Backend: genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: server.URL,
		},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ts := tools.NewToolService(nil)
	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        ts,
		genConfig: &genai.GenerateContentConfig{
			Temperature: genai.Ptr(float32(0.7)),
		},
	}

	reply, contents, err := ag.RunAgent(ctx, nil, "Halo mau tanya", "081234567890", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedReply := "Halo kak! Ada yang bisa Gita bantu?"
	if reply != expectedReply {
		t.Errorf("expected reply %q, got %q", expectedReply, reply)
	}

	// Contents should have 2 entries: 1 user, 1 model
	if len(contents) != 2 {
		t.Fatalf("expected contents length 2, got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("expected first content role 'user', got %s", contents[0].Role)
	}
	if contents[1].Role != "model" {
		t.Errorf("expected second content role 'model', got %s", contents[1].Role)
	}

	// Verify path requested
	if !strings.Contains(requestedPath, "gemini-2.5-flash") {
		t.Errorf("expected path to contain model name, got %s", requestedPath)
	}

	// Verify context injected in user part
	userPartText := contents[0].Parts[0].Text
	if !strings.Contains(userPartText, "[Konteks: Hari ini") {
		t.Errorf("expected date context in user part, got %s", userPartText)
	}
	if !strings.Contains(userPartText, "Nomor WhatsApp customer: 081234567890") {
		t.Errorf("expected phone number in user part, got %s", userPartText)
	}
	if !strings.Contains(userPartText, "Halo mau tanya") {
		t.Errorf("expected user message in user part, got %s", userPartText)
	}
}

func TestRunAgent_EmptyMessageWithImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Gambarnya sudah kulihat ya kak!"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        tools.NewToolService(nil),
		genConfig:    &genai.GenerateContentConfig{},
	}

	images := []ImageData{
		{
			Bytes:    []byte("fake-image-bytes"),
			MIMEType: "image/jpeg",
		},
	}

	reply, contents, err := ag.RunAgent(ctx, nil, "", "081234567890", images)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "Gambarnya sudah kulihat ya kak!" {
		t.Errorf("unexpected reply: %s", reply)
	}

	// Verify default message placeholder
	userParts := contents[0].Parts
	if len(userParts) != 2 {
		t.Fatalf("expected 2 parts (text + image), got %d", len(userParts))
	}
	if !strings.Contains(userParts[0].Text, "(customer mengirim gambar tanpa teks)") {
		t.Errorf("expected placeholder for empty message, got %s", userParts[0].Text)
	}
	if userParts[1].InlineData == nil || userParts[1].InlineData.MIMEType != "image/jpeg" {
		t.Errorf("expected inline image data with image/jpeg")
	}
}

func TestRunAgent_HistoryPreserved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Bisa kak!"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        tools.NewToolService(nil),
		genConfig:    &genai.GenerateContentConfig{},
	}

	history := []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: "Previous question"}}},
		{Role: "model", Parts: []*genai.Part{{Text: "Previous answer"}}},
	}

	_, contents, err := ag.RunAgent(ctx, history, "Next question", "081234567890", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Total contents: 2 (history) + 1 (current user) + 1 (model reply) = 4
	if len(contents) != 4 {
		t.Fatalf("expected 4 contents, got %d", len(contents))
	}
	if contents[0].Parts[0].Text != "Previous question" {
		t.Errorf("expected first content to be history[0], got %s", contents[0].Parts[0].Text)
	}
	if contents[1].Parts[0].Text != "Previous answer" {
		t.Errorf("expected second content to be history[1], got %s", contents[1].Parts[0].Text)
	}
}

func TestRunAgent_ToolExecutionLoop(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)

		var resp map[string]any
		if count == 1 {
			// First call: model requests search_treatments
			resp = map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"role": "model",
							"parts": []map[string]any{
								{
									"functionCall": map[string]any{
										"name": "search_treatments",
										"args": map[string]any{
											"query": "acne",
										},
									},
								},
							},
						},
					},
				},
			}
		} else {
			// Second call: model responds with final text
			resp = map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"role": "model",
							"parts": []map[string]any{
								{"text": "Kami punya Acne Treatment seharga 300rb kak!"},
							},
						},
					},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ts := setupTestToolService(t)
	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        ts,
		genConfig:    &genai.GenerateContentConfig{},
	}

	reply, contents, err := ag.RunAgent(ctx, nil, "Ada treatment jerawat?", "081234567890", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "Kami punya Acne Treatment seharga 300rb kak!" {
		t.Errorf("unexpected reply: %s", reply)
	}

	if atomic.LoadInt32(&callCount) != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}

	// Contents sequence:
	// 0: user (initial prompt)
	// 1: model (function call)
	// 2: user (function response)
	// 3: model (final text)
	if len(contents) != 4 {
		t.Fatalf("expected 4 contents in tool execution loop, got %d", len(contents))
	}
	if contents[1].Parts[0].FunctionCall == nil {
		t.Errorf("expected content[1] to have FunctionCall")
	}
	if contents[2].Parts[0].FunctionResponse == nil {
		t.Errorf("expected content[2] to have FunctionResponse")
	}
}

func TestRunAgent_PreservesThoughtSignature(t *testing.T) {
	var callCount int32
	var secondCallBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)

		if count == 1 {
			// First call returns function call with thought signature
			resp := map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"role": "model",
							"parts": []map[string]any{
								{
									"thought": true,
									"text":    "Mencari treatment acne...",
								},
								{
									"functionCall": map[string]any{
										"name": "search_treatments",
										"args": map[string]any{
											"query": "acne",
										},
									},
									"thoughtSignature": "dGVzdC1zaWduYXR1cmU=",
								},
							},
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Capture second call body
		_ = json.NewDecoder(r.Body).Decode(&secondCallBody)
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Hasil treatment acne ditemukan!"},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ts := setupTestToolService(t)
	ag := &Agent{
		client:       mockClient,
		model:        "gemini-3.1-flash-lite",
		systemPrompt: "System instruction test",
		tools:        ts,
		genConfig:    &genai.GenerateContentConfig{},
	}

	reply, contents, err := ag.RunAgent(ctx, nil, "Ada treatment jerawat?", "081234567890", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != "Hasil treatment acne ditemukan!" {
		t.Errorf("unexpected reply: %s", reply)
	}

	// Verify thoughtSignature is preserved in contents[1].Parts[1]
	if len(contents) < 2 {
		t.Fatalf("expected at least 2 contents, got %d", len(contents))
	}
	modelContent := contents[1]
	if len(modelContent.Parts) < 2 {
		t.Fatalf("expected at least 2 parts in modelContent, got %d", len(modelContent.Parts))
	}
	fcPart := modelContent.Parts[1]
	if len(fcPart.ThoughtSignature) == 0 {
		t.Errorf("expected ThoughtSignature to be preserved on modelContent.Parts[1], got empty")
	}

	// Also verify that the second HTTP request received thoughtSignature in the payload
	reqBytes, _ := json.Marshal(secondCallBody)
	if !strings.Contains(string(reqBytes), "thoughtSignature") {
		t.Errorf("expected second API request to contain thoughtSignature, got: %s", string(reqBytes))
	}
}

func TestRunAgent_MaxToolIterationsExceeded(t *testing.T) {
	var callCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)

		// Always return a function call, exceeding MaxToolIterations
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{
								"functionCall": map[string]any{
									"name": "search_treatments",
									"args": map[string]any{
										"query": "acne",
									},
								},
							},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ts := setupTestToolService(t)
	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        ts,
		genConfig:    &genai.GenerateContentConfig{},
	}

	reply, _, err := ag.RunAgent(ctx, nil, "Tanya acne", "081234567890", nil)
	if err != nil {
		t.Fatalf("expected nil error on iteration exhaustion, got: %v", err)
	}

	if reply != FallbackMessage {
		t.Errorf("expected FallbackMessage when iterations exceeded, got %s", reply)
	}

	if atomic.LoadInt32(&callCount) != int32(MaxToolIterations) {
		t.Errorf("expected %d iterations, got %d", MaxToolIterations, callCount)
	}
}

func TestRunAgent_APIErrorFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": {"message": "internal error"}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        tools.NewToolService(nil),
		genConfig:    &genai.GenerateContentConfig{},
	}

	reply, _, err := ag.RunAgent(ctx, nil, "Halo", "081234567890", nil)
	if err == nil {
		t.Fatal("expected error on API failure, got nil")
	}

	if reply != FallbackMessage {
		t.Errorf("expected FallbackMessage on error, got %s", reply)
	}
}

func TestRunAgent_EmptyCandidatesFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        tools.NewToolService(nil),
		genConfig:    &genai.GenerateContentConfig{},
	}

	reply, _, err := ag.RunAgent(ctx, nil, "Halo", "081234567890", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != FallbackMessage {
		t.Errorf("expected FallbackMessage on empty candidates, got %s", reply)
	}
}

func TestRunAgent_EmptyTextCandidateFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role":  "model",
						"parts": []map[string]any{},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ctx := context.Background()
	mockClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:      "test-key",
		Backend:     genai.BackendGeminiAPI,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	ag := &Agent{
		client:       mockClient,
		model:        "gemini-2.5-flash",
		systemPrompt: "System instruction test",
		tools:        tools.NewToolService(nil),
		genConfig:    &genai.GenerateContentConfig{},
	}

	reply, _, err := ag.RunAgent(ctx, nil, "Halo", "081234567890", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reply != FallbackMessage {
		t.Errorf("expected FallbackMessage when text is empty, got %s", reply)
	}
}
