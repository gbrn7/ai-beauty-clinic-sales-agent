package model

import "time"

const (
	TreatmentDurationMinutes = 90
	MaxConcurrent            = 3
)

type Treatment struct {
	ID             uint    `gorm:"primaryKey" json:"id"`
	Name           string  `gorm:"index;not null" json:"name"`
	Category       string  `gorm:"index;not null" json:"category"`
	Price          int64   `gorm:"not null" json:"price"`
	Description    string  `gorm:"not null" json:"description"`
	Promo          *string `json:"promo"`
	RequiresDoctor bool    `gorm:"default:false;not null" json:"requires_doctor"`
}

type Booking struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Code            string    `gorm:"uniqueIndex;not null" json:"code"`
	CustomerName    string    `gorm:"not null" json:"customer_name"`
	Phone           string    `gorm:"index;not null" json:"phone"`
	TreatmentID     uint      `gorm:"index;not null" json:"treatment_id"`
	Treatment       Treatment `gorm:"foreignKey:TreatmentID" json:"treatment"`
	BookingDate     string    `gorm:"index;not null" json:"booking_date"` // YYYY-MM-DD
	BookingTime     string    `gorm:"not null" json:"booking_time"`       // HH:MM
	DurationMinutes int       `gorm:"default:90;not null" json:"duration_minutes"`
	Status          string    `gorm:"index;default:pending;not null" json:"status"` // pending | confirmed | completed | cancelled
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type ClinicHours struct {
	Weekday   int    `gorm:"primaryKey;autoIncrement:false" json:"weekday"` // 0=Senin ... 6=Minggu
	OpenTime  string `gorm:"not null" json:"open_time"`                     // HH:MM
	CloseTime string `gorm:"not null" json:"close_time"`                    // HH:MM
}

type DoctorSchedule struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	DoctorName string `gorm:"index;not null" json:"doctor_name"`
	Weekday    int    `gorm:"index;not null" json:"weekday"`
	StartTime  string `gorm:"not null" json:"start_time"` // HH:MM
	EndTime    string `gorm:"not null" json:"end_time"`   // HH:MM
}

type SpecialSchedule struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ScheduleDate string    `gorm:"uniqueIndex;not null" json:"schedule_date"` // YYYY-MM-DD
	IsClosed     bool      `gorm:"default:false;not null" json:"is_closed"`
	OpenTime     *string   `json:"open_time"`
	CloseTime    *string   `json:"close_time"`
	DoctorName   *string   `json:"doctor_name"`
	DoctorStart  *string   `json:"doctor_start"`
	DoctorEnd    *string   `json:"doctor_end"`
	Note         *string   `json:"note"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type ConversationMessage struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Phone     string    `gorm:"index;not null" json:"phone"`
	Role      string    `gorm:"not null" json:"role"` // "user" | "model"
	Text      string    `gorm:"not null" json:"text"`
	CreatedAt time.Time `gorm:"index;autoCreateTime" json:"created_at"`
}

type Contact struct {
	Phone       string    `gorm:"primaryKey" json:"phone"`
	Name        *string   `json:"name"`
	AiEnabled   bool      `gorm:"default:true;not null" json:"ai_enabled"`
	IsReturning bool      `gorm:"default:false;not null" json:"is_returning"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}
