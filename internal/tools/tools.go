package tools

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/raygbrn/ai-sales-agent-go/internal/model"
)

var Hari = []string{"Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}

type ToolService struct {
	db      *gorm.DB
	NowFunc func() time.Time
}

func NewToolService(database *gorm.DB) *ToolService {
	return &ToolService{db: database}
}

func (ts *ToolService) now() time.Time {
	if ts.NowFunc != nil {
		return ts.NowFunc()
	}
	return time.Now()
}

func parseTimeMinutes(s string) int {
	var h, m int
	_, _ = fmt.Sscanf(s, "%d:%d", &h, &m)
	return h*60 + m
}

func formatMinutes(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

type dayRules struct {
	IsClosed    bool
	Open        string
	Close       string
	DoctorName  *string
	DoctorStart *string
	DoctorEnd   *string
	Note        *string
}

func (ts *ToolService) getDayRules(d time.Time, dateStr string) dayRules {
	weekday := int(d.Weekday()) - 1
	if weekday < 0 {
		weekday = 6 // Minggu = 6
	}

	var hours model.ClinicHours
	err := ts.db.Where("weekday = ?", weekday).First(&hours).Error

	rules := dayRules{
		IsClosed: err != nil,
		Open:     hours.OpenTime,
		Close:    hours.CloseTime,
	}

	var doc model.DoctorSchedule
	if err := ts.db.Where("weekday = ?", weekday).First(&doc).Error; err == nil {
		rules.DoctorName = &doc.DoctorName
		rules.DoctorStart = &doc.StartTime
		rules.DoctorEnd = &doc.EndTime
	}

	var special model.SpecialSchedule
	if err := ts.db.Where("schedule_date = ?", dateStr).First(&special).Error; err == nil {
		rules.Note = special.Note
		if special.IsClosed {
			rules.IsClosed = true
		}
		if special.OpenTime != nil {
			rules.Open = *special.OpenTime
		}
		if special.CloseTime != nil {
			rules.Close = *special.CloseTime
		}
		if special.DoctorName != nil {
			rules.DoctorName = special.DoctorName
			rules.DoctorStart = special.DoctorStart
			rules.DoctorEnd = special.DoctorEnd
		}
	}

	return rules
}

func (ts *ToolService) countOverlap(dateStr, start string, duration int) int {
	s1 := parseTimeMinutes(start)
	e1 := s1 + duration

	var bookings []model.Booking
	ts.db.Where("booking_date = ? AND status != ?", dateStr, "cancelled").Find(&bookings)

	count := 0
	for _, b := range bookings {
		s2 := parseTimeMinutes(b.BookingTime)
		e2 := s2 + b.DurationMinutes
		if s1 < e2 && s2 < e1 {
			count++
		}
	}
	return count
}

func (ts *ToolService) nextAvailableSlot(d time.Time, dateStr, after string, rules dayRules) *string {
	lastValid := parseTimeMinutes(rules.Close) - model.TreatmentDurationMinutes
	m := parseTimeMinutes(after) + 30
	m += (30 - m%30) % 30

	for m <= lastValid {
		slotTime := formatMinutes(m)
		if ts.countOverlap(dateStr, slotTime, model.TreatmentDurationMinutes) < model.MaxConcurrent {
			return &slotTime
		}
		m += 30
	}
	return nil
}

func generateBookingCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	suffix := make([]byte, 4)
	for i := range suffix {
		suffix[i] = charset[rand.Intn(len(charset))]
	}
	return fmt.Sprintf("GLW-%s-%s", time.Now().Format("20060102"), string(suffix))
}

func (ts *ToolService) SearchTreatments(query string) map[string]any {
	q := "%" + strings.ToLower(query) + "%"
	var treatments []model.Treatment
	ts.db.Where("LOWER(name) LIKE ? OR LOWER(category) LIKE ? OR LOWER(description) LIKE ?", q, q, q).Find(&treatments)

	if len(treatments) == 0 {
		return map[string]any{
			"found":   false,
			"message": fmt.Sprintf("Tidak ada treatment yang cocok dengan '%s'.", query),
		}
	}

	var results []map[string]any
	for _, t := range treatments {
		results = append(results, map[string]any{
			"id":               t.ID,
			"name":             t.Name,
			"price":            t.Price,
			"duration_minutes": model.TreatmentDurationMinutes,
			"description":      t.Description,
			"promo":            t.Promo,
			"requires_doctor":  t.RequiresDoctor,
		})
	}
	return map[string]any{"found": true, "treatments": results}
}

func (ts *ToolService) CheckAvailableSchedule(bookingDate string) map[string]any {
	d, err := time.Parse("2006-01-02", bookingDate)
	if err != nil {
		return map[string]any{"status": "ERROR", "message": "Format tanggal salah. Gunakan YYYY-MM-DD"}
	}

	weekday := int(d.Weekday()) - 1
	if weekday < 0 {
		weekday = 6
	}
	dayName := Hari[weekday]

	rules := ts.getDayRules(d, bookingDate)
	if rules.IsClosed {
		return map[string]any{
			"status": "CLINIC_CLOSED",
			"date":   bookingDate,
			"day":    dayName,
			"note":   rules.Note,
		}
	}

	lastValid := parseTimeMinutes(rules.Close) - model.TreatmentDurationMinutes
	m := parseTimeMinutes(rules.Open)

	now := ts.now()
	if bookingDate == now.Format("2006-01-02") {
		nowM := now.Hour()*60 + now.Minute()
		if m < nowM {
			m = nowM + (30-nowM%30)%30
		}
	}

	var slots []map[string]any
	for m <= lastValid {
		slotTime := formatMinutes(m)
		used := ts.countOverlap(bookingDate, slotTime, model.TreatmentDurationMinutes)
		if used < model.MaxConcurrent {
			slots = append(slots, map[string]any{
				"start":              slotTime,
				"end":                formatMinutes(m + model.TreatmentDurationMinutes),
				"remaining_capacity": model.MaxConcurrent - used,
			})
		}
		m += 30
	}

	var doctorInfo any = nil
	if rules.DoctorName != nil {
		doctorInfo = map[string]any{
			"name":  *rules.DoctorName,
			"start": *rules.DoctorStart,
			"end":   *rules.DoctorEnd,
		}
	}

	return map[string]any{
		"status":          "OK",
		"date":            bookingDate,
		"day":             dayName,
		"open":            rules.Open,
		"close":           rules.Close,
		"last_valid_slot": formatMinutes(lastValid),
		"doctor_on_duty":  doctorInfo,
		"available_slots": slots,
		"all_full":        len(slots) == 0,
		"max_concurrent":  model.MaxConcurrent,
		"note":            rules.Note,
	}
}

func (ts *ToolService) BookTreatment(customerName, phone, treatmentName, bookingDate, bookingTime string) map[string]any {
	d, err := time.Parse("2006-01-02", bookingDate)
	if err != nil {
		return map[string]any{"success": false, "error": "INVALID_DATE", "message": "Format tanggal salah. Gunakan YYYY-MM-DD"}
	}

	weekday := int(d.Weekday()) - 1
	if weekday < 0 {
		weekday = 6
	}
	dayName := Hari[weekday]

	rules := ts.getDayRules(d, bookingDate)
	if rules.IsClosed {
		msg := fmt.Sprintf("Klinik tutup pada %s, %s.", dayName, bookingDate)
		if rules.Note != nil {
			msg += " Catatan: " + *rules.Note
		}
		return map[string]any{"success": false, "error": "CLINIC_CLOSED", "message": msg}
	}

	startM := parseTimeMinutes(bookingTime)
	now := ts.now()
	todayStr := now.Format("2006-01-02")
	nowM := now.Hour()*60 + now.Minute()

	if bookingDate < todayStr || (bookingDate == todayStr && startM <= nowM) {
		var nextSlotVal any = nil
		if bookingDate == todayStr {
			if slot := ts.nextAvailableSlot(d, bookingDate, formatMinutes(nowM), rules); slot != nil {
				nextSlotVal = *slot
			}
		}
		return map[string]any{
			"success":             false,
			"error":               "PAST_TIME",
			"message":             fmt.Sprintf("Jam %s pada %s sudah lewat. Tidak bisa booking untuk waktu yang sudah berlalu.", bookingTime, bookingDate),
			"next_available_slot": nextSlotVal,
		}
	}

	lastValid := parseTimeMinutes(rules.Close) - model.TreatmentDurationMinutes
	if startM < parseTimeMinutes(rules.Open) || startM > lastValid {
		return map[string]any{
			"success": false,
			"error":   "OUTSIDE_HOURS",
			"message": fmt.Sprintf("Jam %s tidak valid. Klinik buka %s-%s, slot terakhir %s (treatment %d menit harus selesai sebelum tutup).",
				bookingTime, rules.Open, rules.Close, formatMinutes(lastValid), model.TreatmentDurationMinutes),
		}
	}

	var treatment model.Treatment
	if err := ts.db.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(treatmentName)+"%").First(&treatment).Error; err != nil {
		return map[string]any{
			"success": false,
			"error":   "TREATMENT_NOT_FOUND",
			"message": fmt.Sprintf("Treatment '%s' tidak ditemukan. Gunakan search_treatments untuk daftar yang tersedia.", treatmentName),
		}
	}

	// Transaction to protect capacity race
	var booking model.Booking
	var nextSlot *string
	err = ts.db.Transaction(func(tx *gorm.DB) error {
		used := 0
		var existingBookings []model.Booking
		tx.Where("booking_date = ? AND status != ?", bookingDate, "cancelled").Find(&existingBookings)
		e1 := startM + model.TreatmentDurationMinutes
		for _, b := range existingBookings {
			s2 := parseTimeMinutes(b.BookingTime)
			e2 := s2 + b.DurationMinutes
			if startM < e2 && s2 < e1 {
				used++
			}
		}

		if used >= model.MaxConcurrent {
			nextSlot = ts.nextAvailableSlot(d, bookingDate, bookingTime, rules)
			return fmt.Errorf("SLOT_CONFLICT")
		}

		booking = model.Booking{
			Code:            generateBookingCode(),
			CustomerName:    customerName,
			Phone:           phone,
			TreatmentID:     treatment.ID,
			BookingDate:     bookingDate,
			BookingTime:     bookingTime,
			DurationMinutes: model.TreatmentDurationMinutes,
			Status:          "pending",
		}
		if err := tx.Create(&booking).Error; err != nil {
			return err
		}

		var contact model.Contact
		if err := tx.Where("phone = ?", phone).First(&contact).Error; err == nil {
			if contact.Name == nil || *contact.Name == "" {
				contact.Name = &customerName
			}
			contact.IsReturning = true
			tx.Save(&contact)
		} else {
			tx.Create(&model.Contact{
				Phone:       phone,
				Name:        &customerName,
				AiEnabled:   true,
				IsReturning: true,
			})
		}
		return nil
	})

	if err != nil {
		if err.Error() == "SLOT_CONFLICT" {
			var nextSlotVal any = nil
			if nextSlot != nil {
				nextSlotVal = *nextSlot
			}
			return map[string]any{
				"success":             false,
				"error":               "SLOT_CONFLICT",
				"message":             fmt.Sprintf("Jam %s sudah penuh.", bookingTime),
				"next_available_slot": nextSlotVal,
			}
		}
		return map[string]any{"success": false, "error": "DB_ERROR", "message": err.Error()}
	}

	var doctorInfo any = nil
	if rules.DoctorName != nil {
		ds := parseTimeMinutes(*rules.DoctorStart)
		de := parseTimeMinutes(*rules.DoctorEnd)
		onDuty := ds <= startM && startM < de
		doctorInfo = map[string]any{
			"name":                    *rules.DoctorName,
			"on_duty_at_booking_time": onDuty,
			"hours":                   fmt.Sprintf("%s-%s", *rules.DoctorStart, *rules.DoctorEnd),
		}
	}

	return map[string]any{
		"success":       true,
		"booking_code":  booking.Code,
		"customer_name": customerName,
		"treatment":     treatment.Name,
		"price":         treatment.Price,
		"date":          bookingDate,
		"day":           dayName,
		"time":          bookingTime,
		"end_time":      formatMinutes(startM + model.TreatmentDurationMinutes),
		"doctor":        doctorInfo,
		"status":        "pending",
	}
}

