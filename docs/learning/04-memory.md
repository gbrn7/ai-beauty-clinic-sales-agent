# Modul 4 — Memory: Sliding Window & Human Takeover

> **Estimasi:** 30 menit  
> **Prasyarat:** Modul 2 selesai

---

## Tujuan Modul

- Memahami cara histori percakapan disimpan dan dibaca
- Mengerti konsep sliding window memory
- Memahami Session TTL (sesi berakhir otomatis)
- Menguasai fitur human takeover (AI on/off per customer)
- Memahami gotcha GORM boolean zero-value

---

## 4.1 Masalah yang Dipecahkan Memory

Tanpa memory, setiap pesan dianggap percakapan baru:
```
User: "Saya mau booking"
AI: "Mau booking apa kak?"
User: "Facial"                 ← AI tidak ingat konteks "mau booking"
AI: "Ada yang bisa saya bantu?" ← SALAH — harusnya tanya tanggal
```

Dengan memory (sliding window):
```
AI menerima: [20 pesan terakhir] + pesan baru
AI tahu konteks → bisa melanjutkan percakapan dengan benar
```

---

## 4.2 Sliding Window — Membaca History

Buka file: `internal/memory/memory.go`

```go
func (m *Memory) GetHistory(phone string) []*genai.Content {
    var rows []model.ConversationMessage

    // Ambil N pesan TERBARU (ORDER DESC), dibatasi window
    m.db.Where("phone = ?", phone).
        Order("created_at DESC, id DESC").
        Limit(m.window).   // default: 20
        Find(&rows)

    // Reverse ke urutan kronologis (lama → baru)
    // Gemini butuh urutan: pesan lama dulu, pesan baru di akhir
    for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
        rows[i], rows[j] = rows[j], rows[i]
    }

    // Konversi ke format Gemini []*genai.Content
    var contents []*genai.Content
    for _, row := range rows {
        contents = append(contents, &genai.Content{
            Role:  row.Role,  // "user" atau "model"
            Parts: []*genai.Part{{Text: row.Text}},
        })
    }
    return contents
}
```

**Ilustrasi Sliding Window (window=5):**
```
Database (10 pesan):  [1][2][3][4][5][6][7][8][9][10]
                                              ↑
                                        window=5 → ambil [6][7][8][9][10]
                                        
Setelah reverse:      [6][7][8][9][10]  ← dikirim ke Gemini
                      (lama → baru = kronologis)
```

---

## 4.3 Session TTL — Sesi Berakhir Otomatis

```go
// Jika pesan terakhir lebih dari TTL jam yang lalu → fresh start
if time.Since(lastMsg.CreatedAt) > time.Duration(m.ttlHours)*time.Hour {
    return []*genai.Content{}  // history kosong = percakapan baru
}
```

**Contoh skenario TTL=8 jam:**
```
Jam 09:00  Customer chat tentang booking facial
Jam 09:30  Selesai booking, percakapan berakhir
...
Jam 18:00  Customer chat lagi (8.5 jam kemudian)
           → history dikosongkan
           → AI mulai fresh, tidak ingat percakapan tadi pagi
           → "Halo kak, ada yang bisa saya bantu?"
```

TTL berguna agar AI tidak bingung dengan konteks percakapan lama yang sudah tidak relevan.

---

## 4.4 Menyimpan Histori

```go
// Dipanggil setiap kali ada pesan masuk atau keluar
func (m *Memory) Add(phone, role, text string) error {
    return m.db.Create(&model.ConversationMessage{
        Phone: phone,
        Role:  role,   // "user" atau "model"
        Text:  text,
    }).Error
}

// Dipanggil di buffer.go setelah AI menjawab:
_ = db.mem.Add(phone, "user", userMessage)
_ = db.mem.Add(phone, "model", aiReply)
```

---

## 4.5 Human Takeover — ai_enabled Flag

Fitur ini memungkinkan admin mengambil alih percakapan dari AI.

```
Contact.AiEnabled = true   → AI membalas otomatis
Contact.AiEnabled = false  → AI diam, admin balas manual
```

**Flow takeover:**
```
1. Customer komplain keras
2. AI panggil tool handover_to_admin()
3. tools.go → set Contact.AiEnabled = false
4. buffer.go: if !mem.IsAIEnabled(phone) → skip AI, simpan pesan saja
5. Admin melihat di dashboard → balas manual
6. Admin toggle AI kembali ON lewat dashboard
```

**Implementasi IsAIEnabled:**
```go
func (m *Memory) IsAIEnabled(phone string) bool {
    var contact model.Contact
    if err := m.db.Where("phone = ?", phone).First(&contact).Error; err != nil {
        return true  // ← default: AI aktif jika contact belum ada
    }
    return contact.AiEnabled
}
```

