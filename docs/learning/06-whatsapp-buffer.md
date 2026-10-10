# Modul 6 — WhatsApp Integration & Debounce Buffer

> **Estimasi:** 45 menit  
> **Prasyarat:** Modul 5 selesai

---

## Tujuan Modul

- Memahami cara pesan WhatsApp masuk diparse dan difilter
- Menguasai konsep debounce (bundling pesan cepat)
- Memahami desain dua-level lock untuk concurrency
- Mengerti cara bubble splitting bekerja pada response AI
- Memahami auto-handover berdasarkan marker `[HANDOVER]`

---

## 6.1 Alur Pesan WhatsApp Masuk

```
Customer kirim pesan di WhatsApp
         ↓
Evolution API (proxy WhatsApp)
         ↓ POST /webhook (JSON payload)
Gin Handler (handler.go)
         ↓ ExtractIncoming()
     Filter pesan:
     ✅ Teks biasa
     ✅ Gambar + caption
     ❌ Pesan dari bot sendiri (fromMe=true)
     ❌ Pesan grup (@g.us)
     ❌ Video, dokumen, dll
         ↓ buffer.Add(msg)
DebounceBuffer
         ↓ (setelah X detik diam)
processBufferedMessages()
         ↓
    AI Processing → Balas ke WhatsApp
```

---

## 6.2 Parser — Memfilter Pesan

Buka file: `internal/whatsapp/parser.go`

```go
func ExtractIncoming(payload map[string]any) *IncomingMessage {
    data, _ := payload["data"].(map[string]any)
    key, _ := data["key"].(map[string]any)

    // Filter 1: Abaikan pesan dari bot sendiri
    if fromMe, _ := key["fromMe"].(bool); fromMe {
        return nil
    }

    // Filter 2: Abaikan pesan grup
    remoteJid, _ := key["remoteJid"].(string)
    if strings.HasSuffix(remoteJid, "@g.us") {
        return nil
    }

    // Filter 3: Hanya proses tipe yang didukung
    msgType, _ := data["messageType"].(string)
    switch msgType {
    case "conversation":
        // Teks biasa
        text = data["message"].(map...)["conversation"].(string)
    case "imageMessage":
        // Gambar dengan caption opsional
        imgMsg = data["message"].(map...)["imageMessage"].(map...)
        text = imgMsg["caption"].(string)  // caption bisa kosong
        mediaType = "image"
    default:
        return nil  // tipe lain diabaikan
    }

    return &IncomingMessage{Phone: phone, Text: text, MediaType: mediaType, ...}
}
```

---

## 6.3 Debounce Buffer — Bundling Pesan Cepat

### Masalah: Pesan Multi-Bubble

Customer WhatsApp sering mengirim pesan terputah-putah:
```
"Halo kak"          [enter]
"Saya mau tanya"    [enter] ← 2 detik kemudian
"Treatment apa yang ada?"   ← 1 detik kemudian
```

Tanpa debounce: AI dipanggil 3x, jawab 3x, tidak natural.  
Dengan debounce: tunggu X detik diam, bundel jadi 1 pesan, panggil AI 1x.

### Cara Kerja Debounce

```
t=0s    "Halo kak"        → Add() → timer reset (8 detik)
t=2s    "Saya mau tanya"  → Add() → timer reset (8 detik lagi)
t=3s    "Treatment apa?"  → Add() → timer reset (8 detik lagi)
t=11s   [Timer fire!]     → processBufferedMessages()
                              combined: "Halo kak\nSaya mau tanya\nTreatment apa?"
                              → panggil AI 1x
```

```go
func (db *DebounceBuffer) Add(msg *IncomingMessage) int {
    db.mapMutex.Lock()
    
    // Tambah ke buffer
    db.buffers[msg.Phone] = append(db.buffers[msg.Phone], msg)
    count := len(db.buffers[msg.Phone])
    
    // Reset timer — stop yang lama, buat yang baru
    if t, exists := db.timers[msg.Phone]; exists {
        t.Stop()
    }
    db.timers[phone] = time.AfterFunc(duration, func() {
        db.processBufferedMessages(phone)
    })
    
    db.mapMutex.Unlock()
    return count
}
```

---

## 6.4 Dua Level Concurrency Lock

Di dalam arsitektur konkurensi Go pada sistem ini, terdapat **dua level mutex lock** yang bekerja secara harmonis:

