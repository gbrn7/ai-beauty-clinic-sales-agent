package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/raygbrn/ai-sales-agent-go/internal/buffer"
	"github.com/raygbrn/ai-sales-agent-go/internal/config"
	"github.com/raygbrn/ai-sales-agent-go/internal/db"
	"github.com/raygbrn/ai-sales-agent-go/internal/handler"
	"github.com/raygbrn/ai-sales-agent-go/internal/memory"
	"github.com/raygbrn/ai-sales-agent-go/internal/model"
	"github.com/raygbrn/ai-sales-agent-go/internal/storage"
	"github.com/raygbrn/ai-sales-agent-go/internal/whatsapp"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *handler.Dependencies, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	tmpDir := t.TempDir()
	database, err := db.InitDB("sqlite://" + filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}

	cfg := &config.Config{
		BufferSeconds:   8,
		DashboardAPIKey: "secret123",
		CompanyName:     "Glowria Aesthetic Clinic",
		AIName:          "Gita",
	}

	mem := memory.NewMemory(database, 20, 24)
	buf := buffer.NewDebounceBuffer(cfg, mem, nil, nil, nil)
	mediaDir := t.TempDir()
	st := storage.NewStorageService(cfg, mediaDir)

	deps := &handler.Dependencies{
		Config:   cfg,
		DB:       database,
		Buffer:   buf,
		Storage:  st,
		Memory:   mem,
		WhatsApp: nil,
	}

	handler.RegisterRoutes(r, deps)
	return r, deps, database
}

func TestHealthEndpoint(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	if int(resp["buffer_seconds"].(float64)) != 8 {
		t.Errorf("expected buffer_seconds 8, got %v", resp["buffer_seconds"])
	}
}

func TestRootRedirect(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302 Found, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("expected redirect to /dashboard, got %q", loc)
	}
}

func TestDashboardAuthMiddleware(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	// Without key -> 401
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/dashboard/api/config", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without key, got %d", w1.Code)
	}

	// With wrong key -> 401
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/dashboard/api/config", nil)
	req2.Header.Set("X-API-Key", "wrong-key")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized with wrong key, got %d", w2.Code)
	}

	// With correct key -> 200
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/dashboard/api/config", nil)
	req3.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected 200 OK with valid X-API-Key, got %d", w3.Code)
	}
}

func TestDashboardHTML(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/dashboard", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(w.Body.String(), "AI Sales Dashboard") {
		t.Errorf("expected dashboard HTML content, got %s", w.Body.String()[:100])
	}
}

func TestDashboardHTML_DiskOverride(t *testing.T) {
	tmpDir := t.TempDir()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to tmpDir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.MkdirAll("templates", 0755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	customHTML := "<html><body>Custom Dashboard Override</body></html>"
	if err := os.WriteFile(filepath.Join("templates", "dashboard.html"), []byte(customHTML), 0644); err != nil {
		t.Fatalf("failed to write custom dashboard: %v", err)
	}

	r, _, _ := setupTestRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/dashboard", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Custom Dashboard Override") {
		t.Errorf("expected disk override HTML, got %s", w.Body.String())
	}
}

func TestWebhookEndpoint(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	// Valid incoming message
	payload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"fromMe":    false,
				"remoteJid": "628123456789@s.whatsapp.net",
			},
			"pushName": "Budi",
			"message": map[string]any{
				"conversation": "Halo tanya promo",
			},
		},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "buffered" {
		t.Errorf("expected status 'buffered', got %v", resp["status"])
	}

	// Ignored message (fromMe = true)
	ignorePayload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"fromMe":    true,
				"remoteJid": "628123456789@s.whatsapp.net",
			},
			"message": map[string]any{
				"conversation": "Halo",
			},
		},
	}
	bodyIgnore, _ := json.Marshal(ignorePayload)
	wIgnored := httptest.NewRecorder()
	reqIgnored, _ := http.NewRequest("POST", "/webhook", bytes.NewReader(bodyIgnore))
	reqIgnored.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wIgnored, reqIgnored)

	var respIgnored map[string]any
	_ = json.Unmarshal(wIgnored.Body.Bytes(), &respIgnored)
	if respIgnored["status"] != "ignored" {
		t.Errorf("expected status 'ignored', got %v", respIgnored["status"])
	}

	// Malformed JSON -> 400
	wBad := httptest.NewRecorder()
	reqBad, _ := http.NewRequest("POST", "/webhook", bytes.NewReader([]byte("{invalid-json")))
	reqBad.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", wBad.Code)
	}
}

