# CLAUDE.md

Bu fayl shu repo ustida ishlaganda Claude Code (yoki boshqa AI agent) o'qiydigan qisqa qo'llanma. Loyiha to'liq tavsifi uchun [PROJECT.md](PROJECT.md).

## Qisqacha

LogDownloader — VictoriaLogs uchun web UI'li log query/export vositasi. Go backend (single static binary, React frontend `go:embed` orqali ichiga joylangan) + React 19 + Vite + TS frontend. Storage — disk ustida JSON fayllar. UI ikki tilli (uz/ru).

## Repo tashkili

```
backend/
  cmd/main.go              # entry: config -> store -> RecoverJobs -> notifier -> worker -> scheduler -> handler -> serve
  internal/
    auth/                  # JWT (sessiya + download token), bcrypt, middleware, rate limiter
    ldapauth/              # LDAP / Active Directory autentifikatsiyasi (go-ldap)
    config/                # YAML/env config (host, port, data_dir, jwt_secret, export_chunk_mb)
    handler/               # /api/* — handler.go (auth/users/profil), datasources, queries, logs (viewer proxy),
                           #   jobs, shares, schedules, admin (audit, settings), system (healthz, metrics)
    model/                 # data tiplari
    store/                 # mutex'li JSON store: store.go (settings/users/queries), jobs.go, shares.go,
                           #   schedules.go, history.go, audit.go (jsonl)
    worker/                # export dvigateli: worker.go (navbat/dispatcher), windows.go (vaqt oynalari),
                           #   export.go (retry/rollback), sink.go (tarkib transformatsiyasi + qismlar), package.go (tar.gz/volumelar)
    scheduler/             # cron parser + rejali exportlar
    notify/                # Telegram / SMTP / webhook
    metrics/               # Prometheus text metrikalar + HTTP hisoblagich
    vlclient/              # VictoriaLogs HTTP wrapper (tenant header'lar, hits/field_names/streams/tail)
  web/embed.go             # //go:embed dist/*
  web/dist/                # frontend build (gitignored, build paytida copy qilinadi)
frontend/
  src/i18n.tsx             # uz/ru lug'at + LangProvider + useT() + formatBytes/formatDuration
  src/api.ts               # bitta `api` object — barcha fetch
  src/App.tsx              # route + auth gating
  src/components/          # Layout, Histogram (SVG), FieldsPanel
  src/pages/*.tsx          # Login, LogViewer, Jobs, Schedules, Settings, Profile, Audit, Users
scripts/smoke/             # end-to-end smoke test (vaqtga sezgir mock VictoriaLogs + python, 87 tekshiruv)
.memory/                   # qo'shimcha texnik reference
config.yaml                # default config (paketga kiradi)
Dockerfile, docker-compose.yml
Makefile, nfpm.yaml        # build + RPM/DEB
.github/workflows/release.yml
```

## Frequent commands

```bash
# Full build (frontend -> embed -> Go binary)
make build

# Faqat backend (frontend dist allaqachon bor bo'lsa)
cd backend && go run ./cmd

# Frontend type-check / dev
cd frontend && npx tsc --noEmit -p tsconfig.app.json
cd frontend && npm run dev          # API'ni proxy qilmaydi

# Smoke test (binary + mock VictoriaLogs, ~2 daqiqa, 87 tekshiruv)
make build && python3 -I scripts/smoke/smoke.py
# race detector bilan:
cd backend && go build -race -o ../build/ld-race ./cmd && cd .. && LOGDOWNLOADER_BIN=build/ld-race SMOKE_PORT=13100 SMOKE_VL_PORT=19528 python3 -I scripts/smoke/smoke.py

# Docker compose (VictoriaLogs + log-generator + app)
docker compose up --build

# Paket
make package-rpm
make package-deb
```

> `make frontend` yoki `npm run build`'dan keyin `backend/web/dist` to'ldirilgan bo'lishi shart, aks holda `cmd/main.go` ichidagi `fs.Sub(web.StaticFiles, "dist")` panic qiladi.

> Smoke test WSL'da `http_proxy` o'rnatilgan bo'lsa ham ishlaydi (skript loopback uchun proxy'siz opener qo'yadi). `pkill -f`/`pgrep -f` bilan jarayon o'ldirmang — pattern o'z shell buyrug'ingizga ham mos kelib shell'ni o'ldiradi; `pkill -x logdownloader` yoki PID ishlating.

## Mustlar / muhim nuktalar

