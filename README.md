# SRE Assistant Simple

Contoh kecil untuk belajar **agent harness** lewat sharing session. Bot Slack
punya dua kemampuan: baca log Docker dan restart container di **satu server SSH**.
Loop agent ditulis langsung di Go supaya alurnya bisa dibaca tanpa framework agent.

```text
@mention Slack → session RAM → LLM → tool call → policy → SSH → Docker
                                  ↑                         ↓
                                  └────── hasil tool ────────┘
                         jawaban akhir → thread Slack
```

Semua user yang mention bot boleh baca log container dalam allowlist. Restart
langsung hanya untuk Slack user ID dalam `SRE_SLACK_USER_IDS`; tidak ada approval
tambahan. Authorization diperiksa kode sebelum SSH, bukan berdasarkan keputusan
LLM, nama user, atau teks “saya SRE”.

## Quickstart

Butuh Go 1.26.2+, Slack app, endpoint LLM yang mendukung Chat Completions dan tool
calling, serta server SSH dengan Docker. User SSH harus bisa menjalankan `docker`
langsung. Tidak perlu Docker di mesin bot, public HTTP endpoint, atau database.

1. Buat Slack app dari [deploy/slack-manifest.yaml](deploy/slack-manifest.yaml), install ke
   workspace, dan ambil bot token `xoxb-...`. Buat app-level token `xapp-...`
   dengan scope `connections:write`. Invite bot ke channel demo.
2. Pada server demo, salin [deploy/compose.demo.yaml](deploy/compose.demo.yaml), sebagai `compose.demo.yaml`, lalu jalankan:

   ```bash
   docker compose -f compose.demo.yaml up -d
   docker logs --tail 10 demo-app
   ```

3. Siapkan SSH key khusus demo dan host key yang sudah diverifikasi di
   `~/.ssh/known_hosts`, atau file yang ditunjuk `SSH_KNOWN_HOSTS_FILE`.
   Untuk port nonstandar, entri host memakai bentuk `[host]:port`.
4. Di folder project ini:

   ```bash
   cp .env.example .env
   chmod 600 .env
   ```

   Isi token, gateway/model, host/user SSH, Slack user IDs SRE, dan nama container.
   `SSH_PRIVATE_KEY` adalah **isi PEM multiline**, bukan path atau base64:

   ```bash
   SSH_PRIVATE_KEY='-----BEGIN OPENSSH PRIVATE KEY-----
   ...isi key khusus demo...
   -----END OPENSSH PRIVATE KEY-----'
   ```

   Alternatif tanpa menaruh key dalam `.env`: hapus assignment key dari `.env`,
   lalu export isi key secara lokal setelah memuat environment:

   ```bash
   set -a
   source .env
   set +a
   export SSH_PRIVATE_KEY="$(cat /path/to/demo-key)"
   go run ./cmd/sre-assistant
   ```

   Key tanpa passphrase digunakan untuk contoh ini. Jangan commit `.env` atau key.
   Port default `22`; base URL LLM menerima suffix `/v1` maupun tanpa suffix itu.
   Model ditentukan lewat `LLM_MODEL`, tidak ada model bawaan.

