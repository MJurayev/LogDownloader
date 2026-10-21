# CLAUDE.md

Bu fayl shu repo ustida ishlaganda Claude Code (yoki boshqa AI agent) o'qiydigan qisqa qo'llanma. Loyiha to'liq tavsifi uchun [PROJECT.md](PROJECT.md).

## Qisqacha

LogDownloader — VictoriaLogs uchun web UI'li log query/export vositasi. Go backend (single static binary, React frontend `go:embed` orqali ichiga joylangan) + React 19 + Vite + TS frontend. Storage — disk ustida JSON fayllar.

## Repo tashkili

```
backend/
  cmd/main.go              # entry: config -> store -> RecoverJobs -> worker -> handler -> serve
  internal/
    auth/                  # JWT + bcrypt + middleware (user har so'rovda store'dan o'qiladi)
    config/                # YAML/env config
    handler/               # /api/* HTTP handlerlar
    model/                 # data tiplari
    store/                 # mutex'li JSON fayl store (settings/users/queries/jobs/shares)
    worker/                # background export goroutine + per-job cancel
    vlclient/              # VictoriaLogs HTTP wrapper
  web/embed.go             # //go:embed dist/*
  web/dist/                # frontend build (gitignored, build paytida copy qilinadi)
frontend/
  src/i18n.tsx             # uz/ru lug'at + LangProvider + useT()
  src/api.ts               # bitta `api` object — barcha fetch
  src/App.tsx              # route + auth gating
  src/components/Layout.tsx
  src/pages/*.tsx          # Login, LogViewer, Jobs, Settings, Users
scripts/smoke/             # end-to-end smoke test (mock VictoriaLogs + python)
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

# Frontend dev (faqat UI, API'siz)
cd frontend && npm run dev

# Smoke test (binary + mock VictoriaLogs, ~15s)
make build && python3 -I scripts/smoke/smoke.py
# race detector bilan:
cd backend && go build -race -o ../build/ld-race ./cmd && LOGDOWNLOADER_BIN=../build/ld-race python3 -I ../scripts/smoke/smoke.py

# Docker compose (VictoriaLogs + log-generator + app)
docker compose up --build

# Paket
make package-rpm
make package-deb
```

> `make frontend` yoki `npm run build`'dan keyin `backend/web/dist` to'ldirilgan bo'lishi shart, aks holda `cmd/main.go` ichidagi `fs.Sub(web.StaticFiles, "dist")` panic qiladi (`embed` direktivasi bo'sh papkani qabul qilmaydi).

## Mustlar / muhim nuktalar

