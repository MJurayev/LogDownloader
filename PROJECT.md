# LogDownloader

VictoriaLogs uchun web-based log query va export vositasi. LogsQL queryni brauzerda yozasiz, natijani jadval, gistogramma va live tail ko'rinishida ko'rasiz, arxiv (default `.tar.gz`, avtomatik qismlarga bo'lingan) sifatida eksport qilib yuklab olasiz yoki rejali export qilasiz.

Backend Go bilan yozilgan single static binary (React frontend `embed.FS` orqali shu binarni ichiga joylanadi). Front-end React 19 + TypeScript + Vite, interfeys ikki tilda (o'zbek / rus). Storage — disk ustida oddiy JSON fayllar (database yo'q).

---

## Asosiy imkoniyatlar

### Log Viewer
- LogsQL query, vaqt oralig'i (presetlar), `_time` bo'yicha saralash, pagination, level highlight, qator JSON detail + nusxalash.
- **Maydonlar paneli**: `field_names` → maydon bosilsa top qiymatlar → qiymat bosilsa `field:"value"` query'ga qo'shiladi; streamlar.
- **Gistogramma** (`/select/logsql/hits`, avtomatik qadam), ustun bosilsa shu oraliqqa zoom.
- **Live tail** (`/select/logsql/tail`) — real vaqt oqimi, oxirgi 1000 qator.
- **Query tarixi** (har user, 50 ta) va **autocomplete** (maydon nomlari, Tab).
- **Ustunlar**: yashirish/tartiblash, datasource bo'yicha brauzerda saqlanadi.
- **URL'da holat** (`?q=&ds=&from=&to=&sort=`) — havolani ulashish mumkin.
- Export shu sahifadan: fayl nomi, arxiv formati (default `.tar.gz`), "Export sozlamalari" (tarkib, maydonlar, arxiv hajmi, xabar, auto-share).

### Export
- **Navbat**: bir vaqtda `max_concurrent` ta export (admin sozlaydi), qolgani `queued`; restart'dan keyin navbat tiklanadi.
- **Vaqt oynalari**: uzoq oraliq avtomatik soatlik/6 soatlik/kunlik oynalarga bo'linadi (VictoriaLogs'ga yuk kamayadi), har oyna alohida **retry** (3 marta, backoff) va rollback. Ochiq oraliq uchun eng eski log probe qilinadi.
- **Progress va ETA**: `hits` bo'yicha taxminiy jami, foiz, oynalar, o'tgan/qolgan vaqt, qayta urinishlar.
- **Tarkib**: `ndjson` (barcha yoki tanlangan maydonlar), faqat `_msg` matni, `csv` (sarlavha har qism boshida).
- **Bo'laklash — "Avtomatik"**: `.tar.gz` ichida xom hajm `export_chunk_mb` (default 100 MB) dan oshsa `<nom>_partNNN.log|csv` qismlar.
- **Arxiv hajmi chegarasi**: 500 MB / 1 / 2 / 4 GB → `<nom>_vol001.tar.gz`, `_vol002`… (har biri alohida yuklab olinadi / share qilinadi).
- **Bildirishnoma** tugaganda: Telegram, email (SMTP), webhook — kanallar admin sozlamasida, manzillar user profilida. **Auto-share**: tugaganda 7 kunlik share link (xabarda ham).
- Formatlar: `.tar.gz` (default), `.log.gz`, `.log`.

### Rejali exportlar
- Cron (5 maydon, presetlar: har soat / har kuni / har hafta), oraliq "oxirgi N" + to'liq davrga tekislash (masalan kechagi 00:00–24:00), format/tarkib/xabar/auto-share, yoqish/o'chirish, "Hozir ishga tushirish", oxirgi job va xato ko'rinadi. Server o'chiq paytda o'tkazib yuborilgan run bitta marta qoplanadi.

### Jobs
- Navbatda / Running / Done / Failed; diskda saqlanadi; oddiy user faqat o'z joblarini, admin hammasini ko'radi. Running job o'chirilsa to'xtatiladi. Share link (muddat + limit, ko'p faylli export uchun har fayl URL'i). Stats: disk, kvota, saqlash muddati, navbat.
- **Retention**: N kundan eski joblar va disk kvotasi (admin, Settings) — avtomatik tozalash.

