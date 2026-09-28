# Modul 2 — Database Layer: GORM, Model, Migrasi, dan Seeding

> **Estimasi:** 45 menit  
> **Prasyarat:** Modul 1 selesai

---

## Tujuan Modul

- Memahami 7 tabel database dan relasinya
- Mengerti cara GORM AutoMigrate bekerja
- Memahami SQLite WAL dan kenapa penting
- Mengerti pattern seeding data default

---

## 2.1 Tujuh Model Database

Buka file: `internal/model/models.go`

```
┌──────────────────────────────────────────────────────┐
│                   DATABASE SCHEMA                    │
├──────────────┬───────────────────────────────────────┤
│   Treatment  │ id, name, category, description,      │
│              │ price, duration_minutes                │
├──────────────┼───────────────────────────────────────┤
│   Booking    │ id, code, customer_name, phone,        │
│              │ treatment_id (FK), booking_date,       │
│              │ booking_time, duration_minutes, status │
├──────────────┼───────────────────────────────────────┤
│ ClinicHours  │ id, weekday(0-6), open, close,         │
│              │ is_closed                              │
├──────────────┼───────────────────────────────────────┤
│DoctorSchedule│ id, weekday, doctor_name, start, end   │
├──────────────┼───────────────────────────────────────┤
│SpecialSchedul│ id, schedule_date, is_closed, open,    │
│              │ close, note                            │
├──────────────┼───────────────────────────────────────┤
│Conversation  │ id, phone, role, text, created_at      │
│   Message    │                                        │
├──────────────┼───────────────────────────────────────┤
│   Contact    │ id, phone, name, ai_enabled,           │
│              │ is_returning                           │
└──────────────┴───────────────────────────────────────┘
```

**Konstanta bisnis:**
```go
const (
    TreatmentDurationMinutes = 90  // semua treatment 90 menit
    MaxConcurrent            = 3   // maks 3 customer bersamaan
)
```

---

## 2.2 GORM Model — Cara Mendefinisikan

```go
type Booking struct {
    gorm.Model              // ← embed: ID, CreatedAt, UpdatedAt, DeletedAt (soft delete)
    Code            string  `gorm:"uniqueIndex;not null"`
    CustomerName    string
    Phone           string  `gorm:"index"`  // ← index untuk query cepat
    TreatmentID     uint
    Treatment       Treatment `gorm:"foreignKey:TreatmentID"` // ← relasi
    BookingDate     string  `gorm:"index"`
    BookingTime     string
    DurationMinutes int
    Status          string  `gorm:"default:pending"` // ← nilai default
}
```

**Tag GORM penting:**
| Tag | Fungsi |
|-----|--------|
| `gorm:"primaryKey"` | Primary key |
| `gorm:"uniqueIndex"` | Unique constraint + index |
| `gorm:"index"` | Index biasa (query cepat) |
| `gorm:"not null"` | NOT NULL constraint |
| `gorm:"default:xxx"` | Nilai default |
| `gorm:"foreignKey:ColID"` | Foreign key |

---

## 2.3 InitDB — Koneksi & AutoMigrate

Buka file: `internal/db/db.go`

```go
func InitDB(databaseURL string) (*gorm.DB, error) {
    // 1. Pilih driver berdasarkan URL
    if strings.HasPrefix(databaseURL, "postgres://") {
        dialector = postgres.Open(databaseURL)
    } else {
        dialector = sqlite.Open(sqlitePath)
    }

    // 2. Buka koneksi
    gormDB, err := gorm.Open(dialector, &gorm.Config{
        Logger: logger.Default.LogMode(logger.Silent),
    })

    // 3. SQLite pragma (hanya untuk SQLite)
    sqlDB.Exec("PRAGMA journal_mode=WAL;")     // WAL mode
    sqlDB.Exec("PRAGMA busy_timeout=5000;")    // tunggu 5 detik jika terkunci

    // 4. Auto-migrate semua model
    gormDB.AutoMigrate(&model.Treatment{}, &model.Booking{}, ...)
}
```

### Kenapa WAL Mode Penting?

```
Mode DEFAULT (rollback journal):
  Writer 1 ──write──► LOCK(shared) ← semua reader BLOKIR

WAL Mode:
  Writer 1 ──write──► WAL file
  Reader 1 ──read───► database file (tidak blokir)
  Reader 2 ──read───► database file (tidak blokir)
```

> WAL (Write-Ahead Logging) memungkinkan pembacaan dan penulisan berjalan bersamaan. Sangat penting untuk web server yang menerima banyak request sekaligus.