```
Level 1: mapMutex (sync.Mutex)
  - Yang di-lock: Variabel map Go di memori RAM (db.buffers, db.timers, db.locks)
  - Durasi lock: Sangat singkat (mikrodetik)
  - Dipakai oleh: Add(), getLock(), Flush(), awal processBufferedMessages()
  - Tujuan: Mencegah fatal crash "concurrent map read and map write" di Go runtime.

Level 2: phoneMutex (locks[phone] — per nomor WhatsApp)
  - Yang di-lock: Alur eksekusi AI dan pengiriman balasan untuk satu nomor HP tertentu
  - Durasi lock: 3-15 detik (selama Gemini AI berpikir, memanggil tool, dan mengirim WhatsApp)
  - Dipakai oleh: processBufferedMessages()
  - Tujuan: Menjamin hanya ada 1 agen AI yang berjalan per customer pada satu waktu.
```

*(Catatan: Selain kedua lock ini, ada juga `bookingMutex` di `internal/tools/tools.go` yang bertugas mengunci operasi booking slot klinik agar tidak terjadi overbooking / double-booking antar customer).*

```go
func (db *DebounceBuffer) processBufferedMessages(phone string) {
    // 1. Ambil gembok khusus nomor HP ini
    lock := db.getLock(phone)
    lock.Lock()         // 🛑 Menunggu antrean jika nomor ini sedang diproses
    defer lock.Unlock() // 🔓 Buka kunci setelah balasan selesai dikirim

    // 2. Ambil semua pesan yang sudah di-buffer (kunci mapMutex mikrodetik)
    db.mapMutex.Lock()
    pending := db.buffers[phone]
    delete(db.buffers, phone)
    delete(db.timers, phone)
    db.mapMutex.Unlock()

    if len(pending) == 0 {
        return
    }

    // 3. Proses pemanggilan Gemini AI dan kirim ke WhatsApp (bisa beberapa detik)
    ...
}
```

---

### 🔍 Apa yang Terjadi Ketika Ada 2 Pesan Masuk dari Customer yang Sama?

Alur penanganannya terbagi menjadi **2 skenario**, tergantung **kapan** pesan kedua tiba:

#### Skenario A: Pesan ke-2 Masuk Cepat (Masih dalam rentang 8 detik debounce)
*Contoh:*
- Detik 00: Customer kirim *"Halo kak"*
- Detik 03: Customer kirim lagi *"Mau tanya harga treatment facial"*

**Alur sistem:**
1. Pesan ke-2 masuk ke `Add(msg)`.
2. Pesan ke-2 dimasukkan ke array `db.buffers[phone]`.
3. Timer 8 detik sebelumnya dihentikan (`t.Stop()`) lalu di-reset kembali 8 detik dari awal.
4. **Hasilnya:** AI **belum berjalan sama sekali**. Kedua pesan dibundel menjadi satu: `"Halo kak\nMau tanya harga treatment facial"`, dan AI hanya dipanggil **1 kali**. Sangat hemat token dan AI langsung memahami konteks lengkap.

#### Skenario B: Pesan ke-2 Masuk Saat AI SEDANG MEMPROSES Pesan ke-1 (Sedang di-Lock)
*Contoh:*
- Detik 00: Customer kirim pesan 1.
- Detik 08: Timer habis. Goroutine 1 memanggil `processBufferedMessages()` dan mengeksekusi `lock.Lock()`. Gembok nomor HP terkunci. AI mulai berpikir.
- Detik 10: Saat AI masih mengetik/berpikir, customer mengirim pesan ke-2 *"Oh ya dokternya ada siapa aja?"*.

**Alur sistem:**
1. Pesan ke-2 tetap diterima oleh server di `Add(msg)` dan ditampung ke `db.buffers[phone]` yang baru, dengan timer 8 detik baru.
2. Ketika timer pesan ke-2 habis, Goroutine 2 memanggil `processBufferedMessages()` dan mencoba mengeksekusi `lock.Lock()`.
3. **Pengecekan Lock:** Karena gembok nomor HP tersebut masih dipegang oleh Goroutine 1, Goroutine 2 **otomatis tertahan mengantre di `lock.Lock()`**.
4. Begitu Goroutine 1 selesai mengirimkan balasan pesan pertama dan menjalankan `defer lock.Unlock()`:
   - Gembok nomor HP terbuka.
   - Goroutine 2 langsung jalan memproses pesan ke-2.
   - AI untuk pesan ke-2 membaca riwayat chat database yang **sudah lengkap ter-update** dengan balasan pesan ke-1.

**Manfaat Utama:**
- Tidak ada balasan AI yang tumpang tindih (*race condition*).
- Pesan customer selalu dibalas berurutan dan nyambung secara konteks.

---

## 6.5 Bubble Splitting — Memecah Response AI

