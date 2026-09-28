# Modul 7 — HTTP Handler: Routes, Dashboard, dan Storage

> **Estimasi:** 45 menit  
> **Prasyarat:** Modul 6 selesai

---

## Tujuan Modul

- Memahami semua route Gin dan fungsinya
- Mengerti pattern Dependencies injection di handler
- Memahami middleware autentikasi API key
- Mengerti cara dashboard admin bekerja
- Memahami Storage Service (R2 vs lokal)

---

## 7.1 Peta Route Lengkap

Buka file: `internal/handler/handler.go`

```
GET  /                           → redirect ke /dashboard
GET  /health                     → status server + buffer_seconds
POST /webhook                    → terima pesan dari Evolution API

GET  /media/:fname               → serve file media (foto customer)

GET  /dashboard                  → halaman HTML (templates/dashboard.html)

────────── Protected by X-API-Key ──────────
GET  /dashboard/api/config                → info company & ai_name
GET  /dashboard/api/conversations         → daftar semua percakapan
GET  /dashboard/api/conversations/:phone  → detail percakapan + booking
POST /dashboard/api/conversations/:phone/ai    → toggle AI on/off
POST /dashboard/api/conversations/:phone/send  → kirim pesan manual
GET  /dashboard/api/bookings              → daftar booking + filter
POST /dashboard/api/bookings/:code/status → update status booking
```

---

## 7.2 Dependencies Injection via Struct

```go
// Semua yang dibutuhkan handler dikumpulkan dalam satu struct
type Dependencies struct {
    Config   *config.Config
    DB       *gorm.DB
    Buffer   *buffer.DebounceBuffer
    Storage  storage.StorageService  // interface, bisa R2 atau lokal
    WhatsApp *whatsapp.WhatsAppClient
    Memory   *memory.Memory
}

// Semua route didaftarkan dalam satu fungsi
func RegisterRoutes(r *gin.Engine, deps *Dependencies) {
    r.GET("/health", healthHandler(deps))
    r.POST("/webhook", webhookHandler(deps))
    // ...
}
```

---

## 7.3 Middleware Autentikasi

Dashboard API dilindungi dengan API key di header:

```go
// Middleware yang dicek sebelum setiap request ke /dashboard/api/*
func apiKeyMiddleware(key string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.GetHeader("X-API-Key") != key {
            c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
            return  // request dihentikan di sini
        }
        c.Next()  // lanjut ke handler asli
    }
}

// Cara mendaftarkan middleware untuk group:
api := r.Group("/dashboard/api", apiKeyMiddleware(deps.Config.DashboardAPIKey))
api.GET("/config", configHandler(deps))
api.GET("/conversations", conversationsHandler(deps))
```

**Cara mengakses API dari curl:**
```bash
curl -H "X-API-Key: your_key_here" http://localhost:8000/dashboard/api/conversations
```

---

## 7.4 Webhook Handler — Pintu Masuk WhatsApp

```go
func webhookHandler(deps *Dependencies) gin.HandlerFunc {
    return func(c *gin.Context) {
        var payload map[string]any
        c.ShouldBindJSON(&payload)

        // Cek event type — hanya proses "messages.upsert"
        if payload["event"] != "messages.upsert" {
            c.JSON(200, gin.H{"status": "ignored"})
            return
        }

        // Parse pesan dari payload Evolution API
        msg := whatsapp.ExtractIncoming(payload)
        if msg == nil {
            c.JSON(200, gin.H{"status": "filtered"})
            return
        }

        // Tambahkan ke debounce buffer — NON-BLOCKING
        count := deps.Buffer.Add(msg)
        c.JSON(200, gin.H{"status": "buffered", "count": count})
        // Response langsung ke Evolution API (< 1 detik)
        // AI processing berjalan di goroutine terpisah
    }
}
```

---

## 7.5 Media Handler — Directory Traversal Protection

```go
func mediaHandler(deps *Dependencies) gin.HandlerFunc {
    return func(c *gin.Context) {
        // KEAMANAN KRITIS: filepath.Base() mencegah path traversal
        // Tanpa ini: GET /media/../../etc/passwd → baca file sistem!
        fname := filepath.Base(c.Param("fname"))  // "../../etc/passwd" → "passwd"

        data, mime, err := deps.Storage.GetMedia(fname)
        if err != nil {
            c.Status(404)
            return
        }

        c.Header("Cache-Control", "public, max-age=86400")  // cache 1 hari
        c.Data(200, mime, data)
    }
}
```

---

## 7.6 Storage Service — Interface Pattern

Buka file: `internal/storage/storage.go`

```go
// Interface memungkinkan implementasi berbeda
type StorageService interface {
    SaveMedia(phone string, data []byte, mime string) (string, error)
    GetMedia(fname string) ([]byte, string, error)
}

// Implementasi 1: Cloudflare R2 (S3 compatible)
// Diaktifkan jika semua 5 env var R2 diisi (R2_BUCKET, R2_ACCOUNT, dst)
type r2Storage struct {
    client s3ClientAPI
    bucket string
}

// Implementasi 2: Local filesystem
// Digunakan jika R2 tidak dikonfigurasi
type localStorageSvc struct {
    dir string  // "media/"
}

// Factory function — pilih implementasi berdasarkan config
func New(cfg *config.Config) StorageService {
    if cfg.IsR2Enabled() {
        return newR2Storage(cfg)
    }
    return newLocalStorage("media")
}
```

