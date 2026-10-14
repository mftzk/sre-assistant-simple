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
