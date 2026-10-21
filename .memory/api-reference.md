# HTTP API Reference

Barcha endpointlar [handler.go](../backend/internal/handler/handler.go)'da. Auth Bearer JWT majburiy, `/api/login` va `/dl/{token}`'dan tashqari. Middleware token'dagi `user_id` bo'yicha userni **store'dan** o'qiydi (o'chirilgan user → 401 `user not found`; rol store'dan olinadi).

## Auth

### POST `/api/login`
```json
// req
{ "username": "admin", "password": "admin" }

// 200
{ "token": "<jwt>", "user_id": "uuid", "username": "admin", "role": "admin" }

// 401: "invalid credentials"
```

### GET `/api/me`
```json
// 200 — store'dagi joriy qiymatlar (rol o'zgargan bo'lsa yangisi)
{ "user_id": "uuid", "username": "admin", "role": "admin" }
```

## Datasources

### GET `/api/datasources`
Qaytarilgan ro'yxat: user owner bo'lgan + `global: true` bo'lgan barcha datasource'lar. **`password` qaytarilmaydi**, o'rniga `has_password`.

```json
[
  {
    "id": "uuid",
    "name": "Prod VL",
    "url": "http://victorialogs:9428",
    "username": "basic-auth-user",
    "has_password": true,
    "headers": { "X-Tenant": "team-a" },
    "global": false,
    "owner_id": "user-uuid"
  }
]
```

### POST `/api/datasources`
Body — Datasource (id/owner_id e'tibordan chiqariladi, server o'rnatadi). `global: true` faqat admin'da saqlanadi, oddiy user'da `false`'ga qaytariladi. Javob — yuqoridagi shakl (parolsiz).

### PUT `/api/datasources/{id}`
Owner yoki admin. Body — Datasource (id/owner_id preserve qilinadi). `global` faqat admin'da o'zgaradi. **Parol semantikasi:** `username` bo'sh bo'lmasa va `password` bo'sh/yo'q bo'lsa — mavjud parol saqlanib qoladi; `username` bo'sh bo'lsa parol ham tozalanadi. 404 — topilmasa, 403 — owner emas.

### DELETE `/api/datasources/{id}`
Owner yoki admin. Aks holda 403. Saqlash xatosi — 500.

## Saved Queries

### GET `/api/queries`
User'niki + global'lar.

### POST `/api/queries`
```json
{ "name": "Errors last hour", "query": "_time:1h AND level:error", "datasource_id": "uuid", "global": false }
```

### PUT `/api/queries/{id}`
Owner yoki admin. Body — SavedQuery (id/owner_id preserve qilinadi). `global` faqat admin'da o'zgartiriladi. 404 — topilmasa.

### DELETE `/api/queries/{id}`
Owner/admin tekshiruvi `handler.go`'da — ID topilmasa ham 204 (idempotent).

## Log query (VictoriaLogs proxy)

### GET `/api/query`
| param | mavzu |
|-------|-------|
| `query` | LogsQL — majburiy |
| `datasource_id` | majburiy |
| `limit` | default 50 |
| `offset` | default 0 |
| `start` / `end` | ixtiyoriy RFC3339 UTC — bo'sh bo'lsa cheksiz |
| `sort` | `asc` yoki `desc` — `_time` bo'yicha. Server query'ga `\| sort by (_time)[desc]` pipe qo'shadi. Agar user query'sida `\|` mavjud bo'lsa avtomatik sort qo'shilmaydi. |

Server VictoriaLogs `/select/logsql/query`'dan `limit+offset` qator oladi, NDJSON'ni o'qib, in-memory slice qiladi:

```json
{
  "logs": [ { "_time": "...", "_msg": "...", "level": "info", "...": "..." } ],
  "total": 123,
  "has_more": true,
  "limit": 50,
  "offset": 0
}
```

`has_more = total == limit+offset` — bu approximation.

## Export jobs

### POST `/api/export`
```json
// req
{
  "query": "...",
  "datasource_id": "uuid",
  "start": "2026-05-21T05:00:00Z",
  "end": "2026-05-21T05:30:00Z",
  "sort_order": "asc",
  "name": "prod-errors",
  "format": "tar.gz"
}

// 200
{ "job_id": "job_1714000000000000000" }
```
`start`/`end` ixtiyoriy (RFC3339 UTC). `sort_order`: `"asc"` | `"desc"` | `""`. `format`: `"tar.gz"` (default — **bo'sh yoki noma'lum qiymat ham `tar.gz`**), `"log.gz"`, `"log"`. UI'da tanlanadi (default `tar.gz`). **Bo'laklash parametri yo'q** — doim avtomatik (server config `export_chunk_mb`). `datasource_id` mavjud bo'lmasa 404. Job egasi = joriy user (`owner_id`, `owner_name`).

`name` — fayl nomi (ixtiyoriy): bo'sh bo'lsa `export` prefix. Server `sanitizeFileName` orqali tozalaydi va doim oxiriga UTC timestamp + format suffix qo'shadi. Misol: `my-prod-logs` → `my-prod-logs_20260521-090655-123.tar.gz`.