### Xavfsizlik va ekspluatatsiya
- JWT (72h), bcrypt; user har so'rovda store'dan tekshiriladi (o'chirilgan user → darhol 401, rol o'zgarishi darhol).
- **Login himoyasi** (brute-force: IP va user+IP bo'yicha), **download token** (5 daqiqalik, job'ga bog'langan, URL'da sessiya JWT yo'q).
- **Audit log** (`audit.jsonl`): kirish, export, yuklab olish, share (public yuklab olish IP bilan), rejalar, userlar, datasource, sozlamalar — admin sahifada filtr + JSONL yuklab olish.
- **`/healthz`, `/readyz`, `/metrics`** (Prometheus) — auth'siz.
- Datasource: basic auth + custom header'lar + **tenant (`AccountID`/`ProjectID`, ixtiyoriy)**, "Ulanishni tekshirish" tugmasi, parol API'da qaytmaydi.
- Profil: email, Telegram chat ID, xabar yoqish, o'z parolini o'zgartirish.
- UZ / RU almashtirgich sidebar va login sahifasida; light/dark tema.

---

## Arxitektura

```
+----------------+        +---------------------------------+        +---------------+
|  Brauzer (SPA) | <----> |  Go backend (:3000)             | <----> |  VictoriaLogs |
|  React+Vite    |        |  - JWT auth, rate limit, audit  |        |  /select/...  |
+----------------+        |  - JSON file store              |        +---------------+
                          |  - worker queue (windows/retry) |
                          |  - scheduler (cron)             |------> Telegram / SMTP / webhook
                          |  - retention sweeper, metrics   |
                          |  - embedded frontend            |
                          +---------------+-----------------+
                                          |
                                          v
                              /var/lib/logdownloader/
                                ├── settings.json      # datasource'lar, retention, worker, notify, public_url
                                ├── users.json         # + email/telegram/notify
                                ├── queries.json
                                ├── jobs.json          # export joblar (persist)
                                ├── shares.json
                                ├── schedules.json
                                ├── history.json       # query tarixi
                                ├── audit.jsonl        # audit (rotatsiya: audit-<ts>.jsonl)
                                ├── jwt_secret         # 0600
                                └── exports/*.tar.gz   # (+ _volNNN.tar.gz, .log.gz, .log, .csv*)
```

### Backend struktura

| Path | Vazifasi |
|------|----------|
| [backend/cmd/main.go](backend/cmd/main.go) | Entry point — config, store, `RecoverJobs`, notifier, worker, scheduler, fon ishlari (share cleanup, retention), handler, embed, middleware zanjiri (metrics → CORS → auth). |
| [backend/internal/config/config.go](backend/internal/config/config.go) | YAML + env (`HOST`, `PORT`, `DATA_DIR`, `JWT_SECRET`, `EXPORT_CHUNK_MB`, `--config`). |
| [backend/internal/auth/](backend/internal/auth/) | bcrypt, sessiya/download JWT, `Middleware` (store lookup), `RequireAdmin`, `ClientIP`, `Limiter`. |
| [backend/internal/model/models.go](backend/internal/model/models.go) | `User`, `Datasource`, `Settings` (+Retention/Worker/Notify), `SavedQuery`, `ExportJob` (+Files, progress), `ShareLink`, `Schedule`, `AuditEntry`, `HistoryEntry`. |
| [backend/internal/store/](backend/internal/store/) | RWMutex ostidagi map'lar, JSON persist; `jobs.go` (recovery, prune), `shares.go`, `schedules.go`, `history.go`, `audit.go`. |
| [backend/internal/handler/](backend/internal/handler/) | `/api/*` — fayllar bo'yicha: auth/users/profil, datasources, queries, logs (viewer proxy'lari), jobs, shares, schedules, admin (audit, settings), system. |
| [backend/internal/worker/](backend/internal/worker/) | Navbat/dispatcher, vaqt oynalari, retry/rollback, tarkib transformatsiyasi, qismlar, paketlash (tar.gz volumelar), auto-share, notify chaqiruvi. |
| [backend/internal/scheduler/](backend/internal/scheduler/) | Cron parser (`cron.go`), 30s sikl, `RunNow`, `Range`. |
| [backend/internal/notify/notify.go](backend/internal/notify/notify.go) | Telegram Bot API, SMTP (STARTTLS, PLAIN), webhook JSON, test xabar. |
| [backend/internal/metrics/metrics.go](backend/internal/metrics/metrics.go) | Prometheus text metrikalar + HTTP status hisoblagich middleware. |
| [backend/internal/vlclient/client.go](backend/internal/vlclient/client.go) | VictoriaLogs so'rovlari: query (long-running, ctx), hits, field_names, field_values, streams, earliest-time probe, test connection; tenant header'lar. |
| [backend/web/embed.go](backend/web/embed.go) | `//go:embed dist/*`. |

