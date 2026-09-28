package db

import (
	"gorm.io/gorm"

	"github.com/raygbrn/ai-sales-agent-go/internal/model"
)

func strPtr(s string) *string {
	return &s
}

func SeedDB(db *gorm.DB) error {
	var count int64
	db.Model(&model.Treatment{}).Count(&count)
	if count > 0 {
		return nil
	}

	treatments := []model.Treatment{
		{Name: "Konsultasi Treatment", Category: "konsultasi", Price: 0,
			Description: "Konsultasi GRATIS untuk memilih treatment kecantikan yang tepat.", RequiresDoctor: true},
		{Name: "Konsultasi Penyakit Kulit & Kelamin", Category: "konsultasi", Price: 125000,
			Description: "Konsultasi kondisi medis, penyakit kulit, dan prosedur dokter spesialis.", RequiresDoctor: true},
		{Name: "Facial Acne", Category: "acne", Price: 100000,
			Description: "Pembersihan mendalam untuk kulit berjerawat.", Promo: strPtr("Promo Pelajar Rp 100.000 (tunjukkan kartu pelajar)")},
		{Name: "Paket Acne A (Peeling Acne + Meso Purifying)", Category: "acne", Price: 275000,
			Description: "Kombinasi peeling dan meso untuk jerawat aktif.", Promo: strPtr("Diskon 50% s/d 30 Juni")},
		{Name: "Paket Acne B (IPL Acne + Meso Purifying)", Category: "acne", Price: 425000,
			Description: "IPL untuk peradangan jerawat plus meso purifying.", Promo: strPtr("Diskon 50% s/d 30 Juni")},
		{Name: "Paket Brightening A (Facial Whitening Premium + Infus Brightening)", Category: "brightening", Price: 495000,
			Description: "Mencerahkan kulit dari luar dan dalam.", Promo: strPtr("Diskon 40% s/d 30 Juni")},
		{Name: "Infus Brightening 2x", Category: "brightening", Price: 695000,
			Description: "Paket dua sesi infus vitamin untuk mencerahkan kulit.", Promo: strPtr("Diskon 40% s/d 30 Juni")},
		{Name: "Glowtox (Skinbooster + Botox)", Category: "anti-aging", Price: 1250000,
			Description: "Treatment baru: skinbooster + botox untuk pori dan minyak.", Promo: strPtr("Hemat 50% (normal Rp 2.500.000)"), RequiresDoctor: true},
		{Name: "Botox Allergan Full Face", Category: "anti-aging", Price: 4800000,
			Description: "Botox full face dengan produk Allergan.", Promo: strPtr("Hemat 38% (normal Rp 7.800.000)"), RequiresDoctor: true},
		{Name: "IPL Hair Removal Underarm 4x", Category: "hair-removal", Price: 475000,
			Description: "Paket 4 sesi hair removal area ketiak.", Promo: strPtr("Diskon 50% s/d 30 Juni")},
	}
	if err := db.Create(&treatments).Error; err != nil {
		return err
	}

	hours := []model.ClinicHours{
		{Weekday: 0, OpenTime: "10:00", CloseTime: "18:00"},
		{Weekday: 1, OpenTime: "10:00", CloseTime: "18:00"},
		{Weekday: 2, OpenTime: "10:00", CloseTime: "18:00"},
		{Weekday: 3, OpenTime: "10:00", CloseTime: "18:00"},
		{Weekday: 4, OpenTime: "10:00", CloseTime: "18:00"},
		{Weekday: 5, OpenTime: "10:00", CloseTime: "18:00"},
		{Weekday: 6, OpenTime: "10:00", CloseTime: "16:00"},
	}
	if err := db.Create(&hours).Error; err != nil {
		return err
	}

	doctors := []model.DoctorSchedule{
		{DoctorName: "Dr. Amara, SpDVE", Weekday: 0, StartTime: "12:00", EndTime: "16:00"},
		{DoctorName: "Dr. Sinta", Weekday: 1, StartTime: "11:00", EndTime: "17:00"},
		{DoctorName: "Dr. Amara, SpDVE", Weekday: 2, StartTime: "12:00", EndTime: "16:00"},
		{DoctorName: "Dr. Sinta", Weekday: 3, StartTime: "11:00", EndTime: "17:00"},
		{DoctorName: "Dr. Amara, SpDVE", Weekday: 4, StartTime: "12:00", EndTime: "16:00"},
		{DoctorName: "Dr. Sinta", Weekday: 5, StartTime: "11:00", EndTime: "17:00"},
		{DoctorName: "Dr. Sinta", Weekday: 6, StartTime: "11:00", EndTime: "15:00"},
	}
	if err := db.Create(&doctors).Error; err != nil {
		return err
	}

	specials := []model.SpecialSchedule{
		{ScheduleDate: "2026-06-22", DoctorName: strPtr("Dr. Amara, SpDVE"), DoctorStart: strPtr("14:30"), DoctorEnd: strPtr("16:00"),
			Note: strPtr("Dokter hanya praktek 1,5 jam. Sarankan jam 14:30 ke atas untuk treatment dokter.")},
		{ScheduleDate: "2026-06-24", IsClosed: true, Note: strPtr("KLINIK TUTUP TOTAL")},
		{ScheduleDate: "2026-06-25", IsClosed: true, Note: strPtr("KLINIK TUTUP TOTAL")},
		{ScheduleDate: "2026-06-26", IsClosed: true, Note: strPtr("KLINIK TUTUP TOTAL")},
		{ScheduleDate: "2026-06-27", DoctorName: strPtr("Dr. Sinta"), DoctorStart: strPtr("12:00"), DoctorEnd: strPtr("17:00"),
			Note: strPtr("Dokter mulai lebih siang. Treatment butuh dokter: sarankan jam 12:00 ke atas.")},
	}
	return db.Create(&specials).Error
}