func TestMediaProxyEndpoint(t *testing.T) {
	r, deps, _ := setupTestRouter(t)

	// Save media first
	path, err := deps.Storage.SaveMedia("628111", []byte("fake-image-content"), "image/jpeg")
	if err != nil {
		t.Fatalf("failed to save media: %v", err)
	}
	fname := filepath.Base(path)

	// Request media
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/media/"+fname, nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("expected Cache-Control header, got %q", w.Header().Get("Cache-Control"))
	}
	if !bytes.Equal(w.Body.Bytes(), []byte("fake-image-content")) {
		t.Errorf("media content mismatch")
	}

	// Non-existent file -> 404
	w404 := httptest.NewRecorder()
	req404, _ := http.NewRequest("GET", "/media/nonexistent.jpg", nil)
	r.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", w404.Code)
	}
}

func TestDashboardConfigEndpoint(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/dashboard/api/config", nil)
	req.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	var data map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &data)
	if data["company"] != "Glowria Aesthetic Clinic" || data["ai_name"] != "Gita" {
		t.Errorf("unexpected config response: %v", data)
	}
}

func TestDashboardConversationsList(t *testing.T) {
	r, _, database := setupTestRouter(t)

	nameAlice := "Alice"
	database.Model(&model.Contact{}).Create(map[string]any{
		"phone":      "6281001",
		"name":       &nameAlice,
		"ai_enabled": false,
	})

	now := time.Now()
	database.Create(&model.ConversationMessage{
		Phone:     "6281001",
		Role:      "user",
		Text:      "Pesan lama",
		CreatedAt: now.Add(-10 * time.Minute),
	})
	database.Create(&model.ConversationMessage{
		Phone:     "6281001",
		Role:      "model",
		Text:      "Pesan terbaru",
		CreatedAt: now.Add(-5 * time.Minute),
	})

	// Add booking for 6281001
	treatment := model.Treatment{
		Name:        "Laser Rejuvenation",
		Category:    "Laser",
		Price:       500000,
		Description: "Laser treatment",
	}
	database.Create(&treatment)
	database.Create(&model.Booking{
		Code:         "BK001",
		CustomerName: "Alice",
		Phone:        "6281001",
		TreatmentID:  treatment.ID,
		BookingDate:  time.Now().Format("2006-01-02"),
		BookingTime:  "14:00",
		Status:       "confirmed",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/dashboard/api/conversations", nil)
	req.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var list []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(list))
	}

	item := list[0]
	if item["phone"] != "6281001" {
		t.Errorf("expected phone 6281001, got %v", item["phone"])
	}
	if item["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", item["name"])
	}
	if item["last_text"] != "Pesan terbaru" {
		t.Errorf("expected last_text 'Pesan terbaru', got %v", item["last_text"])
	}
	if item["last_role"] != "model" {
		t.Errorf("expected last_role 'model', got %v", item["last_role"])
	}
	if item["ai_enabled"] != false {
		t.Errorf("expected ai_enabled false, got %v", item["ai_enabled"])
	}
	if item["handover"] != true {
		t.Errorf("expected handover true, got %v", item["handover"])
	}
	if item["has_booking"] != true {
		t.Errorf("expected has_booking true, got %v", item["has_booking"])
	}
}

