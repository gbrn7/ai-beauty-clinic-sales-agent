package tools

import (
	"fmt"

	"google.golang.org/genai"
)

func (ts *ToolService) GetFunctionDeclarations() []*genai.FunctionDeclaration {
	return []*genai.FunctionDeclaration{
		{
			Name: "search_treatments",
			Description: "Cari treatment, paket, atau layanan konsultasi yang tersedia di klinik " +
				"berdasarkan kata kunci (nama, kategori seperti 'acne'/'brightening', atau keluhan). " +
				"WAJIB dipanggil sebelum menyebut nama/harga treatment apapun ke customer. Jangan pernah mengarang treatment dari ingatan.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"query": {
						Type:        genai.TypeString,
						Description: "Kata kunci, contoh: 'acne', 'botox', 'konsultasi', 'brightening'",
					},
				},
				Required: []string{"query"},
			},
		},
		{
			Name: "check_available_schedule",
			Description: "Cek slot tersedia untuk satu tanggal (MODE B: HANYA saat customer belum " +
				"menyebut jam spesifik, atau saat butuh next_available_slot). Mengembalikan jam buka/tutup, " +
				"dokter yang bertugas, daftar slot dengan sisa kapasitas, dan catatan jadwal khusus.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"booking_date": {
						Type:        genai.TypeString,
						Description: "Tanggal format YYYY-MM-DD, contoh: 2026-06-15",
					},
				},
				Required: []string{"booking_date"},
			},
		},
		{
			Name: "book_treatment",
			Description: "Buat booking treatment/konsultasi. Ini SATU-SATUNYA konfirmasi final " +
				"ketersediaan slot (atomic check). Panggil HANYA setelah semua data lengkap dan " +
				"customer konfirmasi recap. Jika hasilnya SLOT_CONFLICT, tawarkan next_available_slot dari hasil tool, JANGAN mengarang jam.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"customer_name":  {Type: genai.TypeString, Description: "Nama customer"},
					"phone":          {Type: genai.TypeString, Description: "Nomor WhatsApp customer (otomatis dari konteks, jangan tanya)"},
					"treatment_name": {Type: genai.TypeString, Description: "Nama treatment persis dari hasil search_treatments"},
					"booking_date":   {Type: genai.TypeString, Description: "Format YYYY-MM-DD"},
					"booking_time":   {Type: genai.TypeString, Description: "Format HH:MM, contoh: 14:00"},
				},
				Required: []string{"customer_name", "phone", "treatment_name", "booking_date", "booking_time"},
			},
		},
		{
			Name: "get_my_orders",
			Description: "Ambil riwayat booking customer berdasarkan nomor WhatsApp. Dipakai saat " +
				"customer tanya booking mereka, mau reschedule, atau mau cancel. " +
				"Jika data tidak ditemukan padahal customer yakin pernah booking -> handover.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"phone": {Type: genai.TypeString, Description: "Nomor WhatsApp customer"},
				},
				Required: []string{"phone"},
			},
		},
		{
			Name:        "cancel_booking",
			Description: "Batalkan booking berdasarkan kode booking (dapatkan dari get_my_orders dulu).",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"booking_code": {Type: genai.TypeString, Description: "Kode booking, contoh: GLW-20260613-A3F1"},
					"reason":       {Type: genai.TypeString, Description: "Alasan pembatalan dari customer"},
				},
				Required: []string{"booking_code"},
			},
		},
		{
			Name: "handover_to_admin",
			Description: "Alihkan percakapan ke admin manusia dan nonaktifkan AI untuk nomor ini. " +
				"Panggil saat: customer minta bicara admin, komplain berat, data yang ditanya tidak ada di sistem " +
				"(maksimal 1x coba tool lain dulu), atau situasi di luar kemampuan.",
			Parameters: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"phone":  {Type: genai.TypeString, Description: "Nomor WhatsApp customer"},
					"reason": {Type: genai.TypeString, Description: "Alasan handover untuk dicatat ke admin"},
				},
				Required: []string{"phone", "reason"},
			},
		},
	}
}

func (ts *ToolService) ExecuteTool(name string, args map[string]any) map[string]any {
	switch name {
	case "search_treatments":
		q, _ := args["query"].(string)
		return ts.SearchTreatments(q)
	case "check_available_schedule":
		d, _ := args["booking_date"].(string)
		return ts.CheckAvailableSchedule(d)
	case "book_treatment":
		name, _ := args["customer_name"].(string)
		phone, _ := args["phone"].(string)
		tName, _ := args["treatment_name"].(string)
		date, _ := args["booking_date"].(string)
		time, _ := args["booking_time"].(string)
		return ts.BookTreatment(name, phone, tName, date, time)
	case "get_my_orders":
		phone, _ := args["phone"].(string)
		return ts.GetMyOrders(phone)
	case "cancel_booking":
		code, _ := args["booking_code"].(string)
		reason, _ := args["reason"].(string)
		return ts.CancelBooking(code, reason)
	case "handover_to_admin":
		phone, _ := args["phone"].(string)
		reason, _ := args["reason"].(string)
		return ts.HandoverToAdmin(phone, reason)
	default:
		return map[string]any{"error": fmt.Sprintf("Tool '%s' tidak dikenal.", name)}
	}
}