### UI
- **Ikki tilli — uz (lotin, default) va ru.** Barcha matn `frontend/src/i18n.tsx` lug'atidan `t("key")`. Yangi matn **ikkala** lug'atga (TypeScript `ru` ni `Record<TKey, string>` sifatida majburlaydi). Kutubxona qo'shmang.
- **Export sahifasi yo'q** — hammasi Log Viewer'da. `/export` → `/`.
- **Arxiv formati tanlanadi, default `.tar.gz`**; **bo'laklash qulflangan "Avtomatik"** (chip, tanlov yo'q). Tarkib: `ndjson` | `msg` | `csv` (+ maydonlar), arxiv hajmi chegarasi (tar.gz), tugaganda xabar / auto-share — "Export sozlamalari" `<details>` ichida.
- Log Viewer chap paneli: Saqlangan (**papkalar daraxti** — `SavedQuery.Folder` "a/b/c", papka alohida obyekt emas; **"Global querylarni ko'rsatish"** toggle'i, yig'ish holati localStorage) / Maydonlar (field_names → field_values → query'ga qo'shish, streamlar) / Tarix. Gistogramma (`/api/hits`, ustun bosilsa zoom), Live tail (`/api/tail`, NDJSON oqim, AbortController), autocomplete (maydon nomlari, Tab), ustunlar sozlamasi (localStorage, datasource bo'yicha), URL'da `q/ds/from/to/sort`.
- Jobs: Navbatda / Running (progress bar, ETA, oynalar, retry) / Done (ko'p faylli bo'lsa har fayl tugmasi) / Failed; stats panel (disk, kvota, navbat). Download `POST /download-token` → `?dt=` (sessiya JWT URL'ga tushmaydi).

### Backend
- **Go 1.22+ mux pattern** (`mux.HandleFunc("POST /api/...")`, `r.PathValue`). `gorilla/mux` qo'shmang.
- **Auth middleware** `/api/login` va `/api/`'dan tashqari hamma yo'lni bypass qiladi (static, `/dl/`, `/healthz`, `/metrics`). Userni har so'rovda store'dan o'qiydi. Yangi public endpoint — [auth.go](backend/internal/auth/auth.go) `Middleware`. `/download` yo'llarida `?dt=` (typ=download, job'ga bog'langan, 5 min) qabul qilinadi; eski `?token=<jwt>` **yo'q**.
- **Login himoyasi**: IP bo'yicha 30/min, user+IP bo'yicha 5 xato / 15 min → 429 (`auth.Limiter`, xotirada).
- **LDAP / AD** (`ldapauth`, sozlama `Settings.LDAP`, admin UI): kirish tartibi — **lokal paroli bor user → faqat lokal**; aks holda `LDAP.Enabled` bo'lsa `ldapauth.Authenticate` (service bind → UserFilter qidiruv → user DN bind → memberOf + AD nested guruhlar). LDAP user birinchi kirishda yaratiladi (`AuthSource=ldap`, parol hash yo'q, username lower-case); `AdminGroupDN` berilsa rol har kirishda LDAP bo'yicha; `AllowedGroupDN` bo'lsa faqat shu guruh. Admin LDAP userga parol qo'ysa u lokalga aylanadi. Bo'sh parol bilan bind anonim deb o'tib ketmasin — oldindan rad etiladi. LDAP serverni smoke test qamramaydi (mock yo'q) — real AD'da `POST /api/settings/ldap/test` bilan sinang.
- **Store nusxa qaytaradi** (`GetSettings`/`GetDatasource`/`GetJob`/`GetJobsForUser`/`GetSchedule`); ichki pointer qaytaruvchi metod qo'shmang. `UpdateJob(id, persist, fn)` — mutex ostida mutatsiya; progress uchun `persist=false`.
- **Joblar `jobs.json` da.** Holatlar: `queued → running → done|failed`. Startup `RecoverJobs`: running → failed, queued qayta navbatga, orphan fayllar o'chadi. Worker navbati `settings.worker.max_concurrent` (admin UI), dispatcher `Signal()` bilan uyg'otiladi.
- **Export dvigateli (worker/)** — o'zgartirishdan oldin oqimni tushuning:
  1. `planWindows`: oraliq ≤2h → bitta so'rov; ≤48h → 1h oynalar; ≤14d → 6h; aks holda 24h (max 2000 oyna). Ochiq boshlanish → `EarliestTime` probe (pipe'li query'da bo'lmaydi). Oyna `end` = keyingi `start` − 1ns.
  2. `CountHits` (pipe'siz query) → `total_estimate`.
  3. Har oyna: `sink.mark()` (bufer **flush** qilinadi — rollback faqat shu oynani tashlasin) → `fetchWindow` → xato: 5xx/tarmoq = retryable (3 marta, backoff 2/5/15s, `sink.rollback` truncate), 4xx = failed.
  4. `rawSink`: tarkib transformatsiyasi (`transform`) + `partTracker` (qism chegaralari qator chegarasida, `export_chunk_mb`), `.tmp` faylga.
  5. `packageOutput`: tar.gz → qismlar entry (`<nom>_partNNN.<ext>`), `archive_split_mb` → volumelar (`<nom>_volNNN.tar.gz`, siqilgan hajm bo'yicha, qism granulyarligida); log.gz → gzip; log → rename (CSV sarlavha bo'lsa nusxa). CSV sarlavhasi har qism/fayl boshida.
  6. done → `Files`, `Size`, `Parts` → auto-share (7 kun) → `notifier.JobFinished` (goroutine).
  Timeout qo'shmang (soatlab oqim normal); bekor qilish faqat context (`Cancel`).
- **Scheduler**: 5 maydonli cron (o'z parser, `@hourly`… presetlar), 30s tick, `Range(now, lookback, align)`, o'tkazib yuborilgan run bitta marta qoplanadi, `RunNow`. `robfig/cron` qo'shmang.
- **Retention** (settings, admin): `PruneExports(days, quota)` har 10 daqiqada va startup'da; running/queued tegilmaydi.
- **Audit** `audit.jsonl` (append, 20 MB da rotatsiya, 3 ta eski fayl). Yangi muhim amal qo'shsangiz `h.audit(r, "obj.action", target, details)` chaqiring.
- **Metrics** `/metrics` (kutubxonasiz text format). Yangi hisoblagich — `metrics` paketidagi atomic'lar.
- **Bildirishnoma**: kanallar admin `Settings.Notify` (token/parol API'da qaytmaydi, faqat `has_*`), manzillar user profilida. Webhook har job uchun (sozlangan bo'lsa).
- **Datasource**: parol API'da qaytmaydi (`has_password`), PUT bo'sh parol = saqlanadi; `AccountID`/`ProjectID` ixtiyoriy tenant header'lar (Go `Accountid` deb kanonlashtiradi — normal).
- **`/dl/{token}` va `/dl/{token}/{n}`** Range qo'llamaydi (`Accept-Ranges: none`).
- **JWT secret** `<DataDir>/jwt_secret` auto-generate yoki `JWT_SECRET`/`jwt_secret:`.
- **systemd service'da `PORT`/`DATA_DIR` env BERILMAYDI** (env YAML'dan ustun). **Docker image'da `ca-certificates`** shart.
- Versiya `-ldflags "-X main.version=..."` (Makefile `VERSION`, CI tag, Dockerfile `ARG VERSION`).

## Stil

- Go: standard library'ga ustuvor; external dep'lar: uuid, jwt, x/crypto, yaml, go-ldap (+asn1-ber). Yangi dep qo'shishdan oldin kelishish.
- React: function components + hooks, `useState` + props + `LangContext`. State library, i18n library, chart library yo'q (gistogramma — inline SVG).
- CSS: `Pages.css` + `Layout.css` + global `index.css`, CSS o'zgaruvchilar (ikkala tema). **Forma tizimi** (v0.7.1): `.tabs/.tab`, `.panel/.panel-head/.panel-title/.panel-desc/.panel-sub/.panel-actions`, `.form-grid[.cols-3]` + `.field` (+`.span-2`) + `.hint` + `.field-row` + `.check/.check-group`. Yangi forma/sozlama shu klasslar bilan; eski `.form-group`/`.global-check` faqat modallarda qoldi.
- Backend errorlar: `http.Error(w, msg, code)` plain text.

## Test

Unit test yo'q (hozircha kerak emas deb kelishilgan). `scripts/smoke/smoke.py` + `mock_vl.py` — end-to-end: auth/rate limit/profil, system endpointlar, datasource (parol, tenant, test), viewer proxy'lari (fields/values/streams/hits/tail/history), export (default tar.gz, estimate, ownership, persist), download token, tarkib formatlari, qismlar + volumelar, vaqt oynalari + retry/rollback, share, navbat + cancel, schedules, settings + notify test + audit, JWT, auto-share, restart recovery + retention. Backend xatti-harakatini o'zgartirsangiz shu testni yangilab ishga tushiring; race build bilan ham o'tishi kerak. Mock `limit/start/end` ni hurmat qiladi; qatorlar 2026-10-21 10:00–10:49Z da.

## Memory file references

- [.memory/api-reference.md](.memory/api-reference.md) — har bir endpoint request/response misol bilan.
- [.memory/data-model.md](.memory/data-model.md) — store JSON shape va invariantlar.
- [.memory/frontend-flow.md](.memory/frontend-flow.md) — sahifalar, auth flow, i18n.