### Frontend struktura

| Path | Sahifa / Vazifasi |
|------|------------------|
| [frontend/src/main.tsx](frontend/src/main.tsx) | React entry + `LangProvider` + `BrowserRouter`; theme'ni mount'dan oldin qo'llaydi. |
| [frontend/src/i18n.tsx](frontend/src/i18n.tsx) | uz/ru lug'at, `LangProvider`, `useT()`, `formatBytes`, `formatDuration`. |
| [frontend/src/App.tsx](frontend/src/App.tsx) | Route'lar: `/`, `/jobs`, `/schedules`, `/settings`, `/profile`, admin: `/users`, `/audit`; `/export` → `/`. |
| [frontend/src/api.ts](frontend/src/api.ts) | Bitta `api` object — barcha endpointlar (tail oqimi, download token, schedules, audit, settings). |
| [frontend/src/components/Layout.tsx](frontend/src/components/Layout.tsx) | Sidebar nav + UZ/RU + theme toggle. |
| [frontend/src/components/Histogram.tsx](frontend/src/components/Histogram.tsx) | Inline SVG gistogramma, zoom callback. |
| [frontend/src/components/FieldsPanel.tsx](frontend/src/components/FieldsPanel.tsx) | Maydonlar / qiymatlar / streamlar paneli. |
| [frontend/src/pages/LogViewerPage.tsx](frontend/src/pages/LogViewerPage.tsx) | Asosiy sahifa: panel (saqlangan/maydonlar/tarix), vaqt oralig'i, query + autocomplete, Search / Live tail, export sozlamalari, gistogramma, jadval + ustunlar, URL holati. |
| [frontend/src/pages/JobsPage.tsx](frontend/src/pages/JobsPage.tsx) | Stats, navbat/running (progress, ETA)/done (fayllar)/failed, share modal (ko'p faylli). |
| [frontend/src/pages/SchedulesPage.tsx](frontend/src/pages/SchedulesPage.tsx) | Rejalar ro'yxati va formasi, run-now, yoqish/o'chirish. |
| [frontend/src/pages/SettingsPage.tsx](frontend/src/pages/SettingsPage.tsx) | Datasource'lar (tenant, test) + admin: retention/kvota/navbat, tashqi manzil, bildirishnoma kanallari (test tugmalari). |
| [frontend/src/pages/ProfilePage.tsx](frontend/src/pages/ProfilePage.tsx) | Email, Telegram chat ID, xabar, parol o'zgartirish. |
| [frontend/src/pages/AuditPage.tsx](frontend/src/pages/AuditPage.tsx) | Admin: audit jadvali, filtr, pagination, JSONL. |
| [frontend/src/pages/UsersPage.tsx](frontend/src/pages/UsersPage.tsx) | Admin: userlar (rol, parol, email). |

---

## HTTP API

Hammasi `Authorization: Bearer <jwt>` talab qiladi, faqat `POST /api/login`, `GET /dl/...`, `/healthz`, `/readyz`, `/metrics`'dan tashqari. To'liq spec: [.memory/api-reference.md](.memory/api-reference.md).

| Method | Path | Kim | Vazifasi |
|--------|------|-----|----------|
| POST | `/api/login` | hamma | Token (rate limit: 429) |
| GET/PUT | `/api/me` | auth | Profil (email, telegram, notify) + o'z parolini o'zgartirish |
| GET/POST | `/api/datasources`, POST `/api/datasources/test` | auth | Ro'yxat (parolsiz), yaratish, ulanish testi |
| PUT/DELETE | `/api/datasources/{id}` | owner/admin | Tahrirlash / o'chirish |
| GET/POST/PUT/DELETE | `/api/queries[/{id}]` | auth / owner | Saqlangan querylar |
| GET | `/api/query`, `/api/fields`, `/api/field_values`, `/api/streams`, `/api/hits`, `/api/tail` | auth | VictoriaLogs proxy'lari |
| GET/DELETE | `/api/history` | auth | Query tarixi |
| POST | `/api/export` | auth | Job (queued): format, content, fields, archive_split_mb, notify, auto_share |
| GET | `/api/jobs`, `/api/stats` | auth | O'z joblari (admin: hammasi); disk/kvota/navbat |
| DELETE | `/api/jobs/{id}` | owner/admin | O'chirish (queued → navbatdan, running → cancel) |
| POST | `/api/jobs/{id}/download-token` → GET `/api/jobs/{id}/download?dt=&file=` | owner/admin | Yuklab olish |
| POST/GET/DELETE | `/api/jobs/{id}/share` | owner/admin | Share link (fayllar ro'yxati bilan) |
| GET | `/dl/{token}`, `/dl/{token}/{n}` | public | Share orqali yuklab olish (Range yo'q) |
| GET/POST/PUT/DELETE | `/api/schedules[/{id}]`, POST `/api/schedules/{id}/run` | auth / owner | Rejali exportlar |
| GET/POST/PUT/DELETE | `/api/users[/{id}]` | admin | User CRUD |
| GET | `/api/audit`, `/api/audit/download` | admin | Audit |
| GET/PUT | `/api/settings`, POST `/api/settings/notify/test` | admin | Retention, worker, notify, public_url |
| GET | `/healthz`, `/readyz`, `/metrics` | public | Health, Prometheus |

---

## Konfiguratsiya

Prioritet: **env > YAML config**. YAML qidirish: `--config`, `CONFIG_PATH`, `./config.yaml`, `/etc/logdownloader/config.yaml`.

```yaml
host: ""
port: "3000"
data_dir: "/var/lib/logdownloader"
jwt_secret: ""          # ixtiyoriy — bo'sh bo'lsa data_dir/jwt_secret auto
export_chunk_mb: 100    # avtomatik bo'laklash: .tar.gz ichidagi bitta qism (MB); 0 = o'chiq
```

Env: `HOST`, `PORT`, `DATA_DIR`, `JWT_SECRET`, `EXPORT_CHUNK_MB`. **systemd service env bermaydi** — config.yaml ishlaydi.

Qolgan sozlamalar (retention, kvota, navbat, bildirishnoma kanallari, tashqi manzil) — admin UI'da, `settings.json` da.

---

## Ishga tushirish

```bash
# Lokal: backend (frontend embed bo'lmasa `/` 404)
cd backend && go run ./cmd

# Smoke test
make build && python3 -I scripts/smoke/smoke.py

# Production
make build              # build/logdownloader-linux-amd64 (VERSION=... ldflags)
make package-rpm / package-deb

# Docker
docker compose up --build   # http://localhost:3000 (admin/admin), VictoriaLogs :9428, log-generator
```

Systemd (paket): binary `/usr/bin/logdownloader`, unit `/usr/lib/systemd/system/logdownloader.service`, config `/etc/logdownloader/config.yaml`, data `/var/lib/logdownloader/`.

Release: `v*` tag → [.github/workflows/release.yml](.github/workflows/release.yml) amd64/arm64 RPM + DEB + binary.

---

## Xavfsizlik / cheklovlar

- Default admin paroli `admin` — birinchi kirgandan keyin Profil'dan o'zgartiring.
- CORS `*`. Datasource paroli, Telegram token, SMTP parol diskda plain (`settings.json`), API'da qaytmaydi.
- Download token URL'da (5 min, faqat shu job) — reverse-proxy logiga tushsa ham zarar minimal.
- `/api/query` `limit + offset` ni VictoriaLogs'dan oladi va xotirada kesadi — katta offset'larda og'irlik.
- Export `.tar.gz` ikki bosqichli (`.tmp`), vaqtincha 2× disk; volumelar siqilgan hajm bo'yicha qism granulyarligida kesiladi.
- Oyna o'rtasida uzilish → rollback va qayta urinish (3 marta); 4xx (noto'g'ri query) → darhol failed.
- Scheduler mahalliy server vaqtida ishlaydi; `lookback` + `align` bilan davr chegaralari.

Batafsil: [.memory/api-reference.md](.memory/api-reference.md), [.memory/data-model.md](.memory/data-model.md), [.memory/frontend-flow.md](.memory/frontend-flow.md).