**Keuntungan Interface:**
- Handler tidak perlu tahu storage mana yang dipakai
- Mudah diganti tanpa ubah kode handler
- Mudah di-mock untuk testing

---

## 7.7 Dashboard API — Beberapa Contoh Endpoint

**GET /dashboard/api/conversations:**
```go
// Ambil semua nomor HP yang punya percakapan
// Untuk setiap nomor: last_text, last_role, ai_enabled, has_booking
db.Select("DISTINCT phone").From("conversation_messages").Scan(&phones)

for _, phone := range phones {
    var lastMsg model.ConversationMessage
    db.Where("phone = ?", phone).Order("created_at DESC").First(&lastMsg)

    var bookingCount int64
    db.Model(&model.Booking{}).Where("phone = ?", phone).Count(&bookingCount)
    
    results = append(results, gin.H{
        "phone":      phone,
        "last_text":  lastMsg.Text,
        "last_role":  lastMsg.Role,
        "ai_enabled": mem.IsAIEnabled(phone),
        "has_booking": bookingCount > 0,
    })
}
```

**GET /dashboard/api/bookings — filter:**
```bash
# Filter berdasarkan range waktu
GET /dashboard/api/bookings?range=today
GET /dashboard/api/bookings?range=7      # 7 hari terakhir
GET /dashboard/api/bookings?range=30

# Filter berdasarkan status
GET /dashboard/api/bookings?status=pending

# Search customer
GET /dashboard/api/bookings?q=Budi
```

---

## 🧪 Latihan 7.1 — Explore Semua Endpoint

Jalankan server lalu coba setiap endpoint:

```bash
go run ./cmd/server &

# Health check
curl http://localhost:8000/health

# Dashboard (buka di browser)
open http://localhost:8000/dashboard

# API (ganti YOUR_KEY dengan DASHBOARD_API_KEY di .env)
export API_KEY="your_dashboard_api_key"

curl -H "X-API-Key: $API_KEY" http://localhost:8000/dashboard/api/config
curl -H "X-API-Key: $API_KEY" http://localhost:8000/dashboard/api/conversations
curl -H "X-API-Key: $API_KEY" http://localhost:8000/dashboard/api/bookings

# Tanpa API key — harus dapat 401
curl http://localhost:8000/dashboard/api/config
```

## 🧪 Latihan 7.2 — Test Security (Directory Traversal)

```bash
# Ini TIDAK boleh berhasil membaca file di luar folder media/
curl "http://localhost:8000/media/../../etc/passwd"
# Harusnya: 404 Not Found
# Bukan: isi file /etc/passwd
```

## 🧪 Latihan 7.3 — Tambah Endpoint Baru

Tambahkan endpoint `GET /dashboard/api/stats` yang mengembalikan statistik:

```go
// Di handler.go, tambahkan route:
api.GET("/stats", func(c *gin.Context) {
    var totalBookings, pendingBookings, todayBookings int64
    today := time.Now().Format("2006-01-02")
    
    deps.DB.Model(&model.Booking{}).Count(&totalBookings)
    deps.DB.Model(&model.Booking{}).Where("status = ?", "pending").Count(&pendingBookings)
    deps.DB.Model(&model.Booking{}).Where("booking_date = ?", today).Count(&todayBookings)
    
    c.JSON(200, gin.H{
        "total_bookings":   totalBookings,
        "pending_bookings": pendingBookings,
        "today_bookings":   todayBookings,
    })
})
```

## 🧪 Latihan 7.4 — Jalankan Test Handler

```bash
go test -v ./internal/handler/...
```

Perhatikan bagaimana test menggunakan `httptest.NewRecorder()` dan `httptest.NewRequest()` untuk test HTTP tanpa server nyata.

---

## ❓ Pertanyaan Refleksi

1. Mengapa webhook handler mengembalikan respons HTTP **segera** (sebelum AI selesai berpikir)?
   Apa yang terjadi jika Evolution API tidak dapat respons dalam 30 detik?

2. Mengapa Storage menggunakan **interface**, bukan langsung struct R2Storage atau LocalStorage?
   Apa keuntungannya untuk testing?

3. `filepath.Base("../../etc/passwd")` menghasilkan apa? Coba di Go Playground.

4. Mengapa route `/dashboard/api/*` butuh middleware tapi `/health` tidak?

5. Jika `DASHBOARD_API_KEY` tidak diset di `.env`, apa yang terjadi?
   Apakah ini aman? (Hint: lihat bagaimana middleware membandingkan key)

---

## ✅ Checklist Sebelum Lanjut

- [ ] Test `./internal/handler/...` dan `./internal/storage/...` PASS
- [ ] Bisa mengakses semua endpoint via curl
- [ ] Memahami kenapa `filepath.Base()` wajib ada di media handler
- [ ] Berhasil menambah endpoint `/dashboard/api/stats`
- [ ] Mengerti perbedaan R2 storage vs local storage

**Next:** [Modul 8 — Containerisasi](./08-containerisasi.md)
