package tools

import (
	"testing"
	"time"

	"github.com/raygbrn/ai-sales-agent-go/internal/db"
	"github.com/raygbrn/ai-sales-agent-go/internal/model"
)

func setupTestDB(t *testing.T) *ToolService {
	database, err := db.InitDB("sqlite://file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("db init failed: %v", err)
	}
	if err := db.SeedDB(database); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	ts := NewToolService(database)
	ts.NowFunc = func() time.Time {
		return time.Date(2026, 6, 1, 8, 0, 0, 0, time.Local)
	}
	return ts
}

func TestSearchTreatments(t *testing.T) {
	ts := setupTestDB(t)
	res := ts.SearchTreatments("acne")
	if found, ok := res["found"].(bool); !ok || !found {
		t.Errorf("expected acne treatments found, got %v", res)
	}
	treatments, ok := res["treatments"].([]map[string]any)
	if !ok || len(treatments) == 0 {
		t.Errorf("expected treatment list, got %v", res)
	}

	// Search non-existent
	resNone := ts.SearchTreatments("nonexistenttreatmentxyz")
	if found, ok := resNone["found"].(bool); !ok || found {
		t.Errorf("expected not found for nonexistent treatment, got %v", resNone)
	}
}

func TestBookingCapacityAndConflict(t *testing.T) {
	ts := setupTestDB(t)

	// Book 3 slots on 2026-06-15 at 14:00 (capacity 3)
	for _, name := range []string{"Rani", "Sari", "Dewi"} {
		res := ts.BookTreatment(name, "62812000"+name, "Facial Acne", "2026-06-15", "14:00")
		if success, _ := res["success"].(bool); !success {
			t.Fatalf("booking failed for %s: %v", name, res)
		}
	}

	// 4th booking at same time must return SLOT_CONFLICT
	res4 := ts.BookTreatment("Putu", "62812000Putu", "Facial Acne", "2026-06-15", "14:00")
	if success, _ := res4["success"].(bool); success {
		t.Errorf("expected 4th booking to fail, but succeeded")
	}
	if errCode, _ := res4["error"].(string); errCode != "SLOT_CONFLICT" {
		t.Errorf("expected error SLOT_CONFLICT, got %s", errCode)
	}
	if nextSlot, _ := res4["next_available_slot"].(string); nextSlot != "15:30" {
		t.Errorf("expected next_available_slot 15:30, got %s", nextSlot)
	}
}

func TestSpecialScheduleClosed(t *testing.T) {
	ts := setupTestDB(t)
	// 2026-06-25 is marked as closed in special schedule
	res := ts.BookTreatment("Putu", "62812000Putu", "Facial Acne", "2026-06-25", "14:00")
	if success, _ := res["success"].(bool); success {
		t.Errorf("expected booking on closed clinic date to fail")
	}
	if errCode, _ := res["error"].(string); errCode != "CLINIC_CLOSED" {
		t.Errorf("expected CLINIC_CLOSED, got %s", errCode)
	}
}

func TestCancelBookingAndReopen(t *testing.T) {
	ts := setupTestDB(t)
	// Book 3 slots
	var codeToCancel string
	for i, name := range []string{"Rani", "Sari", "Dewi"} {
		res := ts.BookTreatment(name, "62812000"+name, "Facial Acne", "2026-06-15", "14:00")
		if i == 0 {
			codeToCancel = res["booking_code"].(string)
		}
	}

	// Cancel Rani
	cancelRes := ts.CancelBooking(codeToCancel, "berhalangan")
	if success, _ := cancelRes["success"].(bool); !success {
		t.Fatalf("failed to cancel: %v", cancelRes)
	}

	// Cancel again should return ALREADY_CANCELLED
	cancelAgain := ts.CancelBooking(codeToCancel, "berhalangan")
	if errCode, _ := cancelAgain["error"].(string); errCode != "ALREADY_CANCELLED" {
		t.Errorf("expected ALREADY_CANCELLED, got %v", cancelAgain)
	}

	// Cancel nonexistent should return NOT_FOUND
	cancelNonExistent := ts.CancelBooking("INVALID-CODE", "test")
	if errCode, _ := cancelNonExistent["error"].(string); errCode != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %v", cancelNonExistent)
	}

	// Now Putu can book at 14:00
	res := ts.BookTreatment("Putu", "62812000Putu", "Facial Acne", "2026-06-15", "14:00")
	if success, _ := res["success"].(bool); !success {
		t.Errorf("expected booking to succeed after cancellation, got %v", res)
	}
}

func TestPastTimeRejection(t *testing.T) {
	ts := setupTestDB(t)
	yesterday := ts.now().AddDate(0, 0, -1).Format("2006-01-02")
	res := ts.BookTreatment("Budi", "628120001", "Facial Acne", yesterday, "14:00")
	if success, _ := res["success"].(bool); success {
		t.Errorf("expected past date booking to fail")
	}
	if errCode, _ := res["error"].(string); errCode != "PAST_TIME" {
		t.Errorf("expected PAST_TIME error, got %v", res["error"])
	}

	// Booking today but earlier hour (mock now is 08:00, booking at 07:00)
	today := ts.now().Format("2006-01-02")
	resToday := ts.BookTreatment("Budi", "628120001", "Facial Acne", today, "07:00")
	if success, _ := resToday["success"].(bool); success {
		t.Errorf("expected past hour today booking to fail")
	}
	if errCode, _ := resToday["error"].(string); errCode != "PAST_TIME" {
		t.Errorf("expected PAST_TIME error, got %v", resToday["error"])
	}
}