func TestDashboardConversationDetail(t *testing.T) {
	r, _, database := setupTestRouter(t)

	nameBob := "Bob"
	database.Create(&model.Contact{
		Phone:       "6282002",
		Name:        &nameBob,
		AiEnabled:   true,
		IsReturning: true,
	})

	database.Create(&model.ConversationMessage{
		Phone:     "6282002",
		Role:      "user",
		Text:      "Halo dok",
		CreatedAt: time.Now().Add(-2 * time.Minute),
	})

	treatment := model.Treatment{
		Name:        "Acne Peel",
		Category:    "Peeling",
		Price:       350000,
		Description: "Peel acne",
	}
	database.Create(&treatment)
	database.Create(&model.Booking{
		Code:         "BK002",
		CustomerName: "Bob",
		Phone:        "6282002",
		TreatmentID:  treatment.ID,
		BookingDate:  time.Now().Format("2006-01-02"),
		BookingTime:  "10:00",
		Status:       "pending",
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/dashboard/api/conversations/6282002", nil)
	req.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var detail map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if detail["phone"] != "6282002" {
		t.Errorf("expected phone 6282002, got %v", detail["phone"])
	}
	if detail["name"] != "Bob" {
		t.Errorf("expected name Bob, got %v", detail["name"])
	}
	if detail["ai_enabled"] != true {
		t.Errorf("expected ai_enabled true, got %v", detail["ai_enabled"])
	}
	if detail["is_returning"] != true {
		t.Errorf("expected is_returning true, got %v", detail["is_returning"])
	}

	messages := detail["messages"].([]any)
	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
	bookings := detail["bookings"].([]any)
	if len(bookings) != 1 {
		t.Errorf("expected 1 booking, got %d", len(bookings))
	}
}

func TestDashboardConversationToggleAI(t *testing.T) {
	r, _, database := setupTestRouter(t)

	// Contact doesn't exist yet -> creates Contact with ai_enabled: false
	bodyDisable := bytes.NewReader([]byte(`{"enabled": false}`))
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/dashboard/api/conversations/6283003/ai", bodyDisable)
	req1.Header.Set("X-API-Key", "secret123")
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w1.Code)
	}

	var c model.Contact
	if err := database.Where("phone = ?", "6283003").First(&c).Error; err != nil {
		t.Fatalf("contact not created: %v", err)
	}
	if c.AiEnabled != false {
		t.Errorf("expected ai_enabled false, got true")
	}

	// Re-enable
	bodyEnable := bytes.NewReader([]byte(`{"enabled": true}`))
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/dashboard/api/conversations/6283003/ai", bodyEnable)
	req2.Header.Set("X-API-Key", "secret123")
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w2.Code)
	}
	database.Where("phone = ?", "6283003").First(&c)
	if c.AiEnabled != true {
		t.Errorf("expected ai_enabled true, got false")
	}
}

func TestDashboardConversationSend(t *testing.T) {
	r, deps, database := setupTestRouter(t)

	// Empty text -> 400
	wBad := httptest.NewRecorder()
	reqBad, _ := http.NewRequest("POST", "/dashboard/api/conversations/6284004/send", bytes.NewReader([]byte(`{"text": "   "}`)))
	reqBad.Header.Set("X-API-Key", "secret123")
	reqBad.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", wBad.Code)
	}

	// Valid send -> 200 and logs to memory
	wOk := httptest.NewRecorder()
	reqOk, _ := http.NewRequest("POST", "/dashboard/api/conversations/6284004/send", bytes.NewReader([]byte(`{"text": "Halo ini admin"}`)))
	reqOk.Header.Set("X-API-Key", "secret123")
	reqOk.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wOk, reqOk)

	if wOk.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wOk.Code)
	}

	var msgs []model.ConversationMessage
	database.Where("phone = ?", "6284004").Find(&msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 conversation message logged, got %d", len(msgs))
	}
	if msgs[0].Role != "model" || msgs[0].Text != "Halo ini admin" {
		t.Errorf("unexpected message logged: %+v", msgs[0])
	}
	_ = deps
}