---

## 2.4 Seeding — Data Awal

Buka file: `internal/db/seed.go`

Pattern seeding yang aman:
```go
func SeedDB(db *gorm.DB) error {
    // Selalu cek dulu apakah sudah ada data
    var count int64
    db.Model(&model.ClinicHours{}).Count(&count)
    if count > 0 {
        return nil  // ← sudah ada, skip
    }

    // Seed data default
    clinicHours := []model.ClinicHours{
        {Weekday: 0, Open: "09:00", Close: "17:00"}, // Senin
        {Weekday: 1, Open: "09:00", Close: "17:00"}, // Selasa
        // ...
        {Weekday: 6, IsClosed: true},                // Minggu
    }
    return db.Create(&clinicHours).Error
}
```

---

## 2.5 GORM Query Dasar

```go
// CREATE
db.Create(&booking)

// READ — semua
var bookings []model.Booking
db.Find(&bookings)

// READ — dengan kondisi
db.Where("phone = ?", phone).Find(&bookings)

// READ — satu record
var b model.Booking
db.Where("code = ?", "BK-001").First(&b)  // error jika tidak ada

// UPDATE — HATI-HATI dengan boolean!
db.Save(&contact)                         // ❌ boolean false diabaikan
db.Model(&contact).Update("ai_enabled", false)  // ✅ benar

// DELETE (soft delete karena ada gorm.Model)
db.Delete(&booking)  // set DeletedAt, tidak benar-benar hapus

// PRELOAD relasi
db.Preload("Treatment").Where("phone = ?", phone).Find(&bookings)
// bookings[i].Treatment sudah terisi otomatis
```

---

## 🧪 Latihan 2.1 — Eksplorasi Database

```bash
# Jalankan server sekali untuk membuat file DB
go run ./cmd/server &
sleep 2
kill %1

# Install sqlite3 jika belum ada
# Buka DB
sqlite3 clinic.db

# Di dalam sqlite3:
.tables                          -- lihat semua tabel
.schema bookings                 -- lihat struktur tabel
SELECT * FROM treatments;        -- lihat data treatment
SELECT * FROM clinic_hours;      -- lihat jam klinik
.quit
```

## 🧪 Latihan 2.2 — Tambah Field ke Model

Tambahkan field `Notes string` pada `Booking`:
```go
type Booking struct {
    // ... field yang ada
    Notes string  // tambahkan ini
}
```

Kemudian:
```bash
go run ./cmd/server &
# AutoMigrate akan menambah kolom notes otomatis
sqlite3 clinic.db ".schema bookings"
# Harusnya terlihat kolom notes
```

## 🧪 Latihan 2.3 — Tambah Treatment Baru

Di `seed.go`, tambahkan treatment baru setelah yang sudah ada:
```go
{
    Name:        "Crystal Peel",
    Category:    "Exfoliasi",
    Description: "Eksfoliasi kulit dengan kristal mikro",
    Price:        750000,
    DurationMinutes: 90,
},
```

> **Catatan:** Karena seed hanya jalan jika tabel kosong, hapus `clinic.db` dulu lalu jalankan ulang.

## 🧪 Latihan 2.4 — Test Database

Jalankan test database:
```bash
go test -v ./internal/db/...
```

Baca `db_test.go` — perhatikan bagaimana test menggunakan SQLite in-memory (`:memory:`) agar test cepat dan tidak membuat file.

---

## ❓ Pertanyaan Refleksi

1. Mengapa `gorm.Model` di-embed di semua model? Field apa yang ditambahkannya?
2. Apa bedanya `db.Save()` vs `db.Model().Update()`? Kapan sebaiknya masing-masing dipakai?
3. Mengapa seeding mengecek `count > 0` terlebih dahulu? Apa yang terjadi jika tidak dicek?
4. `AutoMigrate` hanya menambah kolom, tidak menghapus. Apa konsekuensinya jika kamu menghapus field dari struct?
5. Mengapa `Booking.Phone` punya `gorm:"index"` tapi `Booking.Code` punya `gorm:"uniqueIndex"`?

---

## ✅ Checklist Sebelum Lanjut

- [ ] Bisa membuka `clinic.db` dan melihat struktur tabel
- [ ] Mengerti perbedaan 7 model dan fungsinya
- [ ] Memahami kenapa WAL mode dipakai
- [ ] Berhasil menambah field baru ke model
- [ ] Test `./internal/db/...` semua PASS

**Next:** [Modul 3 — AI Tools](./03-ai-tools.md)