- **UI ikki tilli — uz (lotin, default) va ru.** Barcha ko'rinadigan matn `frontend/src/i18n.tsx` lug'atidan `t("key")` orqali olinadi. Yangi matn qo'shganda **ikkala** lug'atga kiriting (TypeScript `ru` ni `Record<TKey, string>` sifatida majburlaydi). Til `localStorage("lang")` da; sidebar va login sahifasida UZ/RU tugmasi.
- **Arxiv formati tanlanadi, default `.tar.gz`** (`.tar.gz` / `.log.gz` / `.log`). Backend bo'sh/noma'lum formatni `tar.gz` deb oladi (`worker.normalizeFormat`).
- **Bo'laklash qulflangan: faqat "Avtomatik".** Log Viewer'da bo'laklash tanlovi yo'q, "Bo'laklash: Avtomatik" chip'i bor. Server `.tar.gz` ichida xom hajm `export_chunk_mb` (default 100 MB, env `EXPORT_CHUNK_MB`, 0 = o'chiq) dan oshsa qator chegarasida `<nom>_partNNN.log` qismlarga bo'ladi, kichik export bitta `<nom>.log`. `log`/`log.gz` formatlari bitta fayl (bo'laklash faqat arxivda). Chegaralar `worker.partTracker` da streaming paytida aniqlanadi (qayta o'qish yo'q). UI'ga bo'laklash tanlovini qo'shmang.
- **Export sahifasi yo'q.** Barcha export funksiyasi Log Viewer'da (fayl nomi + format + Export tugmasi). `/export` route Log Viewer'ga redirect qiladi.
- **Go 1.22+ mux pattern** ishlatiladi (`mux.HandleFunc("POST /api/...", ...)` va `r.PathValue("id")`). Eski `gorilla/mux` qo'shmang.
- **Auth middleware** `/api/login` va `/api/`'dan tashqari hamma route'ni bypass qiladi — ya'ni static fayllar va `/dl/{token}` middleware'siz beriladi. Yangi public endpoint qo'shsangiz, [auth.go](backend/internal/auth/auth.go) dagi `Middleware` boshini yangilang. Middleware token'dagi `user_id` bo'yicha userni **store'dan** o'qiydi: o'chirilgan user tokeni 401, rol o'zgarishi keyingi so'rovdan kuchga kiradi.
- **Store nusxa qaytaradi.** `GetSettings`/`GetDatasource`/`GetJob`/`GetJobsForUser` chuqur nusxa beradi — handler ularni mutex'siz o'zgartirib `Save*` ga qaytarishi mumkin. Store'ning ichki slice/map'iga pointer qaytaruvchi metod qo'shmang (bu avval real data race edi).
- **Store o'zgarishlar atomar emas** — `saveFile` xatosi qaytsa, in-memory state allaqachon yangilangan bo'ladi. Yangi entity qo'shganda shu pattern'ni saqlash mumkin (overkill emas).
- **Joblar `jobs.json` da saqlanadi.** Progress (har 1000 qator) faqat xotirada; diskka faqat qo'shish/o'chirish/yakuniy holat yoziladi. Startup'da `store.RecoverJobs()`: running qolgan joblar failed, chala/`.tmp`/orphan fayllar o'chadi, egasiz share'lar o'chadi.
- **Job ownership.** `ExportJob.OwnerID/OwnerName`. Oddiy user faqat o'z joblarini ko'radi/yuklaydi/o'chiradi/share qiladi; admin hammasini. Tekshiruv `handler.loadJobForUser` orqali.
- **Running job o'chirilsa worker to'xtaydi** — `worker.Cancel(id)` context'ni bekor qiladi, chala fayl olib tashlanadi. Worker concurrency limiti yo'q (har export alohida goroutine); cap kerak bo'lsa worker pool, lekin oldindan kelishmasdan abstractsiya qilmang.
- **Export worker'da timeout yo'q (intentional).** `vlclient.DoLongRunning(ctx, ...)` dedicated client ishlatadi: `Timeout=0`, `ResponseHeaderTimeout=0`, `IdleConnTimeout=0`. Faqat initial dial (30s) va TLS handshake (30s)'da limit bor. Bu yerga timeout qo'shmang — export bir necha soat ishlashi normal. Uzish faqat context orqali.
- **Datasource paroli API javobida qaytmaydi** — `password` o'rniga `has_password`. PUT'da bo'sh parol = "o'zgartirilmasin" (username bo'sh bo'lmasa); username tozalansa parol ham o'chadi. Diskda (`settings.json`) parol plain — encryption kelishilgan ish emas.
- **`/dl/{token}` Range qo'llamaydi** (`Accept-Ranges: none`): bitta HTTP so'rov = bitta yuklab olish; aks holda download-manager chunk'lari limitni bir zumda tugatardi.
- **JWT secret** birinchi ishga tushganda `<DataDir>/jwt_secret`'da auto-generate qilinadi (random 32 bayt, hex) yoki `JWT_SECRET` env / `jwt_secret:` YAML qiymatidan olinadi. `main.go`'da `auth.SetSecret(...)` doim chaqiriladi.
- **Default admin/admin** birinchi ishga tushganda yaratiladi ([main.go](backend/cmd/main.go)). Bu chetlab o'tilmaydi, faqat keyin Users sahifasidan yangi admin yaratib eskisini o'chirib bo'ladi.
- **systemd service'da `PORT`/`DATA_DIR` env BERILMAYDI** — env YAML'dan ustun, aks holda `/etc/logdownloader/config.yaml` e'tiborga olinmaydi. Service faylga env qo'shmang.
- **Docker image'da `ca-certificates` bor** (HTTPS datasource uchun). Base image almashtirsangiz saqlab qoling.
- **Frontend dev server** `npm run dev` — Vite default port 5173, lekin API'ni proxy qilmaydi (vite.config.ts'da proxy yo'q). To'liq UI test uchun `make frontend` qilib backend orqali serve qiling.
- **Saved query API'sida `datasource_id` ixtiyoriy** — frontend uni doim yuboradi, lekin handler `Global` flagdan tashqari faqat name + query'ni majburiy qiladi.

## Stil

- Go: standard library'ga ustuvor; faqat 4 ta external dep (uuid, jwt, x/crypto, yaml). Yangi dep qo'shishdan oldin kelishish.
- React: function components + hooks. Class component yo'q. State management library yo'q — `useState` + props + bitta `LangContext` yetarli. i18n kutubxonasi qo'shmang — `i18n.tsx` yetarli.
- CSS: `Pages.css` + `Layout.css` + global `index.css`. CSS-in-JS yoki Tailwind yo'q.
- Backend errorlar: `http.Error(w, msg, code)` + JSON-siz plain text. Mavjud pattern'ni saqlang.

## Test

Unit test yo'q. `scripts/smoke/smoke.py` — binary + mock VictoriaLogs bilan end-to-end tekshiruv (auth, parol strip, export tar.gz, avtomatik bo'laklash `EXPORT_CHUNK_MB=1` bilan, ownership, share, cancel, JWT, restart recovery). Backend xatti-harakatini o'zgartirsangiz shu testni yangilab ishga tushiring; race detector build bilan ham o'tishi kerak. Yangi Go unit test qo'shganda `internal/.../*_test.go` joylash kerak.

## Memory file references

- [.memory/api-reference.md](.memory/api-reference.md) — har bir endpoint request/response misol bilan.
- [.memory/data-model.md](.memory/data-model.md) — store JSON shape va invariantlar.
- [.memory/frontend-flow.md](.memory/frontend-flow.md) — sahifalar, auth flow, i18n.
