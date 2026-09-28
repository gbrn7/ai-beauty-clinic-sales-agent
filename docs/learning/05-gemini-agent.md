# Modul 5 — Gemini Agent: Loop, Tool Calling, dan System Prompt

> **Estimasi:** 60 menit  
> **Prasyarat:** Modul 3 dan 4 selesai

---

## Tujuan Modul

- Memahami "agentic loop" — bagaimana AI memanggil tools berulang kali
- Mengerti cara context (tanggal, nomor HP) diinjeksikan ke setiap pesan
- Memahami cara system prompt ditulis dan dirender
- Memahami multimodal input (gambar dari WhatsApp)
- Mengerti mekanisme fallback saat AI gagal

---

## 5.1 Apa itu Agentic Loop?

AI bukan hanya menjawab pertanyaan — ia **merencanakan** dan **mengeksekusi** serangkaian tool call untuk menyelesaikan tugas.

**Contoh skenario booking:**
```
Customer: "Saya mau booking facial besok jam 10 atas nama Budi"

[Iterasi 1]
  Gemini berpikir: "Saya perlu cek apakah besok ada slot jam 10"
  → Memanggil: check_available_schedule(booking_date="2026-09-29")
  ← Hasil: {available_slots: [{start:"10:00", remaining_capacity: 2}]}

[Iterasi 2]  
  Gemini berpikir: "Slot tersedia, sekarang buat booking"
  → Memanggil: book_treatment(
        customer_name="Budi",
        phone="628xxx",
        treatment_name="facial",
        booking_date="2026-09-29",
        booking_time="10:00"
    )
  ← Hasil: {success: true, booking_code: "BK-ABC123", ...}

[Iterasi 3]
  Gemini berpikir: "Booking berhasil, balas dengan konfirmasi"
  → TIDAK memanggil tool
  ← Text: "Booking berhasil kak! 😊 Kode booking: BK-ABC123..."
  → SELESAI
```

---

## 5.2 Implementasi Loop di agent.go

Buka file: `internal/agent/agent.go`

```go
func (a *Agent) RunAgent(ctx context.Context, history []*genai.Content,
    userMessage, phone string, images []ImageData) (string, []*genai.Content, error) {

    // 1. Bungkus pesan user dengan konteks (tanggal + nomor HP)
    contents := buildContents(history, userMessage, phone, images)

    // 2. Loop maksimal MaxToolIterations=5 kali
    for iter := 0; iter < MaxToolIterations; iter++ {

        // 3. Kirim ke Gemini API
        resp, err := a.client.Models.GenerateContent(ctx, a.model, contents, a.genConfig)
        if err != nil {
            return FallbackMessage, contents, err  // error → fallback
        }

        // 4. Tambah response AI ke riwayat conversation
        contents = append(contents, resp.Candidates[0].Content)

        // 5. Cek apakah AI ingin memanggil tool
        functionCalls := resp.FunctionCalls()
        if len(functionCalls) == 0 {
            // Tidak ada tool call → ini jawaban final
            replyText, _ := resp.Text()
            return replyText, contents, nil
        }

        // 6. Eksekusi setiap tool yang diminta AI
        var resultParts []*genai.Part
        for _, fc := range functionCalls {
            result := a.tools.ExecuteTool(fc.Name, fc.Args)
            resultParts = append(resultParts, &genai.Part{
                FunctionResponse: &genai.FunctionResponse{
                    Name:     fc.Name,
                    Response: map[string]any{"result": result},
                },
            })
        }

        // 7. Masukkan hasil tool ke contents, lanjut iterasi berikutnya
        contents = append(contents, &genai.Content{
            Role:  "user",
            Parts: resultParts,
        })
    }

    // Iterasi habis tanpa jawaban final → fallback
    return FallbackMessage, contents, nil
}
```

### Visualisasi Contents (Riwayat Percakapan)

