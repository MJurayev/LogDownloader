# HTTP API Reference

Barcha endpointlar `[handler.go](../backend/internal/handler/handler.go)`'da. Auth Bearer JWT majburiy, `/api/login`'dan tashqari.

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
// 200
{ "user_id": "uuid", "username": "admin", "role": "admin" }
```

## Datasources

### GET `/api/datasources`
Qaytarilgan ro'yxat: user owner bo'lgan + `global: true` bo'lgan barcha datasource'lar.

```json
[
  {
    "id": "uuid",
    "name": "Prod VL",
    "url": "http://victorialogs:9428",
    "username": "basic-auth-user",
    "password": "basic-auth-pass",
    "headers": { "X-Tenant": "team-a" },
    "global": false,
    "owner_id": "user-uuid"
  }
]
```

### POST `/api/datasources`
Body — Datasource (id/owner_id e'tibordan chiqariladi, server o'rnatadi). `global: true` faqat admin'da saqlanadi, oddiy user'da `false`'ga qaytariladi.

### PUT `/api/datasources/{id}`
Owner yoki admin. Body — Datasource (id/owner_id preserve qilinadi, mavjud qiymatlardan). `global: true` ham faqat admin'da o'zgaradi; oddiy user'ning request'idagi yangi `global` qiymati e'tiborga olinmaydi (eskisi qoladi). 404 — topilmasa, 403 — owner emas.

### DELETE `/api/datasources/{id}`
Owner yoki admin. Aks holda 403.

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
Owner/admin tekshiruvi `handler.go`'da — `else` branch'i yo'q, ya'ni ID topilmasa ham xato bermay o'tib ketadi.

## Log query (VictoriaLogs proxy)

### GET `/api/query`
| param | mavzu |
|-------|-------|
| `query` | LogsQL — majburiy |
| `datasource_id` | majburiy |
| `limit` | default 50 |
| `offset` | default 0 |
| `start` / `end` | ixtiyoriy RFC3339 UTC — bo'sh bo'lsa cheksiz (VictoriaLogs default: barcha loglar) |
| `sort` | `asc` yoki `desc` — `_time` bo'yicha. Server query'ga `\| sort by (_time)[desc]` pipe qo'shadi. Agar user query'sida `\|` mavjud bo'lsa avtomatik sort qo'shilmaydi (user pipe'ini buzmaslik uchun). Bo'sh bo'lsa VL default tartibida. |

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

`has_more = total == limit+offset` — bu approximation; VictoriaLogs aslida shuncha qator borligini bildirmaydi.

## Export jobs

### POST `/api/export`
```json
// req
{
  "query": "...",
  "datasource_id": "uuid",
  "start": "2026-05-21T05:00:00Z",
  "end": "2026-05-21T05:30:00Z",
  "sort_order": "asc"
}

// 200
{ "job_id": "job_1714000000000000000" }
```
`start`/`end` ixtiyoriy (RFC3339 UTC). `sort_order`: `"asc"` | `"desc"` | `""`. Pipe qo'shish mantiqi `/api/query`'dagi `sort` bilan bir xil (`vlclient.WithTimeSort`). Server `worker.StartExport`'ni chaqiradi; goroutine boshlanadi, in-memory map'ga yoziladi (jobs file'ga saqlanmaydi — server restart bo'lsa joblar yo'qoladi, lekin export fayllari diskda qoladi).

### GET `/api/jobs`
Barcha joblar (filterlamayd, har user hammasini ko'radi).

```json
[
  {
    "id": "job_...",
    "query": "...",
    "datasource_id": "uuid",
    "datasource_name": "Prod VL",
    "status": "running" | "done" | "failed",
    "file_name": "export_job_...log",
    "created_at": "2026-...",
    "error": "",
    "lines": 12345
  }
]
```

### DELETE `/api/jobs/{id}`
In-memory entry + diskdagi fayl o'chadi. Ownership yo'q.

### GET `/api/jobs/{id}/download`
`Content-Disposition: attachment` bilan `.log` fayl. Status `done` emas — 400.

## Users (admin only)

### GET `/api/users`
Parolsiz ro'yxat.

### POST `/api/users`
```json
{ "username": "alice", "password": "...", "role": "user" | "admin" }
```
`role` bo'sh bo'lsa `user`. Mavjud username — 409.

### PUT `/api/users/{id}`
Admin only. Body:
```json
{ "role": "admin", "password": "newpass" }
```
`role` bo'sh bo'lsa o'zgarmaydi. `password` bo'sh bo'lsa eski parol qoladi (faqat non-empty parol bcrypt qilinib yoziladi). Username immutable. 404 — topilmasa.

### DELETE `/api/users/{id}`
O'zini o'chirishga ruxsat berilmaydi (400).