Scope bot: `app_mentions:read` dan `chat:write`. Socket Mode memakai app token
`connections:write`; lihat [panduan resmi Slack](https://docs.slack.dev/apis/events-api/using-socket-mode/).

## Sharing session

Ikuti [walkthrough demo](docs/sharing-session.md) untuk melihat tool loop,
penolakan restart non-SRE, dan follow-up dalam thread.

## Membaca harness

```text
cmd/sre-assistant/   entrypoint CLI
internal/
  app/              wiring dan shutdown
  config/           environment
  authz/            izin SRE, intent restart, validasi nama container
  agent/            LLM client, schema tool, loop harness
  docker/           dispatcher tool dan SSH transport
  slackbot/         event Slack, balasan, memory session
deploy/             manifest Slack dan Compose demo
docs/               panduan sharing session
```

Mulai dari `internal/agent/harness.go` untuk membaca loop. Cek
`internal/authz/authz.go` untuk authorization: harness memakai gate yang sama
untuk memilih tool, dispatcher Docker mengeceknya lagi sebelum SSH.
Tool Docker didefinisikan sebagai function biasa: `Log` di
[internal/docker/log.go](internal/docker/log.go) dan `Restart` di
[internal/docker/restart.go](internal/docker/restart.go). Masing-masing berisi
validasi dan command Docker-nya sendiri. `Execute` di `internal/docker/tools.go`
membaca JSON arguments, memilih function lewat `switch`, dan mengemas hasilnya.
Schema LLM ditulis eksplisit di `internal/agent/tools.go`; tidak ada factory schema.
Tests berada di samping package yang diuji. Root `main.go` adalah shim kecil
agar command lama `go run .` tetap bekerja.

Kontrak tool:

```json
{"name":"docker_logs","arguments":{"container":"demo-app","tail":100}}
{"name":"docker_restart","arguments":{"container":"demo-app"}}
```

Hasil tool berbentuk `{"ok":true,"output":"..."}` atau
`{"ok":false,"output":"...","error":"..."}`. Harness mengembalikannya sebagai
message ber-role `tool`, dengan `tool_call_id` yang sesuai. Host/user/key SSH
berasal dari environment dan tidak tersedia sebagai argumen model.

Tool restart hanya ditawarkan pada turn user SRE dengan kata eksplisit `restart`,
`reboot`, atau `restartkan`; pesan dengan `jangan`, `tanpa`, `bukan`, `never`,
`without`, `don't`, atau `do not` tidak membuka tool. Ini gate konservatif untuk
demo, bukan parser bahasa alami umum. Backend tetap mengecek izin walaupun model
mengirim tool yang tidak ditawarkan. Isi log tidak dapat membuka izin restart.

Default batas operasi:

- Hanya exact container name dalam `DOCKER_ALLOWED_CONTAINERS`; daftar kosong
  menolak semua. `SRE_SLACK_USER_IDS` kosong menolak semua restart.
- Log default 100 baris, maksimum 200 dan 32 KiB. Tidak ada follow log streaming.
- SSH/tool timeout 30 detik; handshake maksimum 10 detik. Host key wajib cocok.
- Maksimum enam putaran tool, empat call per putaran, satu percobaan restart
  per container per pesan. Error/timeout tidak menyebabkan retry restart.
- Turn maksimum tiga menit. Satu worker menjalankan request berurutan, antrean
  maksimum 32, event duplikat diabaikan selama sepuluh menit.
- Memory maksimal sepuluh turn lengkap per session, expiry 30 menit dan maksimal
  100 session. Hilang saat proses bot restart; tidak mengambil history dari Slack.

Saat SSH putus atau timeout, restart mungkin sudah terjadi. Cek log sebelum
mengirim permintaan restart baru. Deduplikasi berada di RAM, jadi ini bukan
jaminan exactly-once lintas restart proses bot.

Log demo dikirim ke endpoint LLM dan sebagian dapat dikutip di Slack. Gunakan
container demo yang datanya memang boleh dibagikan. Trace terminal tidak mencetak
token, private key, prompt, atau isi log. Tidak ada tool shell bebas, Docker exec,
discovery host, deployment, scheduler, maupun integrasi ke assistant lama.

## Verifikasi

```bash
go test -race ./...
go vet ./...
go build -o bin/sre-assistant ./cmd/sre-assistant
```

Test memakai HTTP LLM palsu, fake executor, dan server SSH loopback dengan key
sementara. Tidak perlu credential Slack/LLM, Docker, atau akses server sungguhan.

Hentikan demo dari server SSH:

```bash
docker compose -f compose.demo.yaml down
```