```
contents = [
    // history dari database (sliding window)
    {role: "user",  parts: ["Halo"]}
    {role: "model", parts: ["Halo kak!"]}
    
    // pesan baru + konteks
    {role: "user",  parts: ["[Konteks: Senin, 28 Sep 2026. WA: 628xxx]\nMau booking facial"]}
    
    // [setelah iterasi 1]
    {role: "model", parts: [FunctionCall{check_available_schedule}]}
    {role: "user",  parts: [FunctionResponse{slots: [...]}]}
    
    // [setelah iterasi 2]
    {role: "model", parts: [FunctionCall{book_treatment}]}
    {role: "user",  parts: [FunctionResponse{success: true, ...}]}
    
    // [iterasi 3 — jawaban final]
    {role: "model", parts: ["Booking berhasil kak! 😊"]}
]
```

---

## 5.3 Context Injection

AI perlu tahu **tanggal hari ini** (untuk booking) dan **nomor HP customer** (untuk menyimpan data).

```go
// Informasi ini diinjeksikan ke setiap pesan user:
dateCtx := fmt.Sprintf("Hari ini %s, %d %s %d (%s), pukul %02d:%02d",
    HariNama[weekday],   // "Senin"
    now.Day(),           // 28
    BulanNama[...],      // "September"
    now.Year(),          // 2026
    now.Format("2006-01-02"),  // "2026-09-28"
    now.Hour(), now.Minute(),  // 23, 10
)

parts := []*genai.Part{
    {Text: fmt.Sprintf("[Konteks: %s. Nomor WhatsApp customer: %s]\n%s",
        dateCtx, phone, userMessage)},
}
```

Hasilnya, setiap pesan yang diterima Gemini seperti:
```
[Konteks: Hari ini Senin, 28 September 2026 (2026-09-28), pukul 23:10.
Nomor WhatsApp customer: 6281234567890]
Halo kak, saya mau booking facial besok
```

---

## 5.4 System Prompt — Instruksi AI

Buka file: `prompts/system.md`

System prompt adalah "manual kerja" untuk AI. Berisi:

```markdown
# Bagian-bagian system prompt:

1. PERSONA          → siapa AI ini, cara bicara, emoji yang dipakai
2. TOOLS USAGE      → kapan dan bagaimana memanggil setiap tool
3. BOOKING WORKFLOW → langkah-langkah proses booking yang benar
4. EDGE CASES       → skenario khusus (diskon dokter, customer komplain)
5. HANDOVER RULES   → kapan harus alihkan ke admin manusia
6. KNOWLEDGE BASE   → info klinik (lokasi, no telp, dll)
```

**Cara prompt dirender:**
```go
// internal/agent/prompt.go
func RenderSystemPrompt(promptPath, aiName, companyName string) (string, error) {
    content, _ := fs.ReadFile(FS, "system.md")  // baca dari embedded FS
    result := string(content)
    result = strings.ReplaceAll(result, "{{AI_NAME}}", aiName)
    result = strings.ReplaceAll(result, "{{COMPANY_NAME}}", companyName)
    return result, nil
}
```

**Embedded asset dengan go:embed:**
```go
// prompts/prompts.go
//go:embed system.md
var FS embed.FS
```

Binary yang dikompilasi sudah mengandung `system.md` di dalamnya — bisa dijalankan dari direktori mana saja.

---

## 5.5 Multimodal — Gambar dari WhatsApp

Customer bisa mengirim foto kulit mereka, dan AI bisa melihat + menganalisis:

```go
// Gambar dikonversi ke bytes, lalu dikirim ke Gemini bersama teks
for _, img := range images {
    parts = append(parts, &genai.Part{
        InlineData: &genai.Blob{
            MIMEType: img.MIMEType,  // "image/jpeg"
            Data:     img.Bytes,     // raw image bytes
        },
    })
}
```

Contoh percakapan multimodal:
```
Customer: [kirim foto kulit + teks "Kak ini cocok treatment apa ya?"]
AI: [melihat gambar] "Dari foto yang kakak kirim, kulit kakak terlihat
    kering dan ada beberapa flek. Saya rekomendasikan treatment
    Salmon DNA atau Brightening Facial kak 😊..."
```

