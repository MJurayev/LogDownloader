# LogDownloader

VictoriaLogs uchun web-based log query va export vositasi. LogsQL queryni brauzerda yozasiz, natijani jadval ko'rinishida ko'rasiz va `.log` fayl sifatida eksport qilib yuklab olasiz.

Backend Go bilan yozilgan single static binary (React frontend `embed.FS` orqali shu binarni ichiga joylanadi). Front-end React 19 + TypeScript + Vite. Storage — disk ustida oddiy JSON fayllar (database yo'q).

---

## Asosiy imkoniyatlar

- **Log Viewer** — LogsQL query yozish, natijani jadval ko'rinishida ko'rish (pagination, level highlight, qatorni JSON detail bilan ochish).
- **Export** — query'ni background worker'ga jo'natadi; worker VictoriaLogs'dan stream qilib oladi va `.log` fayl yaratadi.
- **Jobs** — running / done / failed exportlar; tugaganini "Yuklab olish" tugmasi orqali olish, qator soni va xato matni ko'rinadi.
- **Saved Queries** — tez-tez ishlatadigan querylarni nomi bilan saqlash. Admin "Global" deb belgilab hammaga ko'rinadigan qilishi mumkin.
- **Multi-datasource** — bir necha VictoriaLogs endpoint qo'shsa bo'ladi (basic auth + custom headers bilan). Per-user yoki global.
- **Auth** — JWT (72h TTL), bcrypt parol hash. Birinchi ishga tushganda `admin/admin` yaratiladi.
- **Roles** — `admin` (foydalanuvchilar, global datasource/query) va `user` (faqat o'zining resurslari va global'lar).

---

## Arxitektura

```
+----------------+        +-----------------------+        +---------------+
|  Brauzer (SPA) | <----> |  Go backend (:3000)   | <----> |  VictoriaLogs |
|  React+Vite    |        |  - JWT auth           |        |  /select/...  |
+----------------+        |  - JSON file store    |        +---------------+
                          |  - worker (goroutine) |
                          |  - embedded frontend  |
                          +-----------+-----------+
                                      |
                                      v
                              /var/lib/logdownloader/
                                ├── settings.json
                                ├── queries.json
                                ├── users.json
                                └── exports/*.log
```

### Backend struktura

| Path | Vazifasi |
|------|----------|
| [backend/cmd/main.go](backend/cmd/main.go) | Entry point — config, store, worker, handler, embed serving, CORS, auth middleware. |
| [backend/internal/config/config.go](backend/internal/config/config.go) | YAML config + env override (`PORT`, `DATA_DIR`, `--config`). |
| [backend/internal/auth/auth.go](backend/internal/auth/auth.go) | bcrypt parol, JWT, `Middleware`, `RequireAdmin`. |
| [backend/internal/model/models.go](backend/internal/model/models.go) | `User`, `Datasource`, `SavedQuery`, `ExportJob`, `Role`, `JobStatus`. |
| [backend/internal/store/store.go](backend/internal/store/store.go) | Thread-safe (`sync.RWMutex`) in-memory map'lar, har o'zgarishda JSON faylga yoziladi. |
| [backend/internal/handler/handler.go](backend/internal/handler/handler.go) | Barcha `/api/...` HTTP handlerlar (Go 1.22+ `mux` pattern bilan). |
| [backend/internal/worker/worker.go](backend/internal/worker/worker.go) | `StartExport` — yangi job yaratadi va `go w.runExport(job)` ishga tushiradi. |
| [backend/internal/vlclient/client.go](backend/internal/vlclient/client.go) | Datasource auth/headers'ni qo'shadigan HTTP wrapper. |
| [backend/web/embed.go](backend/web/embed.go) | `//go:embed dist/*` — frontend build artifact'i binarning ichiga kiradi. |

### Frontend struktura

| Path | Sahifa / Vazifasi |
|------|------------------|
| [frontend/src/main.tsx](frontend/src/main.tsx) | React entry + `BrowserRouter`. |
| [frontend/src/App.tsx](frontend/src/App.tsx) | Token mavjudligiga qarab Login yoki Layout + route'larga yo'naltiradi. |
| [frontend/src/api.ts](frontend/src/api.ts) | Bitta `api` object — barcha endpoint chaqiruvlari, 401'da localStorage tozalanib `/login` ga redirect. |
| [frontend/src/components/Layout.tsx](frontend/src/components/Layout.tsx) | Sidebar nav + outlet. |
| [frontend/src/pages/LoginPage.tsx](frontend/src/pages/LoginPage.tsx) | Username/password form. |
| [frontend/src/pages/LogViewerPage.tsx](frontend/src/pages/LogViewerPage.tsx) | Asosiy sahifa: datasource selector + LogsQL textarea + Search/Save/Export + jadval + saved queries panel. |
| [frontend/src/pages/ExportPage.tsx](frontend/src/pages/ExportPage.tsx) | Faqat export'ni boshlash uchun alohida form. |
| [frontend/src/pages/JobsPage.tsx](frontend/src/pages/JobsPage.tsx) | Jobs ro'yxati; har 2s da auto-refresh. |
| [frontend/src/pages/SettingsPage.tsx](frontend/src/pages/SettingsPage.tsx) | Datasource'larni qo'shish/o'chirish. |
| [frontend/src/pages/UsersPage.tsx](frontend/src/pages/UsersPage.tsx) | Admin uchun foydalanuvchilarni boshqarish. |

Yaml config'da JWT secret ham qo'shimcha qiymat sifatida belgilash mumkin:

```yaml
port: "3000"
data_dir: "/var/lib/logdownloader"
jwt_secret: "<hex-string>"   # ixtiyoriy — bo'sh bo'lsa data_dir/jwt_secret faylidan auto-load qilinadi
```

---

## HTTP API

Hammasi `Authorization: Bearer <jwt>` talab qiladi, faqat `POST /api/login`'dan tashqari. To'liq spec uchun [.memory/api-reference.md](.memory/api-reference.md).

| Method | Path | Kim | Vazifasi |
|--------|------|-----|----------|
| POST | `/api/login` | hamma | Token oladi |
| GET | `/api/me` | auth | Joriy user |
| GET | `/api/datasources` | auth | Userning + global datasource'lar |
| POST | `/api/datasources` | auth | Yangi datasource (global faqat admin) |
| PUT | `/api/datasources/{id}` | owner/admin | Tahrirlash |
| DELETE | `/api/datasources/{id}` | owner/admin | O'chirish |
| GET | `/api/queries` | auth | Userning + global queries |
| POST | `/api/queries` | auth | Yangi saqlangan query |
| PUT | `/api/queries/{id}` | owner/admin | Tahrirlash |
| DELETE | `/api/queries/{id}` | owner/admin | O'chirish |
| GET | `/api/query?query=&datasource_id=&limit=&offset=&start=&end=` | auth | VictoriaLogs'ga proxy + paginate |
| POST | `/api/export` | auth | Background job ishga tushadi |
| GET | `/api/jobs` | auth | Barcha joblar |
| DELETE | `/api/jobs/{id}` | auth | Job + fayl o'chadi |
| GET | `/api/jobs/{id}/download` | auth | `.log` faylni qaytaradi (done bo'lsa) |
| GET/POST/DELETE | `/api/users[...]` | admin | User CRUD |

---

## Konfiguratsiya

Prioritet (yuqoridan pastga override): **env > YAML config**.

YAML qidirish tartibi:
1. `--config <path>` flag
2. `CONFIG_PATH` env var
3. `./config.yaml`
4. `/etc/logdownloader/config.yaml`

```yaml
port: "3000"
data_dir: "/var/lib/logdownloader"
```

Env: `PORT`, `DATA_DIR`.

`DataDir` ichida `settings.json`, `users.json`, `queries.json`, `exports/` papkasi yaratiladi.

---

## Ishga tushirish

### Lokal (dev)

```bash
# Backend (port 3000, frontend embed bo'lmasligi mumkin — bu holda `/` 404)
cd backend && go run ./cmd

# Frontend (port 5173, API'ni proxy qilmaydi — `/api/...` 404 bo'ladi, agar Vite proxy sozlanmagan bo'lsa)
cd frontend && npm install && npm run dev
```

> Hozircha frontend Vite dev server'ida API'ni proxy qilmaydi. Faqat embed qilingan build orqali ishlaydi. Dev paytida `Makefile`'dagi `make frontend && go run ./cmd` ishlatilsa to'g'ri bo'ladi.

### Production binary

```bash
make build              # build/logdownloader-linux-amd64
make package-rpm        # build/logdownloader-<version>.rpm
make package-deb        # build/logdownloader-<version>.deb
```

### Docker

```bash
docker compose up --build
# http://localhost:3000  (admin / admin)
# VictoriaLogs http://localhost:9428
# log-generator service 5 soatlik sintetik loglarni yaratadi
```

### Systemd (paket o'rnatilgandan keyin)

- Binary: `/usr/bin/logdownloader`
- Service: `/usr/lib/systemd/system/logdownloader.service` (user: `logdownloader`)
- Config: `/etc/logdownloader/config.yaml`
- Data: `/var/lib/logdownloader/`

```bash
systemctl status logdownloader
journalctl -u logdownloader -f
```

---

## Release flow

`v*` tag push qilinganda [.github/workflows/release.yml](.github/workflows/release.yml) `amd64` va `arm64` uchun RPM + DEB yaratib GitHub Release'ga yuklaydi.

```bash
git tag v0.1.0
git push origin v0.1.0
```

---

## Xavfsizlik / cheklovlar

- JWT secret birinchi ishga tushganda `<DataDir>/jwt_secret` faylida 32 baytlik random hex sifatida yaratiladi (0600). `JWT_SECRET` env yoki `jwt_secret:` YAML qiymati orqali ham bersa bo'ladi. Bu fayl o'chirilsa — barcha mavjud tokenlar yaroqsiz bo'ladi (re-login kerak).
- Default admin paroli `admin` — birinchi kirgandan keyin o'zgartirish lozim (Users sahifasidan yangi admin yaratib eskisini o'chirish bilan).
- CORS hamma origin'larga ochiq (`*`).
- Datasource basic auth paroli storage'da plain JSON sifatida saqlanadi (`settings.json`). Disk shifrlanmagan bo'lsa, ehtiyot bo'lish kerak.
- `/api/query`'da `limit + offset` orqali server haqiqiy data'ni shu miqdorda fetch qiladi va shundan keyin in-memory slice qiladi — katta offset'larda VictoriaLogs'ga og'irlik tushadi.
- Export job worker concurrency limiti yo'q — har export alohida goroutine, ko'p parallel export RAM/network'ni egallaydi.

Batafsil texnik kontekst uchun `.memory/`:
- [.memory/api-reference.md](.memory/api-reference.md) — barcha endpointlar request/response bilan
- [.memory/data-model.md](.memory/data-model.md) — storage shakli va JSON shape'lar
- [.memory/frontend-flow.md](.memory/frontend-flow.md) — sahifalar va o'zaro ta'sir