---

## 4.6 ⚠️ GORM Boolean Zero-Value Gotcha

Ini adalah bug umum yang sering terjadi dengan GORM!

**Masalah:**
```go
// Ingin set ai_enabled = false
contact.AiEnabled = false
db.Save(&contact)  // ❌ TIDAK BEKERJA!
// GORM menganggap false = zero value = "tidak perlu diupdate"
// Kolom ai_enabled tetap true di database!
```

**Penjelasan:**
GORM's `Save()` menggunakan reflection untuk mendeteksi field mana yang "berubah". Dalam Go, zero value untuk bool adalah `false` — jadi GORM menganggap `false` berarti "field ini kosong, skip".

**Solusi:**
```go
// ✅ Cara 1: Update kolom spesifik
db.Model(&contact).Update("ai_enabled", false)

// ✅ Cara 2: Pakai map
db.Model(&contact).Updates(map[string]interface{}{
    "ai_enabled": false,
})

// ✅ Cara 3: Pakai Select untuk memaksa update
db.Select("ai_enabled").Save(&contact)
```

**Implementasi di project ini:**
```go
// memory.go
func (m *Memory) SetAIEnabled(phone string, enabled bool) error {
    var contact model.Contact
    if err := m.db.Where("phone = ?", phone).First(&contact).Error; err == nil {
        return m.db.Model(&contact).Update("ai_enabled", enabled).Error  // ✅
    }
    // Jika belum ada contact → buat baru
    return m.db.Model(&model.Contact{}).Create(map[string]interface{}{
        "phone":      phone,
        "ai_enabled": enabled,
    }).Error  // ✅ create via map
}
```

---

## 🧪 Latihan 4.1 — Jalankan Test Memory

```bash
go test -v ./internal/memory/...
```

Perhatikan test cases:
- `TestGetHistory_SlidingWindow` — verifikasi window
- `TestTakeover` — verifikasi ai_enabled flag
- `TestSessionTTL` — verifikasi TTL reset history

## 🧪 Latihan 4.2 — Simulasi Takeover Manual

```bash
# Di terminal pertama — jalankan server
go run ./cmd/server

# Di terminal kedua — toggle AI off
curl -X POST http://localhost:8000/dashboard/api/conversations/6281234567890/ai \
  -H "X-API-Key: your_dashboard_key" \
  -H "Content-Type: application/json" \
  -d '{"ai_enabled": false}'

# Sekarang kirim pesan dari CLI (simulasi customer)
# AI tidak akan membalas
```

## 🧪 Latihan 4.3 — Ubah Window Size

Di `.env`, tambahkan:
```
MEMORY_WINDOW=3
SESSION_TTL_HOURS=1
```

Kemudian chat panjang lebih dari 3 pesan — observasi apakah AI "lupa" konteks awal.

## 🧪 Latihan 4.4 — Bug Simulation

Coba reproduksi GORM boolean bug:

```go
// Buat file scratch: /tmp/gorm_test.go
// Gunakan SQLite in-memory
// Update contact.AiEnabled = false dengan db.Save(&contact)
// Baca kembali dari DB → verifikasi nilainya
// Lalu coba dengan db.Model(&contact).Update("ai_enabled", false)
// Bandingkan hasilnya
```

---

## ❓ Pertanyaan Refleksi

1. Mengapa history di-sort `DESC` saat dibaca, lalu di-reverse sebelum dikirim ke Gemini?
   Mengapa tidak langsung sort `ASC`?

2. Jika `MemoryWindow = 20` dan customer sudah chat 100 pesan, pesan ke-1 sampai ke-80 hilang?
   Apakah data benar-benar terhapus dari database?

3. Apa dampak positif dan negatif dari TTL yang sangat pendek (mis. 1 jam)?
   Dan TTL yang sangat panjang (mis. 30 hari)?

4. Mengapa default `IsAIEnabled` mengembalikan `true` jika contact belum ada di database?

5. Selain boolean, tipe Go apa lagi yang bisa mengalami zero-value problem di GORM?

---

## ✅ Checklist Sebelum Lanjut

- [ ] Test `./internal/memory/...` semua PASS
- [ ] Mengerti cara sliding window bekerja dengan ilustrasi
- [ ] Memahami GORM boolean zero-value gotcha dan solusinya
- [ ] Berhasil simulasi takeover manual lewat API
- [ ] Bisa menjelaskan kapan TTL berguna

**Next:** [Modul 5 — Gemini Agent](./05-gemini-agent.md)
