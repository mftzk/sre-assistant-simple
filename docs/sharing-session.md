# Sharing session: SRE Assistant Simple

Gunakan container yang sudah ada di server `SSH_HOST` dan terdaftar dalam
`DOCKER_ALLOWED_CONTAINERS`. Ganti `demo-app` di contoh dengan nama container itu.

1. Buka terminal bot agar peserta melihat trace `llm_round` dan `tool`.
2. User biasa: `@sre-assistant-simple cek log demo-app, jelaskan error jika ada`.
   Telusuri schema di `internal/agent/tools.go`, routing di
   `internal/docker/tools.go`, lalu function `Log` di `internal/docker/log.go`.
   Tunjukkan hasil tool yang masuk ke LLM dan jawaban berbasis log.
3. User biasa: `@sre-assistant-simple restart demo-app` → ditolak, tanpa SSH restart.
4. User SRE: mention bot untuk cek log terlebih dahulu. Dalam thread yang sama,
   kirim `@sre-assistant-simple restart container tadi` → restart langsung.
   Buka `internal/docker/restart.go` untuk menunjukkan cek SRE sebelum command SSH.
5. Dalam thread itu: `@sre-assistant-simple cek log demo-app lagi` → periksa log
   setelah restart di server SSH. Keberhasilan restart sendiri belum membuktikan
   masalah aplikasi sudah diperbaiki.
6. Coba container di luar allowlist → ditolak sebelum SSH.

Setiap follow-up harus mention bot. Konteks terpisah per channel/thread/**user**;
user SRE tidak mewarisi percakapan user biasa di thread yang sama.

## Urutan baca kode

Baca dari luar ke dalam, satu konsep per file:

1. `internal/app/app.go` — wiring: config → SSH executor → Docker tools →
   harness (LLM + tools) → Slack bot. Gambaran besar dalam satu layar.
2. `internal/agent/llm.go` — "protokol" ke model: daftar message masuk, satu
   message keluar (teks atau `tool_calls`).
3. `internal/agent/tools.go` — schema tool: cara model tahu tool apa yang ada.
   Restart hanya ditawarkan saat `canRestart` true.
4. `internal/agent/harness.go` — inti sesi. `Run` punya empat langkah berlabel
   `Step 1..4`: tentukan izin, susun percakapan, cek jawaban final, jalankan
   tool lalu kirim hasil sebagai message `tool`. Batas loop di konstanta atas file.
5. `internal/agent/prompt.go` — system prompt. Poin diskusi: aturan di prompt
   hanya permintaan; aturan penting dicek ulang di kode.
6. `internal/authz/authz.go` — gate izin dari user ID Slack asli + kata
   restart eksplisit, bukan dari keputusan LLM.
7. `internal/docker/tools.go`, `log.go`, `restart.go` — dispatcher `switch`
   lalu function Go biasa. Hasil selalu `{"ok":...,"output":...,"error":...}`.
8. `internal/docker/ssh.go` — eksekusi nyata: host key wajib cocok, output
   dipotong 32 KiB sebelum masuk LLM.
9. `internal/slackbot/` — antarmuka: `receiveMentions` (filter + dedup) →
   antrean → `processMentions`/`handleMention` → balasan thread; `session.go`
   menyimpan memory di RAM.

Semua batas (jumlah putaran, timeout, ukuran output, TTL session) berupa
konstanta bernama di atas file masing-masing, jadi gampang ditunjuk dan diubah
saat demo.