func TestDashboardBookingsListAndFilter(t *testing.T) {
	r, _, database := setupTestRouter(t)

	treatment1 := model.Treatment{Name: "Hydra Facial", Category: "Facial", Price: 300000, Description: "Hydra"}
	treatment2 := model.Treatment{Name: "Botox Allergan", Category: "Injection", Price: 1500000, Description: "Botox"}
	database.Create(&treatment1)
	database.Create(&treatment2)

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	past10Days := time.Now().AddDate(0, 0, -10).Format("2006-01-02")

	database.Create(&model.Booking{Code: "B1", CustomerName: "Charlie", Phone: "6285001", TreatmentID: treatment1.ID, BookingDate: today, BookingTime: "11:00", Status: "confirmed"})
	database.Create(&model.Booking{Code: "B2", CustomerName: "David", Phone: "6285002", TreatmentID: treatment2.ID, BookingDate: today, BookingTime: "13:00", Status: "pending"})
	database.Create(&model.Booking{Code: "B3", CustomerName: "Eve", Phone: "6285003", TreatmentID: treatment1.ID, BookingDate: yesterday, BookingTime: "15:00", Status: "completed"})
	database.Create(&model.Booking{Code: "B4", CustomerName: "Frank", Phone: "6285004", TreatmentID: treatment2.ID, BookingDate: past10Days, BookingTime: "16:00", Status: "cancelled"})

	// 1. All bookings
	wAll := httptest.NewRecorder()
	reqAll, _ := http.NewRequest("GET", "/dashboard/api/bookings", nil)
	reqAll.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(wAll, reqAll)

	if wAll.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wAll.Code)
	}
	var dataAll struct {
		Stats    map[string]int           `json:"stats"`
		Bookings []map[string]interface{} `json:"bookings"`
	}
	_ = json.Unmarshal(wAll.Body.Bytes(), &dataAll)
	if dataAll.Stats["total"] != 4 {
		t.Errorf("expected 4 total bookings, got %d", dataAll.Stats["total"])
	}
	if dataAll.Stats["today"] != 2 { // B1 & B2 are today and not cancelled
		t.Errorf("expected 2 today bookings, got %d", dataAll.Stats["today"])
	}
	if dataAll.Stats["active"] != 2 { // pending + confirmed
		t.Errorf("expected 2 active bookings, got %d", dataAll.Stats["active"])
	}
	if dataAll.Stats["completed"] != 1 {
		t.Errorf("expected 1 completed booking, got %d", dataAll.Stats["completed"])
	}
	if len(dataAll.Bookings) != 4 {
		t.Errorf("expected 4 bookings returned, got %d", len(dataAll.Bookings))
	}

	// 2. Filter range=today
	wToday := httptest.NewRecorder()
	reqToday, _ := http.NewRequest("GET", "/dashboard/api/bookings?range=today", nil)
	reqToday.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(wToday, reqToday)
	var dataToday struct {
		Bookings []map[string]interface{} `json:"bookings"`
	}
	_ = json.Unmarshal(wToday.Body.Bytes(), &dataToday)
	if len(dataToday.Bookings) != 2 {
		t.Errorf("expected 2 bookings for today, got %d", len(dataToday.Bookings))
	}

	// 3. Filter range=7 (should exclude past10Days)
	w7 := httptest.NewRecorder()
	req7, _ := http.NewRequest("GET", "/dashboard/api/bookings?range=7", nil)
	req7.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(w7, req7)
	var data7 struct {
		Bookings []map[string]interface{} `json:"bookings"`
	}
	_ = json.Unmarshal(w7.Body.Bytes(), &data7)
	if len(data7.Bookings) != 3 {
		t.Errorf("expected 3 bookings for range=7, got %d", len(data7.Bookings))
	}

	// 4. Filter status=completed
	wCompleted := httptest.NewRecorder()
	reqCompleted, _ := http.NewRequest("GET", "/dashboard/api/bookings?status=completed", nil)
	reqCompleted.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(wCompleted, reqCompleted)
	var dataCompleted struct {
		Bookings []map[string]interface{} `json:"bookings"`
	}
	_ = json.Unmarshal(wCompleted.Body.Bytes(), &dataCompleted)
	if len(dataCompleted.Bookings) != 1 || dataCompleted.Bookings[0]["code"] != "B3" {
		t.Errorf("expected 1 booking B3, got %v", dataCompleted.Bookings)
	}

	// 5. Filter search q=facial
	wQuery := httptest.NewRecorder()
	reqQuery, _ := http.NewRequest("GET", "/dashboard/api/bookings?q=facial", nil)
	reqQuery.Header.Set("X-API-Key", "secret123")
	r.ServeHTTP(wQuery, reqQuery)
	var dataQuery struct {
		Bookings []map[string]interface{} `json:"bookings"`
	}
	_ = json.Unmarshal(wQuery.Body.Bytes(), &dataQuery)
	if len(dataQuery.Bookings) != 2 { // B1 and B3 have Hydra Facial
		t.Errorf("expected 2 bookings matching 'facial', got %d", len(dataQuery.Bookings))
	}
}

