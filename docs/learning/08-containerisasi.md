# Modul 8 — Containerisasi: Docker & Podman

> **Estimasi:** 30 menit  
> **Prasyarat:** Docker atau Podman terinstall

---

## Tujuan Modul

- Memahami multi-stage Docker build
- Mengerti kenapa `CGO_ENABLED=0` penting untuk Go + Alpine
- Memahami cara `//go:embed` membuat binary self-contained
- Bisa menjalankan aplikasi dengan Docker maupun Podman
- Memahami named volumes untuk persistensi data

---

## 8.1 Mengapa Containerisasi?

**Tanpa container:**
```
"Di komputer saya jalan, di server tidak" ← masalah klasik
- Go version berbeda
- SQLite library berbeda
- Path file berbeda
- Environment variable tidak diset
```

**Dengan container:**
```
Image = binary + dependencies + config = reproducible
- Sama di laptop, staging, production
- Mudah di-deploy ke cloud mana saja
- Mudah di-rollback ke versi lama
```

---

## 8.2 Dockerfile — Multi-Stage Build

Buka file: `Dockerfile`

```dockerfile
# ═══════════════════════════════════════
# STAGE 1: BUILD
# Image besar (~600MB) — hanya untuk compile
# ═══════════════════════════════════════
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Cache dependencies (layer ini hanya rebuild jika go.mod berubah)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Compile — hasilkan binary static (tidak butuh library eksternal)
RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" \   # -s=strip debug, -w=strip DWARF
    -o /out/server ./cmd/server

RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" \
    -o /out/cli ./cmd/cli

# ═══════════════════════════════════════
# STAGE 2: RUNTIME
# Image kecil (~20MB) — hanya yang perlu untuk jalan
# ═══════════════════════════════════════
FROM alpine:3.21

# Buat user non-root (keamanan)
RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

# Salin binary dari stage builder
COPY --from=builder /out/server /app/server
COPY --from=builder /out/cli    /app/cli
# Tidak perlu salin templates/ atau prompts/ karena sudah di-embed ke binary!

# Buat direktori untuk data persisten
RUN mkdir -p /app/data /app/media && chown -R app:app /app

USER app          # Jalankan sebagai non-root
EXPOSE 8000

ENTRYPOINT ["/app/server"]
```

### Perbandingan Ukuran Image:

| Image | Ukuran |
|-------|--------|
| golang:1.25-alpine (builder) | ~600 MB |
| Final runtime (Alpine + binary) | ~25 MB |
| Penghematan | ~96% |

---

## 8.3 Kenapa CGO_ENABLED=0?

Project ini menggunakan `github.com/glebarez/sqlite` — implementasi SQLite **murni Go** tanpa CGO.

```bash
# CGO_ENABLED=1 (default): butuh C compiler di runtime image
# Binary linking ke libc.so → tidak portable

# CGO_ENABLED=0: binary static, semua termasuk di dalam binary
# Tidak butuh C library → bisa jalan di Alpine (musl libc) tanpa masalah
CGO_ENABLED=0 go build → binary yang benar-benar standalone
```

---

## 8.4 go:embed — Binary Self-Contained

Sebelum `//go:embed`, binary butuh file di lokasi tertentu:
```bash
# Harus dijalankan dari direktori yang benar:
cd /home/user/project && ./bin/server
# Jika dari tempat lain: error "prompts/system.md: no such file"
```

Dengan `//go:embed`:
```go
// prompts/prompts.go
//go:embed system.md
var FS embed.FS
// system.md sudah ada di DALAM binary saat kompilasi
```

```bash
# Sekarang bisa dijalankan dari mana saja:
/path/to/anywhere/server  # ← bekerja!
# Binary sudah membawa system.md dan dashboard.html di dalamnya
```

---

## 8.5 docker-compose.yml — Menjalankan dengan Mudah

Buka file: `docker-compose.yml`

```yaml
name: ai-sales-agent-workshop

services:
  app:
    build:
      context: .          # build dari Dockerfile di direktori ini
      dockerfile: Dockerfile
    image: ai-sales-agent-go:latest
    container_name: glowria-sales-agent
    ports:
      - "8000:8000"
    env_file:
      - .env
    environment:
      - DATABASE_URL=${DATABASE_URL:-sqlite:///data/clinic.db}
      - PORT=8000
      - EVOLUTION_API_URL=${EVOLUTION_API_URL:-http://evolution-api:8080}
    volumes:
      # :z flag mengaktifkan SELinux relabeling untuk kompatibilitas Podman rootless
      - ./data:/app/data:z
      - ./media:/app/media:z
    restart: unless-stopped
    healthcheck:
      test: [ "CMD-SHELL", "curl -f http://localhost:8000/ || exit 1" ]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 5s
    networks:
      - sales-agent-network

  evolution-api:
    image: evoapicloud/evolution-api:latest
    container_name: glowria-evolution-api
    profiles:
      - full
      - whatsapp
    ports:
      - "127.0.0.1:8083:8080"
    env_file:
      - .env
    volumes:
      - evolution_instances:/evolution/instances:z
    restart: unless-stopped
    networks:
      - sales-agent-network

  postgres:
    image: postgres:15
    container_name: glowria-postgres
    profiles:
      - postgres
    ports:
      - "5439:5432"
    environment:
      - POSTGRES_DB=clinic
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
    volumes:
      - postgres_data:/var/lib/postgresql/data:z
    restart: unless-stopped
    networks:
      - sales-agent-network

  redis:
    container_name: glowria-redis
    image: redis:7-alpine
    restart: always
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data
    networks:
      - sales-agent-network

volumes:
  evolution_instances:
    name: glowria_evolution_instances
  postgres_data:
    name: glowria_postgres_data
  redis_data:
    name: glowria_redis_data

networks:
  sales-agent-network:
    driver: bridge
```