Response AI bisa mengandung tag khusus:

| Tag | Fungsi |
|-----|--------|
| `[NEXT]` | Pisahkan menjadi pesan baru |
| `[WAIT]` | Hapus (placeholder "sedang mengetik") |
| `[HANDOVER]` | Hapus dari pesan, trigger takeover ke admin |

```go
// internal/whatsapp/parser.go
func SplitBubbles(reply string) []string {
    // Hapus tag [WAIT] dan [HANDOVER]
    clean := strings.ReplaceAll(reply, "[WAIT]", "")
    clean = strings.ReplaceAll(clean, "[HANDOVER]", "")

    // Split berdasarkan [NEXT] atau double newline
    parts := regexp.MustCompile(`\[NEXT\]|\n\n`).Split(clean, -1)

    // Filter bagian kosong
    var bubbles []string
    for _, p := range parts {
        if t := strings.TrimSpace(p); t != "" {
            bubbles = append(bubbles, t)
        }
    }
    return bubbles
}
```

**Contoh:**
```
Input:  "Booking berhasil kak! 😊[NEXT]Kode: BK-123[NEXT]Ada lagi?[WAIT]"
Output: ["Booking berhasil kak! 😊", "Kode: BK-123", "Ada lagi?"]
```

**Pengiriman dengan delay proporsional:**
```go
func (wa *WhatsAppClient) SendReply(phone, reply string) {
    bubbles := SplitBubbles(reply)
    for _, bubble := range bubbles {
        wa.SendTyping(phone, time.Duration(len(bubble)/10)*time.Second) // simulasi mengetik
        wa.SendText(phone, bubble)
    }
}
```

---

## 6.6 Auto-Handover

Jika response AI mengandung `[HANDOVER]`, AI dimatikan untuk nomor tersebut:

```go
// Di processBufferedMessages(), setelah AI menjawab:
if strings.Contains(reply, "[HANDOVER]") {
    _ = db.mem.SetAIEnabled(phone, false)
    log.Printf("[handover] AI dinonaktifkan untuk %s.", phone)
}
```

---

## 🧪 Latihan 6.1 — Test Parsing & Splitting

```bash
go test -v ./internal/whatsapp/...
```

Perhatikan test cases untuk `ExtractIncoming` dan `SplitBubbles`.

## 🧪 Latihan 6.2 — Test Buffer

```bash
go test -v ./internal/buffer/...
```

Baca `buffer_test.go` — perhatikan penggunaan mock (interface) untuk mensimulasikan AI dan WhatsApp tanpa koneksi nyata.

## 🧪 Latihan 6.3 — Simulasi Debounce

Ubah `BufferSeconds = 3` di `.env`, jalankan server, kirim 3 pesan dengan jeda < 3 detik.  
Observasi dari log server berapa kali AI dipanggil.

## 🧪 Latihan 6.4 — Tambah Tag Baru

Tambahkan tag `[SLOW]` yang menyebabkan delay ekstra 3 detik sebelum mengirim bubble berikutnya:

```go
// Di SplitBubbles atau SendReply — detect [SLOW] dan tambah sleep
```

---

## ❓ Pertanyaan Refleksi

1. Mengapa `Add()` harus **non-blocking** (cepat, tidak menunggu)?
   Apa yang terjadi di sisi Evolution API jika webhook lambat merespons?

2. Jika customer A dan customer B kirim pesan bersamaan — apakah mereka akan saling tunggu?
   (Hint: lihat lock adalah per nomor HP, bukan global)

3. Mengapa tag `[HANDOVER]` dihapus dari teks sebelum dikirim ke customer?
   Tapi tetap dicek dengan `strings.Contains(reply, "[HANDOVER]")`?

4. Apa yang terjadi jika `BufferSeconds = 0`?
   Lihat kode — ada guard untuk ini?

5. Mengapa `processBufferedMessages` mengambil seluruh buffer sekaligus (`pending := db.buffers[phone]; delete(...)`)
   bukan mengambil satu-satu?

---

## ✅ Checklist Sebelum Lanjut

- [ ] Test `./internal/whatsapp/...` dan `./internal/buffer/...` PASS
- [ ] Bisa menjelaskan debounce dengan analogi kata-kata sendiri
- [ ] Mengerti perbedaan `mapMutex` vs `phoneMutex`
- [ ] Memahami flow dari customer kirim WhatsApp sampai AI menjawab
- [ ] Tahu apa arti setiap tag: `[NEXT]`, `[WAIT]`, `[HANDOVER]`

**Next:** [Modul 7 — HTTP Handler & Dashboard](./07-handler-dashboard.md)
