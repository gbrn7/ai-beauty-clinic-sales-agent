# Modul 1 — Fondasi Project: Config, Struktur, dan Entry Point

> **Estimasi:** 30 menit  
> **Prasyarat:** Go terinstall, repo sudah di-clone

---

## Tujuan Modul

Setelah modul ini kamu akan memahami:
- Bagaimana project di-bootstrap dari `main.go`
- Cara konfigurasi dibaca dari file `.env`
- Perbedaan antara binary `server` dan `cli`
- Pattern dependency injection manual (tanpa framework)

---

## 1.1 Struktur Direktori

```
AI-Sales-Agent-Workshop-Go/
├── cmd/
│   ├── server/main.go   ← entry point HTTP server (production)
│   └── cli/main.go      ← entry point CLI interaktif (development/testing)
├── internal/
│   ├── config/          ← membaca environment variables
│   ├── db/              ← koneksi database
│   ├── model/           ← GORM model
│   ├── tools/           ← logika bisnis klinik (AI tools)
│   ├── memory/          ← histori percakapan
│   ├── agent/           ← Gemini AI agent loop
│   ├── whatsapp/        ← client WhatsApp Evolution API
│   ├── buffer/          ← debounce buffer pesan masuk
│   ├── storage/         ← penyimpanan media (R2 / lokal)
│   └── handler/         ← Gin HTTP routes
├── prompts/
│   └── system.md        ← instruksi sistem untuk Gemini AI
├── templates/
│   └── dashboard.html   ← halaman dashboard admin
├── docs/learning/       ← ← kamu sedang di sini
└── .env.example         ← template konfigurasi
```

---

## 1.2 Membaca Kode Config

Buka file: `internal/config/config.go`

```go
type Config struct {
    GeminiAPIKey    string   // Kunci API Gemini AI
    GeminiModel     string   // Model yang dipakai, default: gemini-2.5-flash
    GeminiThinking  string   // Level reasoning: low, medium, high, atau off
    DatabaseURL     string   // sqlite://clinic.db atau postgres://...
    Port            string   // Port server, default: :8000
    AIName          string   // Nama persona AI, contoh: "Sari"
    CompanyName     string   // Nama klinik, contoh: "Klinik Cantik"
    EvolutionURL    string   // URL Evolution API WhatsApp
    EvolutionKey    string   // API key Evolution
    InstanceName    string   // Nama instance WhatsApp
    DashboardAPIKey string   // Kunci untuk akses dashboard API
    BufferSeconds   int      // Waktu tunggu debounce (detik)
    MemoryWindow    int      // Berapa pesan terakhir diingat AI
    SessionTTLHours int      // Jam sebelum sesi dianggap baru
    // ... R2 storage config
}
```

**Cara kerjanya:**
1. `godotenv.Load()` membaca file `.env` ke environment
2. `os.Getenv("KEY")` membaca setiap variabel
3. Default value diberikan jika variabel kosong

---

## 1.3 Entry Point: cmd/server/main.go

Buka file: `cmd/server/main.go`

Perhatikan urutan inisialisasi — ini adalah **dependency chain**:

```
cfg := config.Load()
         ↓
db := db.InitDB(cfg.DatabaseURL)
         ↓
db.SeedDB(db)        ← isi data awal jika kosong
         ↓
ts := tools.NewToolService(db)
         ↓
mem := memory.NewMemory(db, window, ttl)
         ↓
st := storage.New(cfg)
         ↓
wa := whatsapp.NewClient(cfg)
         ↓
ag := agent.NewAgent(cfg, ts, promptPath)
         ↓
buf := buffer.NewDebounceBuffer(cfg, mem, ag, wa, st)
         ↓
handler.RegisterRoutes(r, &handler.Dependencies{...})
         ↓
r.Run(cfg.Port)      ← mulai listening
```

> **Konsep:** Ini adalah **manual dependency injection** — setiap layer menerima dependensinya lewat constructor. Tidak pakai framework DI seperti Wire atau Dig.

---

## 1.4 Entry Point: cmd/cli/main.go

Buka file: `cmd/cli/main.go`

CLI lebih sederhana — tidak perlu WhatsApp, storage, atau handler:

```
cfg → db → seed → tools → agent
                             ↓
                      loop: baca input → RunAgent() → print response
```

CLI menggunakan nomor HP dummy `6281234567890` untuk simulasi percakapan.

---

## 🧪 Latihan 1.1 — Jalan CLI

```bash
# Setup environment
cp .env.example .env
# Edit .env, isi setidaknya: GEMINI_API_KEY=xxx, AI_NAME=Sari, COMPANY_NAME=Klinik Cantik

# Jalankan CLI
go run ./cmd/cli

# Coba chat:
# > Halo, ada treatment apa?
# > Saya mau booking facial besok jam 10
# > Batalkan booking saya
```

## 🧪 Latihan 1.2 — Tambah Config Baru

Tambahkan variabel config baru `MAX_REPLY_LENGTH` di `config.go`:

```go
// Di struct Config
MaxReplyLength int

// Di fungsi Load()
MaxReplyLength: getEnvInt("MAX_REPLY_LENGTH", 500),
```

Buat helper `getEnvInt`:
```go
func getEnvInt(key string, defaultVal int) int {
    v := os.Getenv(key)
    if v == "" { return defaultVal }
    n, err := strconv.Atoi(v)
    if err != nil { return defaultVal }
    return n
}
```

## 🧪 Latihan 1.3 — Baca Config di Handler

Setelah Latihan 1.2, tampilkan `MaxReplyLength` di endpoint `/health`:
```bash
curl http://localhost:8000/health
# Harusnya muncul field max_reply_length di response
```

---

## ❓ Pertanyaan Refleksi

1. Mengapa `cmd/cli` tidak membutuhkan `WhatsAppClient` dan `DebounceBuffer`?
2. Apa yang terjadi jika `GEMINI_API_KEY` tidak diisi di `.env`?
3. Mengapa dependency diinisialisasi secara berurutan (cfg → db → tools → agent)?
   Apa yang terjadi jika urutannya dibalik?
4. Apa keuntungan dan kerugian manual DI dibanding menggunakan framework DI?

---

## ✅ Checklist Sebelum Lanjut

- [ ] Bisa menjalankan `go run ./cmd/cli` dan chat dengan AI
- [ ] Memahami urutan inisialisasi di `main.go`  
- [ ] Tahu perbedaan `cmd/server` vs `cmd/cli`
- [ ] Bisa menambahkan variabel config baru

**Next:** [Modul 2 — Database & GORM](./02-database-gorm.md)