---

## 8.6 Docker vs Podman — Perbedaan

| Aspek | Docker | Podman |
|-------|--------|--------|
| Daemon | Butuh daemon (background process) | Daemonless (langsung fork) |
| Root | Docker daemon berjalan sebagai root | Bisa rootless (lebih aman) |
| Compose | `docker compose` (built-in v2) | `podman compose` atau `podman-compose` |
| Format Image | OCI compliant | OCI compliant (sama) |
| Dockerfile | Standard | Standard (sama) |

> [!TIP] Untuk developer, keduanya identik dari sisi penggunaan sehari-hari. Perbedaan utama di keamanan (Podman lebih aman karena rootless) dan arsitektur (Podman tidak butuh background daemon).

---

## 🧪 Latihan 8.1 — Build dan Jalankan

**Docker:**
```bash
# Setup
cp .env.example .env
# Edit .env — isi GEMINI_API_KEY

# Build image
docker compose build

# Jalankan
docker compose up -d

# Lihat log
docker compose logs -f

# Akses
open http://localhost:8000/dashboard

# Stop
docker compose down
```

**Podman:**
```bash
# Sama, ganti docker dengan podman
podman compose up -d
podman compose logs -f
podman compose down
```

## 🧪 Latihan 8.2 — Verifikasi Data Persisten

```bash
# Jalankan, buat beberapa booking via CLI atau WhatsApp
docker compose up -d
# ... buat booking ...

# Stop dan hapus container (tapi BUKAN volume)
docker compose down   # volumes tetap ada

# Jalankan ulang
docker compose up -d

# Booking sebelumnya harus masih ada!
curl -H "X-API-Key: $API_KEY" http://localhost:8000/dashboard/api/bookings
```

## 🧪 Latihan 8.3 — Inspect Image

```bash
# Lihat ukuran image
docker images ai-sales-agent-go

# Lihat layer-layer yang dibuat
docker history ai-sales-agent-go:latest

# Masuk ke dalam container (debugging)
docker run --rm -it --entrypoint sh ai-sales-agent-go:latest
ls /app    # harusnya ada: server, cli, data/, media/
```

## 🧪 Latihan 8.4 — Override untuk Development

Buat `docker-compose.override.yml` untuk development:
```yaml
# File ini otomatis digabung dengan docker-compose.yml saat `docker compose up`
services:
  app:
    environment:
      GIN_MODE: debug    # tampilkan debug logs dari Gin
    volumes:
      # Mount source code langsung — tapi binary tetap dari image
      - ./prompts:/app/prompts-local  # untuk override prompt jika tidak embed
```

---

## ❓ Pertanyaan Refleksi

1. Mengapa layer `COPY go.mod go.sum ./` dan `RUN go mod download` **terpisah** dari `COPY . .`?
   (Hint: Docker layer caching)

2. Apa yang terjadi dengan data di `clinic.db` jika container dihapus (`docker compose down -v`)?
   Flag `-v` artinya apa?

3. Mengapa image runtime menggunakan Alpine (`alpine:3.21`) dan bukan Ubuntu atau Debian?

4. Jika kamu mengubah `prompts/system.md`, apakah perlu rebuild image?
   Mengapa ya/tidak?

5. Kenapa user `app` (non-root) dibuat di Dockerfile? Apa risikonya jika container berjalan sebagai root?

---

## ✅ Checklist Selesai!

- [ ] Berhasil build image dengan `docker compose build` atau `podman compose build`
- [ ] Berhasil menjalankan container dan akses dashboard
- [ ] Verifikasi data persisten setelah container restart
- [ ] Mengerti perbedaan multi-stage build
- [ ] Memahami kenapa `CGO_ENABLED=0` + Alpine bekerja bersama

---

## 🎯 Kamu Sudah Menyelesaikan Semua Modul!

Sekarang kamu memahami seluruh sistem dari ujung ke ujung:

```
Config → DB → Tools → Memory → Agent → Buffer → Handler → Container
  M1      M2     M3      M4      M5       M6       M7         M8
```

---

## 🏆 Proyek Akhir — Pilih Tantangan

### Level 1 — Mudah (1-2 jam)
Tambahkan endpoint `GET /dashboard/api/treatments` yang mengembalikan semua treatment, dan `POST /dashboard/api/treatments` untuk menambah treatment baru via API (tanpa restart server).

### Level 2 — Menengah (4-6 jam)
Implementasikan **reminder booking** — cron job yang mengirim pesan WhatsApp H-1 sebelum jadwal booking:
```go
// Setiap jam 09:00, cari booking besok yang statusnya confirmed
// Kirim pesan reminder via whatsapp.SendText()
```

### Level 3 — Lanjut (1-2 hari)
**Migrasi ke PostgreSQL** tanpa ubah kode bisnis — ganti `DATABASE_URL` ke Postgres dan tambahkan service `postgres` di `docker-compose.yml`.

Hint: Kode sudah mendukung Postgres di `db.go`, tapi perlu:
- Update docker-compose.yml dengan service postgres
- Handle connection pooling berbeda dari SQLite
- Review apakah ada query SQLite-specific yang perlu disesuaikan