func TestOutsideHoursRejection(t *testing.T) {
	ts := setupTestDB(t)
	// Monday 2026-06-15 hours: 10:00 - 18:00, last valid slot is 16:30 (18:00 - 90 min)
	resEarly := ts.BookTreatment("Budi", "628120001", "Facial Acne", "2026-06-15", "08:00")
	if success, _ := resEarly["success"].(bool); success {
		t.Errorf("expected early booking to fail")
	}
	if errCode, _ := resEarly["error"].(string); errCode != "OUTSIDE_HOURS" {
		t.Errorf("expected OUTSIDE_HOURS error, got %v", resEarly["error"])
	}

	resLate := ts.BookTreatment("Budi", "628120001", "Facial Acne", "2026-06-15", "17:00")
	if success, _ := resLate["success"].(bool); success {
		t.Errorf("expected late booking to fail")
	}
	if errCode, _ := resLate["error"].(string); errCode != "OUTSIDE_HOURS" {
		t.Errorf("expected OUTSIDE_HOURS error, got %v", resLate["error"])
	}
}

func TestCheckAvailableSchedule(t *testing.T) {
	ts := setupTestDB(t)

	// Regular day
	sched := ts.CheckAvailableSchedule("2026-06-15")
	if status, _ := sched["status"].(string); status != "OK" {
		t.Fatalf("expected status OK, got %v", sched)
	}
	slots, ok := sched["available_slots"].([]map[string]any)
	if !ok || len(slots) == 0 {
		t.Fatalf("expected available slots, got %v", sched)
	}

	// Closed day
	closedSched := ts.CheckAvailableSchedule("2026-06-25")
	if status, _ := closedSched["status"].(string); status != "CLINIC_CLOSED" {
		t.Fatalf("expected status CLINIC_CLOSED, got %v", closedSched)
	}

	// Invalid date format
	invalidSched := ts.CheckAvailableSchedule("invalid-date")
	if status, _ := invalidSched["status"].(string); status != "ERROR" {
		t.Fatalf("expected status ERROR for invalid date, got %v", invalidSched)
	}
}

func TestGetMyOrders(t *testing.T) {
	ts := setupTestDB(t)
	phone := "628123456789"

	// Initial orders should be empty
	emptyRes := ts.GetMyOrders(phone)
	if found, _ := emptyRes["found"].(bool); found {
		t.Errorf("expected found=false for new phone, got %v", emptyRes)
	}

	// Book a treatment
	bookRes := ts.BookTreatment("Sari", phone, "Facial Acne", "2026-06-15", "11:00")
	if success, _ := bookRes["success"].(bool); !success {
		t.Fatalf("booking failed: %v", bookRes)
	}

	// Retrieve orders
	ordersRes := ts.GetMyOrders(phone)
	if found, _ := ordersRes["found"].(bool); !found {
		t.Fatalf("expected found=true, got %v", ordersRes)
	}
	bookings, ok := ordersRes["bookings"].([]map[string]any)
	if !ok || len(bookings) != 1 {
		t.Fatalf("expected 1 booking in orders, got %v", ordersRes)
	}
	if bCode, _ := bookings[0]["booking_code"].(string); bCode != bookRes["booking_code"].(string) {
		t.Errorf("expected booking_code %v, got %v", bookRes["booking_code"], bCode)
	}
}

func TestHandoverToAdmin(t *testing.T) {
	ts := setupTestDB(t)
	phone := "628999888777"

	res := ts.HandoverToAdmin(phone, "Customer marah minta refund")
	if success, _ := res["success"].(bool); !success {
		t.Fatalf("handover failed: %v", res)
	}

	var contact model.Contact
	if err := ts.db.Where("phone = ?", phone).First(&contact).Error; err != nil {
		t.Fatalf("contact not found: %v", err)
	}
	if contact.AiEnabled {
		t.Errorf("expected AiEnabled to be false after handover")
	}
}

func TestDeclarationsAndExecuteTool(t *testing.T) {
	ts := setupTestDB(t)

	decls := ts.GetFunctionDeclarations()
	if len(decls) != 6 {
		t.Fatalf("expected 6 function declarations, got %d", len(decls))
	}

	expectedNames := map[string]bool{
		"search_treatments":        false,
		"check_available_schedule": false,
		"book_treatment":           false,
		"get_my_orders":            false,
		"cancel_booking":           false,
		"handover_to_admin":        false,
	}

	for _, decl := range decls {
		if _, ok := expectedNames[decl.Name]; ok {
			expectedNames[decl.Name] = true
		} else {
			t.Errorf("unexpected declaration name: %s", decl.Name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("missing declaration for %s", name)
		}
	}

	// Test ExecuteTool dispatcher
	execSearch := ts.ExecuteTool("search_treatments", map[string]any{"query": "acne"})
	if found, _ := execSearch["found"].(bool); !found {
		t.Errorf("ExecuteTool search_treatments failed: %v", execSearch)
	}

	execUnknown := ts.ExecuteTool("unknown_tool", map[string]any{})
	if _, ok := execUnknown["error"]; !ok {
		t.Errorf("expected error for unknown tool, got %v", execUnknown)
	}
}
