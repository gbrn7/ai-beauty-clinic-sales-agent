# 🌸 AI Sales Agent Workshop (Go & Gin Edition)

Implementasi berkinerja tinggi **AI Sales Agent** (Gita — *Customer Care & Sales Specialist* untuk **Glowria Aesthetic Clinic**) menggunakan bahasa pemrograman **Go**, framework **Gin**, ORM **GORM**, dan **Google GenAI SDK**.

Aplikasi ini mengotomatisasi interaksi penjualan dan konsultasi klinik kecantikan: menjawab pertanyaan treatment, cek jadwal kosong, membuat dan membatalkan booking appointment, hingga deteksi pembayaran dan serah terima ke CS manusia (*human takeover*).

---

## 🌟 Fitur Utama

- **🧠 Otak AI Gemini dengan Tool Calling**:
  - Menggunakan SDK resmi [`google.golang.org/genai`](https://pkg.go.dev/google.golang.org/genai) dengan implementasi autonomous agentic loop.
  - Mendukung preservasi `ThoughtSignature` untuk model Gemini reasoning (seperti `gemini-3.1-flash-lite` dengan thinking level).
  - Dilengkapi 6 tool klinik bisnis: `check_availability`, `book_appointment`, `cancel_booking`, `search_treatments`, `get_customer_bookings`, dan `transfer_to_human`.
- **📅 Engine Booking Berbasis Kapasitas Atomik**:
  - Deteksi bentrok jadwal (*overlap*) secara atomik dengan kapasitas maksimal perawat (3 perawat simultan, durasi default 90 menit).
  - Proteksi *concurrency* menggunakan mutex untuk mencegah *double-booking* saat request bersamaan.
- **💬 Integrasi WhatsApp Realistis (Evolution API)**:
  - **Debounce Buffer**: Menampung pesan chat beruntun per nomor telepon selama beberapa detik sebelum diteruskan ke AI, mencegah respons terpotong-potong.
  - **Bubble Split**: Memisahkan jawaban panjang menjadi beberapa bubble chat alami menggunakan pemisah `[NEXT]`.
  - **Typing Indicator**: Simulasi indikator mengetik proporsional dengan panjang teks balasan.
- **🖥️ Dashboard Admin Interaktif**:
  - Monitoring seluruh percakapan customer secara *real-time*.
  - Saklar *Takeover Mode* (AI On/Off) per nomor WhatsApp untuk intervensi manual staf klinik.
  - Kirim pesan manual dan pembaruan status booking (*confirmed*, *completed*, *cancelled*).
- **🗄️ Fleksibilitas Database & Storage**:
  - Database bawaan: **SQLite** (zero setup via pure-Go driver `modernc.org/sqlite` / `glebarez/sqlite`) dengan mode WAL.
  - Mendukung koneksi production: **PostgreSQL**.
  - Dual media storage: Penyimpanan foto klinik via **Cloudflare R2** (S3-compatible) dengan auto fallback ke disk lokal (`media/`).
- **💻 Interactive CLI Simulator**:
  - Menguji logika percakapan dan function calling secara langsung di terminal tanpa ketergantungan WhatsApp atau ngrok.
- **🐳 Kontainerisasi Multi-Platform (Docker & Podman)**:
  - Multi-stage build dengan binary statis (`CGO_ENABLED=0`) dan aset embedded via `//go:embed`.
  - Dilengkapi skrip otomasi [`container.sh`](./container.sh) yang secara cerdas mendeteksi Docker atau Podman.

---

## 🏗️ Arsitektur Sistem

```
WhatsApp Customer
       │ (pesan masuk)
       ▼
Evolution API ────────► POST /webhook (Gin Handler)
                              │
                              ▼
                     Debounce Buffer (per nomor)
                              │ (setelah buffer selesai)
                              ▼
                      processBufferedMessages
                       ┌──────┴──────┐
                       ▼             ▼
              Sliding Memory     Agentic Loop (Gemini)
             (histori & state)       │
                                     ├─► 6 Tools Bisnis (GORM DB)
                                     ▼
                               Respon Agen
                                     │
                               Bubble Split ([NEXT])
                                     │
                                     ▼
WhatsApp Customer ◄─── Evolution API (SendReply)
```

---

## 📂 Struktur Direktori

```
AI-Sales-Agent-Workshop-Go/
├── cmd/
│   ├── server/               # Entrypoint HTTP Server Gin & Dashboard
│   └── cli/                  # Entrypoint Simulator CLI Interaktif
├── internal/
│   ├── agent/                # Gemini SDK agentic loop & prompt compiler
│   ├── buffer/               # Debounce buffer per nomor telepon customer
│   ├── config/               # Pemuatan environment variable & validasi
│   ├── db/                   # Inisialisasi GORM (SQLite/Postgres) & seed data
│   ├── handler/              # HTTP router, webhook, API dashboard, & media handler
│   ├── memory/               # Conversation history sliding window & takeover tracker
│   ├── model/                # Model database GORM (Customer, Booking, Message, dll.)
│   ├── storage/              # Abstraksi media storage (Local disk & Cloudflare R2)
│   ├── tools/                # 6 Business Tools (Booking, Treatment, Capacity)
│   └── whatsapp/             # Klien Evolution API & parser payload pesan
├── docs/
│   └── learning/             # 8 Modul Belajar Komprehensif (M1 - M8)
├── prompts/                  # Template prompt instruksi sistem AI Gita
├── templates/                # Template HTML dashboard admin (embed.FS)
├── container.sh              # Skrip helper container otomatis (Docker / Podman)
├── docker-compose.yml        # Spesifikasi multi-service compose
├── Dockerfile                # Multi-stage Docker build
└── go.mod / go.sum           # Dependensi Go
```

---

## 🚀 Panduan Memulai

### 1. Konfigurasi Lingkungan (`.env`)

Salin file contoh konfigurasi dan lengkapi nilai variabelnya:

```bash
cp .env.example .env
```

Nilai minimal yang wajib diisi:
```dotenv
GEMINI_API_KEY=AIzaSy...                # API key Google Gemini
GEMINI_MODEL=gemini-3.1-flash-lite       # Model rekomendasi
AI_NAME=Gita
COMPANY_NAME=Glowria Aesthetic Clinic
DATABASE_URL=sqlite:///data/clinic.db   # Atau PostgreSQL URI
```

---

### 2. Menjalankan Aplikasi Secara Lokal (Native Go)

Pastikan telah menginstal **Go 1.25** atau lebih baru.

#### A. Jalankan Web Server & Webhook
```bash
go run ./cmd/server
```
- **Dashboard Admin**: [http://localhost:8000/dashboard](http://localhost:8000/dashboard)
- **Healthcheck**: [http://localhost:8000/health](http://localhost:8000/health)
- **Webhook Endpoint**: `http://localhost:8000/webhook`

#### B. Jalankan Simulator CLI
Gunakan simulator terminal interaktif untuk menguji prompt dan tools AI tanpa perlu koneksi WhatsApp:
```bash
go run ./cmd/cli
```

#### C. Jalankan Unit Tests
```bash
go test -v ./...

# Untuk pengujian race condition:
go test -race ./...
```

---

### 3. Menjalankan via Container (Docker / Podman)

Repositori ini menyediakan script manajemen container cerdas [`container.sh`](./container.sh) yang secara otomatis mendeteksi ketersediaan Docker Engine atau Podman di sistem Anda:

```bash
# Menampilkan bantuan dan daftar perintah
./container.sh help

# 1. Build image container
./container.sh build

# 2. Jalankan Sales Agent saja (mode default: Port 8000)
./container.sh up

# 3. Jalankan Full Stack (Sales Agent + Evolution WhatsApp lokal di Port 8083)
./container.sh up:full

# 4. Jalankan Sales Agent + PostgreSQL lokal (Port 5439)
./container.sh up:postgres

# 5. Jalankan SEMUA service (Sales Agent + Evolution API + PostgreSQL + Redis)
./container.sh up:all

# 6. Buka Simulator CLI interaktif di dalam container
./container.sh cli

# 7. Pantau logs aplikasi
./container.sh logs

# 8. Cek status container
./container.sh status

# 9. Hentikan seluruh container
./container.sh down
```

> **Catatan Podman (Rootless Linux)**:  
> Direktori `./data` dan `./media` di-*mount* dengan flag `:z` di [`docker-compose.yml`](./docker-compose.yml) untuk kompatibilitas penuh dengan relabeling konteks SELinux.

---

## 🌐 Endpoint HTTP Utama

| Method | Endpoint | Deskripsi |
|---|---|---|
| `GET` | `/` | Redirect ke `/dashboard` |
| `GET` | `/dashboard` | Tampilan web dashboard monitoring admin |
| `GET` | `/health` | Pemeriksaan kesehatan aplikasi & nilai buffer debounce |
| `POST` | `/webhook` | Penerimaan payload pesan masuk dari Evolution API |
| `GET` | `/media/:fname` | Akses file gambar lokal (jika tidak menggunakan R2) |
| `GET` | `/dashboard/api/conversations` | Daftar riwayat chat per pelanggan |
| `GET` | `/dashboard/api/bookings` | Daftar reservasi perawatan |
| `POST` | `/dashboard/api/conversations/:phone/toggle` | Toggle manual saklar AI Takeover |
| `POST` | `/dashboard/api/send` | Pengiriman pesan manual dari admin ke customer |

---

## 📚 Modul Pembelajaran (Learning Path)

Untuk mempelajari arsitektur dan kode secara mendalam langkah-demi-langkah, kunjungi direktori [`docs/learning/`](./docs/learning/README.md):

| Modul | Dokumen | Fokus Pembahasan |
|---|---|---|
| **01** | [Config & Entrypoint](./docs/learning/01-config-dan-entrypoint.md) | Fondasi, env loader, struktur Go idiomatik, `cmd/server` & `cmd/cli` |
| **02** | [Database GORM](./docs/learning/02-database-gorm.md) | 7 model entitas, AutoMigrate, WAL mode SQLite, database seeding |
| **03** | [AI Business Tools](./docs/learning/03-ai-tools.md) | 6 tool klinik, kalkulasi overlap booking atomik, proteksi mutex |
| **04** | [Memory Management](./docs/learning/04-memory.md) | Sliding window histori chat, TTL sesi, state human takeover |
| **05** | [Gemini Agentic Loop](./docs/learning/05-gemini-agent.md) | SDK GenAI, autonomous function calling loop, preservasi thought signature |
| **06** | [WhatsApp Buffer & Parser](./docs/learning/06-whatsapp-buffer.md) | Debounce concurrency buffer, bubble split `[NEXT]`, typing indicator |
| **07** | [HTTP Handler & Dashboard](./docs/learning/07-handler-dashboard.md) | Gin routes, webhook parser, dashboard admin, dual storage (R2/Local) |
| **08** | [Kontainerisasi](./docs/learning/08-containerisasi.md) | Multi-stage Docker build, static binary `CGO_ENABLED=0`, Podman rootless |

---

## 📄 Lisensi & Kontribusi

Dibuat untuk keperluan materi pembelajaran dan implementasi production-ready AI Sales Agent. Silakan ajukan Issue atau Pull Request untuk penyempurnaan fitur.