func (ts *ToolService) GetMyOrders(phone string) map[string]any {
	var bookings []model.Booking
	ts.db.Preload("Treatment").Where("phone = ?", phone).Order("booking_date DESC").Find(&bookings)

	if len(bookings) == 0 {
		return map[string]any{"found": false, "message": "Tidak ada riwayat booking untuk nomor ini."}
	}

	var list []map[string]any
	for _, b := range bookings {
		list = append(list, map[string]any{
			"booking_code": b.Code,
			"treatment":    b.Treatment.Name,
			"date":         b.BookingDate,
			"time":         b.BookingTime,
			"status":       b.Status,
		})
	}
	return map[string]any{"found": true, "bookings": list}
}

func (ts *ToolService) CancelBooking(bookingCode, reason string) map[string]any {
	var booking model.Booking
	if err := ts.db.Where("code = ?", bookingCode).First(&booking).Error; err != nil {
		return map[string]any{"success": false, "error": "NOT_FOUND", "message": fmt.Sprintf("Booking %s tidak ditemukan.", bookingCode)}
	}
	if booking.Status == "cancelled" {
		return map[string]any{"success": false, "error": "ALREADY_CANCELLED", "message": "Booking ini sudah dibatalkan sebelumnya."}
	}

	booking.Status = "cancelled"
	ts.db.Save(&booking)
	return map[string]any{
		"success":      true,
		"booking_code": bookingCode,
		"message":      "Booking berhasil dibatalkan.",
		"reason":       reason,
	}
}

func (ts *ToolService) HandoverToAdmin(phone, reason string) map[string]any {
	var contact model.Contact
	if err := ts.db.Where("phone = ?", phone).First(&contact).Error; err == nil {
		contact.AiEnabled = false
		ts.db.Model(&contact).Update("ai_enabled", false)
	} else {
		ts.db.Model(&model.Contact{}).Create(map[string]any{
			"phone":      phone,
			"ai_enabled": false,
		})
	}
	return map[string]any{
		"success":         true,
		"ai_disabled_for": phone,
		"reason":          reason,
		"message":         "AI dinonaktifkan untuk nomor ini. Admin telah dinotifikasi.",
	}
}
