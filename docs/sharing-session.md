# Sharing session: SRE Assistant Simple

Gunakan container yang sudah ada di server `SSH_HOST` dan terdaftar dalam
`DOCKER_ALLOWED_CONTAINERS`, atau jalankan target demo di server itu:

```bash
git clone https://github.com/mftzk/sre-assistant-simple.git && cd sre-assistant-simple
docker compose -f deploy/compose.demo.yaml up -d --build
```

Isinya tiga service HTTP kecil dari satu binary `cmd/demo` (pilih perilaku
lewat env `MODE`), plus `demo-loadgen` yang mengirim request terus-menerus.
Datanya palsu dan aman dikirim ke LLM:

- `demo-app` (`MODE=healthy`, port host `18081`): selalu 200.
- `demo-api` (`MODE=degrade`, port `18082`): sehat 60 detik pertama, lalu tiap
  request jadi 503 dengan log `ERROR ... db connection pool exhausted` dan
  `/healthz` ikut 503 sampai di-restart. Skenario utama: baca log → model
  menemukan masalah → SRE restart → cek log lagi, kembali sehat.
- `demo-worker` (`MODE=flaky`, port `18083`): tiap request ke-4 kena
  `WARN upstream smtp timeout` tapi tetap 200. Bahan diskusi: restart bukan
  solusi, model sebaiknya tidak menyarankannya.

Opsional saat demo: `curl http://<server>:18082/healthz` untuk menunjukkan
status sebelum dan sesudah restart.

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
6. User biasa: `@sre-assistant-simple demo-api kenapa ya? jelasin yang terjadi`
   → model memakai `docker_list`/`docker_inspect` (state, exit code, restart
   count) lalu `docker_logs`, dan menjelaskannya dengan bahasa awam. Buka
   `internal/docker/inspect.go`: template `--format` sengaja tidak mengambil Env.
7. Coba container di luar allowlist → ditolak sebelum SSH.

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
7. `internal/docker/tools.go`, `list.go`, `inspect.go`, `log.go`, `restart.go` — dispatcher `switch`
   lalu function Go biasa. Hasil selalu `{"ok":...,"output":...,"error":...}`.
8. `internal/docker/ssh.go` — eksekusi nyata: host key wajib cocok, output
   dipotong 32 KiB sebelum masuk LLM.
9. `internal/slackbot/` — antarmuka: `receiveMentions` (filter + dedup) →
   antrean → `processMentions`/`handleMention` → balasan thread; `session.go`
   menyimpan memory di RAM.

Semua batas (jumlah putaran, timeout, ukuran output, TTL session) berupa
konstanta bernama di atas file masing-masing, jadi gampang ditunjuk dan diubah
saat demo.
