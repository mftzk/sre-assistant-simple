# Sharing session: SRE Assistant Simple

1. Buka terminal bot agar peserta melihat trace `llm_round` dan `tool`.
2. User biasa: `@sre-assistant-simple cek log demo-app, jelaskan warning-nya`.
   Telusuri schema dan tool arguments di kode harness, hasil tool yang masuk ke
   LLM, lalu jawaban berbasis log.
3. User biasa: `@sre-assistant-simple restart demo-app` → ditolak, tanpa SSH restart.
4. User SRE: mention bot untuk cek log terlebih dahulu. Dalam thread yang sama,
   kirim `@sre-assistant-simple restart container tadi` → restart langsung.
5. Dalam thread itu: `@sre-assistant-simple cek log demo-app lagi` → timestamp
   startup baru terlihat. Warning contoh sengaja tetap muncul; restart bukan
   bukti bahwa masalah aplikasi sudah diperbaiki.
6. Coba container di luar allowlist → ditolak sebelum SSH.

Setiap follow-up harus mention bot. Konteks terpisah per channel/thread/**user**;
user SRE tidak mewarisi percakapan user biasa di thread yang sama.