**Format implementatsiyasi (worker.go):**
- `tar.gz` — ikki bosqich: oldin xom faylni `.tmp`'ga yozadi (tar header'i size talab qiladi), keyin `.tar.gz` quradi va `.tmp`'ni o'chiradi. **Avtomatik bo'laklash:** `partTracker` streaming paytida qator chegarasida qism chegaralarini belgilaydi (xom hajm `export_chunk_mb` × 1 MiB dan oshganda); natija ≥2 qism bo'lsa arxiv ichida `<nom>_part001.log`, `_part002.log`, …, aks holda bitta `<nom>.log`. `parts` job maydonida. Bitta qator limitdan katta bo'lsa o'zi alohida qism.
- `log.gz` — `bufio.Writer` → `gzip.Writer` → fayl, streaming
- `log` — `bufio.Writer` → fayl, streaming

Streaming `vlclient.DoLongRunning(ctx, ...)` orqali; job o'chirilsa ctx bekor qilinadi. Job `size` — diskdagi yakuniy fayl hajmi.

### GET `/api/jobs`
**Oddiy user faqat o'z joblarini**, admin hammasini ko'radi. `created_at` DESC.

```json
[
  {
    "id": "job_...",
    "owner_id": "user-uuid",
    "owner_name": "admin",
    "query": "...",
    "datasource_id": "uuid",
    "datasource_name": "Prod VL",
    "status": "running" | "done" | "failed",
    "file_name": "export_20260521-090655-123.tar.gz",
    "created_at": "2026-...",
    "error": "",
    "lines": 12345,
    "start": "...", "end": "...", "sort_order": "asc",
    "format": "tar.gz",
    "size": 1048576,
    "parts": 3
  }
]
```

### DELETE `/api/jobs/{id}`
Owner yoki admin (404 / 403). Running bo'lsa worker to'xtatiladi (`worker.Cancel`), fayl + `.tmp` + share'lar o'chadi, `jobs.json` yangilanadi.

### GET `/api/jobs/{id}/download`
Owner yoki admin. `Content-Disposition: attachment` bilan fayl (`http.ServeFile`, Range bor). Status `done` emas — 400.

**Auth:** `Authorization: Bearer <jwt>` standart yo'l. Brauzer `<a href download>` Authorization yubora olmagani sababli, **bu endpoint** uchun `?token=<jwt>` query param ham qabul qilinadi (auth middleware'da `strings.HasSuffix(path, "/download")` tekshiruvi).

## Share links

Public download URL (`/dl/{token}`) auth talab qilmaydi. Create/get/revoke — **job egasi yoki admin** (403 aks holda).

**1 share per job** qoidasi: yangi share yaratilsa, mavjud share avtomatik bekor qilinadi.

### POST `/api/jobs/{id}/share`
```json
// req
{
  "max_downloads": 5,                      // 0 = cheksiz
  "expires_at": "2026-05-22T10:00:00Z"     // RFC3339 UTC, bo'sh = cheksiz
}

// 201
{
  "token": "LmKTeynWiJTEtIiBuakjrw",
  "job_id": "job_...",
  "created_by": "<user-id>",
  "created_at": "...",
  "expires_at": "2026-05-22T10:00:00Z",
  "max_downloads": 5,
  "download_count": 0,
  "url": "http://server/dl/LmKTeynWiJTEtIiBuakjrw"
}
```
`expires_at` o'tmishda bo'lsa 400. Job `done` emas — 400.

### GET `/api/jobs/{id}/share`
Job uchun joriy share ma'lumotini qaytaradi yoki 404.

### DELETE `/api/jobs/{id}/share`
Share'ni bekor qiladi. Yo'q bo'lsa 204 (idempotent). Revoke fayl/job'ni o'chirmaydi (faqat link bekor bo'ladi).

### GET `/dl/{token}`
Public download. **Range qo'llab-quvvatlanmaydi** — `Accept-Ranges: none`, `Range` header e'tiborsiz, doim to'liq fayl 200 bilan. Sabab: har HTTP so'rov = bitta yuklab olish; download-manager'larning parallel chunk'lari limitni bir zumda tugatib faylni o'chirardi.

Mantiq (atomik, store mutex ostida):
1. Token bo'yicha share qidirish; yo'q bo'lsa 404
2. `expires_at` o'tgan bo'lsa → share/job/fayl o'chiriladi, 410 Gone
3. `download_count >= max_downloads` (max > 0) bo'lsa → o'chadi, 410 Gone
4. Bog'liq job yo'q yoki `done` emas → o'chadi, 404
5. `download_count++`, persist
6. Fayl `io.Copy` bilan oqim qilinadi (`Content-Length` bor)
7. Agar limit yetgan bo'lsa serve qilingandan keyin share+job+fayl o'chiriladi

### Background cleanup
Server `time.Ticker(1 * time.Minute)` orqali `s.CleanupExpiredShares()` ni chaqiradi. Startup'da `RecoverJobs` egasiz (job yo'q yoki done emas) share'larni ham olib tashlaydi.

## Users (admin only)

### GET `/api/users`
Parolsiz ro'yxat.

### POST `/api/users`
```json
{ "username": "alice", "password": "...", "role": "user" | "admin" }
```
`role` bo'sh bo'lsa `user`. Mavjud username — 409.

### PUT `/api/users/{id}`
```json
{ "role": "admin", "password": "newpass" }
```
`role` bo'sh bo'lsa o'zgarmaydi. `password` bo'sh bo'lsa eski parol qoladi. Username immutable. 404 — topilmasa. Rol o'zgarishi userning mavjud tokeni bilan keyingi so'rovdan kuchga kiradi.

### DELETE `/api/users/{id}`
O'zini o'chirishga ruxsat berilmaydi (400). O'chirilgan userning tokeni darhol 401 qaytaradi.
