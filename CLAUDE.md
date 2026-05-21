# CLAUDE.md

Bu fayl shu repo ustida ishlaganda Claude Code (yoki boshqa AI agent) o'qiydigan qisqa qo'llanma. Loyiha to'liq tavsifi uchun [PROJECT.md](PROJECT.md).

## Qisqacha

LogDownloader — VictoriaLogs uchun web UI'li log query/export vositasi. Go backend (single static binary, React frontend `go:embed` orqali ichiga joylangan) + React 19 + Vite + TS frontend. Storage — disk ustida JSON fayllar.

## Repo tashkili

```
backend/
  cmd/main.go              # entry: config -> store -> worker -> handler -> serve
  internal/
    auth/                  # JWT + bcrypt + middleware
    config/                # YAML/env config
    handler/               # /api/* HTTP handlerlar
    model/                 # data tiplari
    store/                 # mutex'li JSON fayl store
    worker/                # background export goroutine
    vlclient/              # VictoriaLogs HTTP wrapper
  web/embed.go             # //go:embed dist/*
  web/dist/                # frontend build (gitignored, build paytida copy qilinadi)
frontend/
  src/api.ts               # bitta `api` object — barcha fetch
  src/App.tsx              # route + auth gating
  src/components/Layout.tsx
  src/pages/*.tsx          # Login, LogViewer, Export, Jobs, Settings, Users
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

# Docker compose (VictoriaLogs + log-generator + app)
docker compose up --build

# Paket
make package-rpm
make package-deb
```

> `make frontend` yoki `npm run build`'dan keyin `backend/web/dist` to'ldirilgan bo'lishi shart, aks holda `cmd/main.go` ichidagi `fs.Sub(web.StaticFiles, "dist")` panic qiladi (`embed` direktivasi bo'sh papkani qabul qilmaydi).

## Mustlar / muhim nuktalar

- **UI tili — O'zbek (lotin).** Yangi tugma/yorliq qo'shganda til mosligini saqlang ("Saqlash", "O'chirish", "Yuklab olish" va h.k.).
- **Go 1.22+ mux pattern** ishlatiladi (`mux.HandleFunc("POST /api/...", ...)` va `r.PathValue("id")`). Eski `gorilla/mux` qo'shmang.
- **Auth middleware** `/api/login` va `/api/`'dan tashqari hamma route'ni bypass qiladi — ya'ni static fayllar ham middleware'siz beriladi. Yangi public endpoint qo'shsangiz, [auth.go:74](backend/internal/auth/auth.go#L74) ni yangilang.
- **Store o'zgarishlar atomar emas** — `saveFile` xatosi qaytsa, in-memory state allaqachon yangilangan bo'ladi. Yangi entity qo'shganda shu pattern'ni saqlash mumkin (overkill emas).
- **Worker concurrency limiti yo'q** — har `StartExport` alohida goroutine. Hozircha bu yetarli; cap qo'yish kerak bo'lsa worker pool yarating, lekin oldindan kelishmasdan abstractsiya qilmang.
- **Export worker'da timeout yo'q (intentional).** `vlclient.DoLongRunning` dedicated client ishlatadi: `Timeout=0`, `ResponseHeaderTimeout=0`, `IdleConnTimeout=0`. Faqat initial dial (30s) va TLS handshake (30s)'da limit bor. Bu yerga timeout qo'shmang — export bir necha soat ishlashi normal. Qator o'qishi `bufio.Reader.ReadString` orqali — qator uzunligi cheklanmagan.
- **JWT secret** birinchi ishga tushganda `<DataDir>/jwt_secret`'da auto-generate qilinadi (random 32 bayt, hex) yoki `JWT_SECRET` env / `jwt_secret:` YAML qiymatidan olinadi. `main.go`'da `auth.SetSecret(...)` doim chaqiriladi. Default hardcoded value endi ishlatilmaydi — agar `LoadOrCreateSecret` xato qaytarsa main fatal qiladi.
- **Datasource paroli plain JSON'da** saqlanadi. Encryption qo'shish kelishilgan ish emas.
- **Default admin/admin** birinchi ishga tushganda yaratiladi ([main.go:29](backend/cmd/main.go#L29)). Bu chetlab o'tilmaydi, faqat keyin Users sahifasidan yangi admin yaratib eskisini o'chirib bo'ladi.
- **Frontend dev server** `npm run dev` — Vite default port 5173, lekin API'ni proxy qilmaydi (vite.config.ts'da proxy yo'q). To'liq UI test uchun `make frontend` qilib backend orqali serve qiling.
- **Saved query API'sida `datasource_id` ixtiyoriy** — frontend uni doim yuboradi, lekin handler `Global` flagdan tashqari faqat name + query'ni majburiy qiladi.
- **QueriesPage.tsx** ishlatilmaydi (App.tsx'da route yo'q). Tegmang yoki o'chirib tashlash kerak bo'lsa — alohida task. `handleExport` chaqirig'i eski signature bilan (datasource'siz).

## Stil

- Go: standard library'ga ustuvor; faqat 3 ta external dep (uuid, jwt, crypto, yaml). Yangi dep qo'shishdan oldin kelishish.
- React: function components + hooks. Class component yo'q. State management library yo'q — `useState` + props yetarli.
- CSS: bitta `Pages.css` + `Layout.css`. CSS-in-JS yoki Tailwind yo'q.
- Backend errorlar: `http.Error(w, msg, code)` + JSON-siz plain text. Mavjud pattern'ni saqlang.

## Test

Hozircha unit yoki integration test yo'q. Yangi test qo'shganda Go uchun `internal/.../*_test.go` joylash kerak.

## Memory file references

- [.memory/api-reference.md](.memory/api-reference.md) — har bir endpoint request/response misol bilan.
- [.memory/data-model.md](.memory/data-model.md) — store JSON shape va invariantlar.
- [.memory/frontend-flow.md](.memory/frontend-flow.md) — sahifalar va auth flow.