---

## 5.6 Fallback Message

Ketika API gagal atau iterasi habis:

```go
const FallbackMessage = "Mohon maaf kak, whatsapp kami sedang ada sedikit kendala🙏" +
    "[NEXT]Admin kami akan segera membantu ya kak 😊[HANDOVER]"
```

Perhatikan `[HANDOVER]` di akhir — ini memicu **automatic handover ke admin** bahkan saat error. Customer tidak akan ditinggalkan tanpa respons.

---

## 🧪 Latihan 5.1 — Observasi Loop

Tambahkan logging ke loop di `agent.go`:
```go
for iter := 0; iter < MaxToolIterations; iter++ {
    log.Printf("[agent] iterasi %d dimulai", iter+1)
    // ... kode yang ada
    
    if len(functionCalls) == 0 {
        log.Printf("[agent] iterasi %d: jawaban final", iter+1)
        return ...
    }
    
    for _, fc := range functionCalls {
        log.Printf("[agent] iterasi %d: memanggil tool %s", iter+1, fc.Name)
    }
}
```

Jalankan `go run ./cmd/cli` dan tanya "Booking facial besok jam 10 atas nama Budi" — perhatikan log.

## 🧪 Latihan 5.2 — Batasi Iterasi

Ubah `MaxToolIterations = 1`, jalankan CLI, tanya hal yang butuh 2 tool call.
Apa yang terjadi? Apakah AI masih bisa menjawab dengan benar?

Kembalikan ke `MaxToolIterations = 5` setelah selesai.

## 🧪 Latihan 5.3 — Edit System Prompt

Buka `prompts/system.md`, ubah nama klinik atau tambahkan aturan baru:
```markdown
## ATURAN TAMBAHAN
- Jika customer menanyakan parkir → jawab bahwa tersedia parkir gratis
- Jika customer menanyakan WiFi → jawab bahwa tersedia WiFi gratis
```

Test di CLI apakah AI mengikuti aturan baru.

## 🧪 Latihan 5.4 — Test Multimodal

```bash
# Jalankan CLI — modifikasi cmd/cli/main.go untuk test kirim gambar
# Tambah kode untuk membaca file gambar dari disk:
imageBytes, _ := os.ReadFile("test_image.jpg")
images := []agent.ImageData{{Bytes: imageBytes, MIMEType: "image/jpeg"}}

# Lalu panggil RunAgent dengan images tersebut
```

---

## ❓ Pertanyaan Refleksi

1. Mengapa iterasi dibatasi ke `MaxToolIterations = 5`?
   Apa yang terjadi jika dibiarkan tak terbatas?

2. Apa yang terjadi jika Gemini memanggil tool yang tidak ada di `ExecuteTool` switch case?

3. Mengapa hasil tool dikemas dalam `map[string]any{"result": result}` dan bukan langsung `result`?

4. Jika AI ingin memanggil 2 tool sekaligus dalam satu iterasi — apakah bisa?
   Lihat kode — apakah ada `for _, fc := range functionCalls`?

5. Mengapa context injection (tanggal, nomor HP) dilakukan di Go, bukan di system prompt?
   (Hint: system prompt dibaca sekali saat startup, sedangkan tanggal berubah setiap hari)

---

## ✅ Checklist Sebelum Lanjut

- [ ] Bisa menjelaskan agentic loop dengan kata-kata sendiri
- [ ] Mengerti mengapa context diinjeksikan setiap pesan
- [ ] Berhasil menambah instruksi baru di system prompt dan melihat efeknya
- [ ] Memahami cara embedded asset (`//go:embed`) bekerja
- [ ] Mengerti mengapa fallback message mengandung `[HANDOVER]`

**Next:** [Modul 6 — WhatsApp Buffer](./06-whatsapp-buffer.md)