func TestDashboardBookingUpdateStatus(t *testing.T) {
	r, _, database := setupTestRouter(t)

	treatment := model.Treatment{Name: "Acne Care", Category: "Facial", Price: 250000, Description: "Acne"}
	database.Create(&treatment)
	database.Create(&model.Booking{
		Code:         "BK999",
		CustomerName: "Grace",
		Phone:        "6286001",
		TreatmentID:  treatment.ID,
		BookingDate:  "2026-10-01",
		BookingTime:  "12:00",
		Status:       "pending",
	})

	// 1. Invalid status -> 400
	wBad := httptest.NewRecorder()
	reqBad, _ := http.NewRequest("POST", "/dashboard/api/bookings/BK999/status", bytes.NewReader([]byte(`{"status": "unknown"}`)))
	reqBad.Header.Set("X-API-Key", "secret123")
	reqBad.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", wBad.Code)
	}

	// 2. Non-existent booking -> 404
	w404 := httptest.NewRecorder()
	req404, _ := http.NewRequest("POST", "/dashboard/api/bookings/NONEXIST/status", bytes.NewReader([]byte(`{"status": "confirmed"}`)))
	req404.Header.Set("X-API-Key", "secret123")
	req404.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", w404.Code)
	}

	// 3. Valid update -> 200 and DB updated
	wOk := httptest.NewRecorder()
	reqOk, _ := http.NewRequest("POST", "/dashboard/api/bookings/BK999/status", bytes.NewReader([]byte(`{"status": "confirmed"}`)))
	reqOk.Header.Set("X-API-Key", "secret123")
	reqOk.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wOk, reqOk)

	if wOk.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", wOk.Code)
	}
	var resp map[string]string
	_ = json.Unmarshal(wOk.Body.Bytes(), &resp)
	if resp["code"] != "BK999" || resp["status"] != "confirmed" {
		t.Errorf("unexpected response: %v", resp)
	}

	var b model.Booking
	database.Where("code = ?", "BK999").First(&b)
	if b.Status != "confirmed" {
		t.Errorf("expected DB status 'confirmed', got %q", b.Status)
	}
}

func TestWebhookNilBuffer(t *testing.T) {
	r, deps, _ := setupTestRouter(t)
	deps.Buffer = nil

	payload := map[string]any{
		"data": map[string]any{
			"key": map[string]any{
				"fromMe":    false,
				"remoteJid": "6289999@s.whatsapp.net",
			},
			"pushName": "Test",
			"message": map[string]any{
				"conversation": "Hello",
			},
		},
	}
	body, _ := json.Marshal(payload)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "buffered" || int(resp["pending"].(float64)) != 1 {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestMediaProxyNilStorage(t *testing.T) {
	r, deps, _ := setupTestRouter(t)
	deps.Storage = nil

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/media/sample.jpg", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found with nil storage, got %d", w.Code)
	}
}

func TestDashboardConversationToggleAIEdgeCases(t *testing.T) {
	r, deps, database := setupTestRouter(t)

	// Invalid JSON -> 400
	wBad := httptest.NewRecorder()
	reqBad, _ := http.NewRequest("POST", "/dashboard/api/conversations/628777/ai", bytes.NewReader([]byte("{invalid-json")))
	reqBad.Header.Set("X-API-Key", "secret123")
	reqBad.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", wBad.Code)
	}

	// Memory == nil fallback branch
	deps.Memory = nil
	// Existing contact update
	database.Model(&model.Contact{}).Create(map[string]any{
		"phone":      "6287771",
		"ai_enabled": true,
	})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/dashboard/api/conversations/6287771/ai", bytes.NewReader([]byte(`{"enabled": false}`)))
	req1.Header.Set("X-API-Key", "secret123")
	req1.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w1.Code)
	}

	// New contact create with nil memory
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/dashboard/api/conversations/6287772/ai", bytes.NewReader([]byte(`{"enabled": true}`)))
	req2.Header.Set("X-API-Key", "secret123")
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w2.Code)
	}
}

func TestDashboardConversationSendWithWhatsAppClient(t *testing.T) {
	done := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "SUCCESS"}`))
		if strings.Contains(r.URL.Path, "sendText") {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}))
	defer srv.Close()

	r, deps, _ := setupTestRouter(t)
	waCfg := &config.Config{
		EvolutionAPIURL:   srv.URL,
		EvolutionAPIKey:   "test-key",
		EvolutionInstance: "test-instance",
	}
	waClient := whatsapp.NewWhatsAppClient(waCfg)
	waClient.SetSleepFn(func(time.Duration) {}) // do not sleep in tests
	deps.WhatsApp = waClient

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/dashboard/api/conversations/6288888/send", bytes.NewReader([]byte(`{"text": "Pesan admin"}`)))
	req.Header.Set("X-API-Key", "secret123")
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
	}
}

func TestDashboardBookingUpdateInvalidJSON(t *testing.T) {
	r, _, _ := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/dashboard/api/bookings/BK001/status", bytes.NewReader([]byte("{invalid-json")))
	req.Header.Set("X-API-Key", "secret123")
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}
