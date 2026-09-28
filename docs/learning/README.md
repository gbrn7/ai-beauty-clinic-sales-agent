# 📚 Modul Belajar — AI Sales Agent Workshop Go

> Project ini adalah implementasi AI Sales Agent berbasis Go (Gin + GORM + Gemini AI + WhatsApp).  
> Modul ini dirancang agar kamu bisa memahami setiap bagian sistem secara bertahap.

---

## 🗺️ Urutan Belajar

```
[M1] Fondasi & Config
      ↓
[M2] Database & Model
      ↓
[M3] AI Tools (Logika Bisnis)
      ↓
[M4] Memory (Histori & Takeover)
      ↓
[M5] Gemini Agent Loop
      ↓
[M6] WhatsApp & Debounce Buffer
      ↓
[M7] HTTP Handler & Dashboard
      ↓
[M8] Containerisasi Docker/Podman
```

---

## 📂 Daftar Modul

| # | File | Topik | Estimasi |
|---|------|-------|----------|
| 1 | [01-config-dan-entrypoint.md](./01-config-dan-entrypoint.md) | Fondasi, config, cmd/server & cmd/cli | 30 menit |
| 2 | [02-database-gorm.md](./02-database-gorm.md) | GORM models, AutoMigrate, SQLite WAL, seeding | 45 menit |
| 3 | [03-ai-tools.md](./03-ai-tools.md) | 6 tool klinik, booking overlap, mutex | 60 menit |
| 4 | [04-memory.md](./04-memory.md) | Sliding window, TTL sesi, human takeover | 30 menit |
| 5 | [05-gemini-agent.md](./05-gemini-agent.md) | Agentic loop, tool calling, system prompt | 60 menit |
| 6 | [06-whatsapp-buffer.md](./06-whatsapp-buffer.md) | Debounce, bubble split, concurrency | 45 menit |
| 7 | [07-handler-dashboard.md](./07-handler-dashboard.md) | Gin routes, dashboard admin, storage | 45 menit |
| 8 | [08-containerisasi.md](./08-containerisasi.md) | Dockerfile, docker-compose, Podman | 30 menit |

---

## 🚀 Setup Sebelum Mulai

```bash
# 1. Salin env
cp .env.example .env

# 2. Isi minimal ini di .env:
#    GEMINI_API_KEY=your_key_here
#    AI_NAME=Sari
#    COMPANY_NAME=Klinik Cantik

# 3. Coba jalankan CLI
go run ./cmd/cli

# 4. Jalankan semua test
go test -v ./...
```

---

## 🏗️ Arsitektur Sistem (Ringkas)

```
WhatsApp User
     ↓ pesan
Evolution API ──POST /webhook──► Gin Handler
                                      ↓
                              DebounceBuffer.Add()
                                      ↓ (setelah X detik diam)
                              processBufferedMessages()
                                 ↙            ↘
                          Memory             Agent.RunAgent()
                       (history)          ↙            ↘
                                    Gemini AI      Tools (DB)
                                         ↓
                              WhatsApp.SendReply()
                                     ↓
                              WhatsApp User ← balasan
```

---

## 📝 Catatan Penting

- Semua test ada di file `*_test.go` di setiap package
- Jalankan dengan `-race` untuk deteksi race condition: `go test -race ./...`
- Gunakan `go run ./cmd/cli` untuk testing tanpa perlu WhatsApp
