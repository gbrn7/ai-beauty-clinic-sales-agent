# AI Sales Agent Workshop (Go & Gin Version)

Implementasi ulang AI Sales Agent (Gita - Glowria Aesthetic Clinic) menggunakan bahasa Go dan framework Gin.

## Fitur Utama

- **Otak AI Gemini**: Menggunakan `google.golang.org/genai` dengan Function Calling otomatis (6 tools klinik).
- **Engine Booking Berbasis Kapasitas**: Deteksi tumpang tindih waktu (overlap) booking secara atomik (kapasitas 3 perawat, durasi 90 menit).
- **Integrasi WhatsApp via Evolution API**: Buffer debounce per nomor, simulasi typing indicator proporsional, dan bubble message split (`[NEXT]`).
- **Dashboard Admin**: Monitoring percakapan, saklar takeover AI on/off, kirim pesan manual, dan manajemen status booking.
- **Penyimpanan Media Fleksibel**: Support Cloudflare R2 (S3-compatible) dengan auto fallback ke disk lokal (`media/`).
- **Mode CLI**: Percakapan interaktif langsung di terminal untuk testing instan.

## Menjalankan Aplikasi

### 1. Konfigurasi
Salin `.env.example` ke `.env` dan isi `GEMINI_API_KEY`:
```bash
cp .env.example .env
```

### 2. Jalankan Server Web (Gin)
```bash
go run ./cmd/server
```
Buka browser di `http://localhost:8000/dashboard`.

### 3. Jalankan CLI Interaktif
```bash
go run ./cmd/cli
```

### 4. Menjalankan via Container (Docker / Podman)
Gunakan helper script `container.sh`:
```bash
./container.sh up         # Jalankan Sales Agent saja
./container.sh up:full    # Sales Agent + Evolution API
./container.sh cli        # Mode CLI simulator di dalam container
./container.sh down       # Hentikan semua container
./container.sh help       # Tampilkan daftar perintah lengkap
```

### 5. Menjalankan Unit Tests
```bash
go test -v ./...
```

