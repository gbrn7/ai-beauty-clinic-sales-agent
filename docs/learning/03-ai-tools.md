# Modul 3 — AI Tools: Logika Bisnis Klinik

> **Estimasi:** 60 menit  
> **Prasyarat:** Modul 2 selesai

---

## Tujuan Modul

- Memahami 6 tool yang bisa dipanggil AI Gemini
- Menguasai algoritma overlap booking
- Memahami hierarki jadwal (special > doctor > clinic hours)
- Mengerti cara concurrency dilindungi dengan mutex
- Memahami Function Declaration untuk Gemini

---

## 3.1 Enam Tool Klinik

Buka file: `internal/tools/tools.go`

| Tool | Parameter Input | Output |
|------|----------------|--------|
| `search_treatments` | `query string` | daftar treatment yang cocok |
| `check_available_schedule` | `booking_date` | slot waktu tersedia + info dokter |
| `book_treatment` | `customer_name, phone, treatment_name, booking_date, booking_time` | kode booking atau error |
| `get_my_orders` | `phone` | riwayat booking customer |
| `cancel_booking` | `booking_code, reason` | konfirmasi pembatalan |
| `handover_to_admin` | `phone, reason` | set ai_enabled=false |

---

## 3.2 Algoritma Overlap Booking

Ini adalah **inti logika bisnis** sistem booking.

### Kondisi Overlap (Tabrakan Jadwal)

Dua slot waktu overlap jika: `start_A < end_B AND start_B < end_A`

```
Contoh visual:
Slot A: [10:00 ─────── 11:30]
Slot B: [10:30 ─────── 12:00]   ← OVERLAP (10:30 < 11:30 && 10:00 < 12:00)

Slot A: [10:00 ─────── 11:30]
Slot C:              [11:30 ─── 13:00]   ← TIDAK OVERLAP (11:30 tidak < 11:30)

Slot A: [10:00 ─────── 11:30]
Slot D:  [09:00 ── 10:00]   ← TIDAK OVERLAP (10:00 tidak < 10:00)
```

```go
// Implementasi di countOverlap()
func (ts *ToolService) countOverlap(dateStr, start string, duration int) int {
    s1 := parseTimeMinutes(start)    // "10:00" → 600
    e1 := s1 + duration              // 600 + 90 = 690 (= 11:30)
    used := 0

    var bookings []model.Booking
    ts.db.Where("booking_date = ? AND status != ?", dateStr, "cancelled").Find(&bookings)

    for _, b := range bookings {
        s2 := parseTimeMinutes(b.BookingTime)
        e2 := s2 + b.DurationMinutes
        if s1 < e2 && s2 < e1 {  // ← formula overlap
            used++
        }
    }
    return used
}
```

### Kenapa Menit, Bukan String?

```go
// String comparison SALAH — "9:30" > "10:00" secara leksikografis!
"9:30" > "10:00"  // true — SALAH!

// Konversi ke menit BENAR
parseTimeMinutes("9:30")  // = 570
parseTimeMinutes("10:00") // = 600
570 < 600                 // true — BENAR
```

---

## 3.3 Hierarki Jadwal

```
Prioritas (dari tertinggi):
   1. SpecialSchedule  → hari libur nasional, event khusus
   2. DoctorSchedule   → jadwal dokter yang berbeda tiap hari  
   3. ClinicHours      → jam buka default per hari dalam seminggu
```

```go
// Implementasi di getDayRules()
func (ts *ToolService) getDayRules(d time.Time, dateStr string) DayRules {
    // 1. Cek SpecialSchedule terlebih dahulu
    var special model.SpecialSchedule
    if ts.db.Where("schedule_date = ?", dateStr).First(&special).Error == nil {
        // special schedule ditemukan → gunakan ini, abaikan sisanya
        if special.IsClosed { return DayRules{IsClosed: true, Note: &special.Note} }
        return DayRules{Open: special.Open, Close: special.Close, ...}
    }

    // 2. Cek DoctorSchedule
    weekday := int(d.Weekday()) - 1
    var doc model.DoctorSchedule
    if ts.db.Where("weekday = ?", weekday).First(&doc).Error == nil {
        rules.DoctorName = &doc.DoctorName
        // ...
    }

    // 3. Default: ClinicHours
    var hours model.ClinicHours
    ts.db.Where("weekday = ?", weekday).First(&hours)
    return DayRules{Open: hours.Open, Close: hours.Close, IsClosed: hours.IsClosed}
}
```

---

## 3.4 Perlindungan Concurrency pada Booking

### Masalah: Race Condition

```
Bayangkan 2 orang booking slot 10:00 bersamaan:

Goroutine A (Customer A)      Goroutine B (Customer B)
countOverlap → 2 (tersedia)   countOverlap → 2 (tersedia)
... (gap waktu kecil) ...
INSERT booking A              INSERT booking B    ← KEDUANYA BERHASIL!
                                                 ← Tapi slot hanya untuk 3!
                                                 ← Jika sudah 2 terpakai → OVERBOOKING!
```

### Solusi: Mutex + Transaction

```go
type ToolService struct {
    db           *gorm.DB
    bookingMutex sync.Mutex  // ← kunci in-process
}

func (ts *ToolService) BookTreatment(...) map[string]any {
    // Kunci — hanya 1 goroutine bisa masuk sekaligus
    ts.bookingMutex.Lock()
    defer ts.bookingMutex.Unlock()

    // GORM Transaction — atomik di level database
    err = ts.db.Transaction(func(tx *gorm.DB) error {
        // Gunakan 'tx' (bukan 'ts.db') untuk semua query di dalam transaction!
        tx.Where("booking_date = ? AND status != ?", date, "cancelled").Find(&bookings)
        if used >= MaxConcurrent { return fmt.Errorf("SLOT_CONFLICT") }
        tx.Create(&booking)
        return nil
    })
}
```

> **Penting:** Di dalam `db.Transaction()`, gunakan `tx` (parameter callback), BUKAN `ts.db`. Jika pakai `ts.db`, akan terjadi deadlock karena transaction dan query luar saling tunggu koneksi.

---

## 3.5 Function Declaration untuk Gemini

Buka file: `internal/tools/declarations.go`

Declaration adalah "spesifikasi" yang diberikan ke Gemini agar AI tahu tool apa yang tersedia dan parameter apa yang dibutuhkan:

```go
// Contoh deklarasi book_treatment
var BookTreatmentDecl = &genai.FunctionDeclaration{
    Name:        "book_treatment",
    Description: "Membuat booking treatment untuk customer",
    Parameters: &genai.Schema{
        Type:     genai.TypeObject,
        Required: []string{"customer_name", "phone", "treatment_name", "booking_date", "booking_time"},
        Properties: map[string]*genai.Schema{
            "customer_name": {
                Type:        genai.TypeString,
                Description: "Nama lengkap customer",
            },
            "booking_date": {
                Type:        genai.TypeString,
                Description: "Tanggal booking format YYYY-MM-DD",
            },
            // ...
        },
    },
}
```

**Dispatcher ExecuteTool:**
```go
func (ts *ToolService) ExecuteTool(name string, args map[string]any) map[string]any {
    switch name {
    case "search_treatments":
        return ts.SearchTreatments(args["query"].(string))
    case "book_treatment":
        return ts.BookTreatment(
            args["customer_name"].(string),
            args["phone"].(string),
            // ...
        )
    // ...
    }
}
```

---

## 🧪 Latihan 3.1 — Jalankan Test Tools

```bash
go test -v -race ./internal/tools/...
```

Baca output setiap test — perhatikan nama test dan apa yang diverifikasi:
- `TestBookingCapacityAndConflict` — uji MaxConcurrent=3
- `TestSpecialScheduleClosed` — uji hari libur
- `TestCancelBookingAndReopen` — uji pembatalan membuka slot
- `TestPastTimeRejection` — uji tidak bisa booking waktu lalu
- `TestBookingConcurrentSafety` — uji race condition

## 🧪 Latihan 3.2 — Visualisasi Overlap

Tulis kode kecil di `main_test.go` untuk memahami formula overlap:

```go
// Coba di Go Playground atau file test sementara
func isOverlap(s1, e1, s2, e2 int) bool {
    return s1 < e2 && s2 < e1
}

// Test berbagai skenario:
fmt.Println(isOverlap(600, 690, 630, 720)) // true  — 10:00-11:30 vs 10:30-12:00
fmt.Println(isOverlap(600, 690, 690, 780)) // false — 10:00-11:30 vs 11:30-13:00
fmt.Println(isOverlap(600, 690, 540, 600)) // false — 10:00-11:30 vs 09:00-10:00
```

## 🧪 Latihan 3.3 — Tambah Tool Baru

Buat tool `get_promo` yang mengembalikan daftar promo hardcoded:

**Langkah 1:** Tambah method di `tools.go`:
```go
func (ts *ToolService) GetPromo() map[string]any {
    return map[string]any{
        "promos": []map[string]any{
            {"name": "Promo Lebaran", "discount": "20%", "valid_until": "2026-04-10"},
            {"name": "Member Baru", "discount": "15%", "valid_until": "2026-12-31"},
        },
    }
}
```

**Langkah 2:** Tambah declaration di `declarations.go`:
```go
var GetPromoDecl = &genai.FunctionDeclaration{
    Name:        "get_promo",
    Description: "Mendapatkan daftar promo yang sedang aktif",
    Parameters:  &genai.Schema{Type: genai.TypeObject, Properties: map[string]*genai.Schema{}},
}
```

**Langkah 3:** Daftarkan di `GetFunctionDeclarations()` dan `ExecuteTool()`.

**Langkah 4:** Test via CLI — tanya ke AI "Ada promo apa sekarang?"

---

## ❓ Pertanyaan Refleksi

1. Mengapa formula overlap `s1 < e2 && s2 < e1` dan bukan `s1 <= e2 && s2 <= e1`?
   (Hint: apa yang terjadi jika booking A selesai jam 11:30 dan booking B mulai jam 11:30?)

2. Jika `MaxConcurrent = 3` dan sudah ada 3 booking jam 10:00, bisakah seseorang booking jam 11:00?
   Mengapa?

3. Mengapa `bookingMutex` tidak cukup saja tanpa `db.Transaction()`?
   (Hint: apa yang terjadi jika server restart di tengah-tengah proses insert?)

4. Apa yang terjadi jika SpecialSchedule ada untuk tanggal tertentu tapi `IsClosed = false`?
   Apakah DoctorSchedule masih dipakai?

5. Mengapa Function Declaration perlu mendefinisikan tipe setiap parameter?
   Apa yang terjadi jika tipe salah?

---

## ✅ Checklist Sebelum Lanjut

- [ ] Semua test `./internal/tools/...` PASS
- [ ] Bisa menjelaskan formula overlap booking dengan kata-kata sendiri
- [ ] Mengerti kapan special schedule mengoverride clinic hours
- [ ] Memahami peran mutex vs transaction dalam mencegah race condition
- [ ] Berhasil menambah tool baru `get_promo`

**Next:** [Modul 4 — Memory](./04-memory.md)
