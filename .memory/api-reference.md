# HTTP API Reference

Handlerlar [backend/internal/handler/](../backend/internal/handler/) da (fayllar bo'yicha). Auth Bearer JWT majburiy, `/api/login`, `/dl/...`, `/healthz`, `/readyz`, `/metrics` dan tashqari. Middleware token'dagi `user_id` bo'yicha userni **store'dan** o'qiydi (o'chirilgan user → 401; rol store'dan). Xatolar plain text.

## Auth / profil

### POST `/api/login`
```json
{ "username": "admin", "password": "admin" }
→ 200 { "token": "<jwt>", "user_id": "uuid", "username": "admin", "role": "admin" }
```
401 `invalid credentials`. **Kirish tartibi:** lokal paroli bor va `auth_source != ldap` user → faqat lokal bcrypt; aks holda `settings.ldap.enabled` → LDAP (`ldapauth.Authenticate`); LDAP muvaffaqiyatli bo'lsa user yaratiladi/yangilanadi (`auth_source: ldap`, `dn`, email, rol — `admin_group_dn` berilsa LDAP bo'yicha). Audit `login.ok` details `local|ldap`, `login.fail` details `local` yoki `ldap: <sabab>`. **Rate limit:** IP bo'yicha 30 so'rov/min → 429 `too many requests` (Retry-After 60); bitta user+IP uchun 5 xato / 15 min → 429 `too many failed attempts...` (Retry-After 900). Audit: `login.ok` / `login.fail` / `login.blocked`.

### GET `/api/me`
`{ user_id, username, role, email, telegram_chat_id, notify_on_done, auth_source }` (store'dan; `auth_source`: `local` | `ldap`).

### PUT `/api/me`
```json
{ "email": "a@b", "telegram_chat_id": "123", "notify_on_done": true,
  "current_password": "...", "new_password": "..." }
```
Maydonlar ixtiyoriy (pointer semantikasi: berilmagani o'zgarmaydi). Parol o'zgarishi uchun `current_password` to'g'ri bo'lishi shart (403), `new_password` ≥ 6 belgi (400); LDAP user uchun 400 `password is managed by LDAP`. Audit `profile.update` / `password.change`.

## Datasources

### GET `/api/datasources`
User owner bo'lgan + `global` datasource'lar. **`password` qaytarilmaydi**, o'rniga `has_password`.
```json
[{ "id", "name", "url", "username", "has_password": true, "headers": {...},
   "account_id": "7", "project_id": "", "global": false, "owner_id" }]
```
`account_id` / `project_id` — VictoriaLogs multitenancy header'lari (`AccountID`, `ProjectID`), ikkalasi ixtiyoriy.

### POST `/api/datasources` — yaratish (`url` majburiy; `global` faqat admin). Audit `datasource.create`.
### PUT `/api/datasources/{id}` — owner/admin. `username` bo'sh bo'lmasa va `password` bo'sh bo'lsa mavjud parol saqlanadi; `username` bo'sh → parol o'chadi. Audit `datasource.update`.
### DELETE `/api/datasources/{id}` — owner/admin. Audit `datasource.delete`.
### POST `/api/datasources/test`
Body — Datasource (saqlanmagan bo'lishi mumkin). `id` berilsa va parol bo'sh bo'lsa saqlangan parol ishlatiladi. `/select/logsql/query?query=*&limit=1` ga 10s timeout.
```json
→ { "ok": true, "status": 200, "latency_ms": 12 }   |   { "ok": false, "latency_ms": 10003, "error": "..." }
```

## Saved Queries — `GET/POST /api/queries`, `PUT/DELETE /api/queries/{id}` (owner/admin; `global` faqat admin). Audit `query.create` / `query.delete`.

## Log viewer (VictoriaLogs proxy'lari)

Hammasida `datasource_id` majburiy (foydalanuvchiga ko'rinadigan bo'lishi shart), `query` (default `*`), `start`/`end` ixtiyoriy RFC3339 UTC.

| Endpoint | Qo'shimcha | VictoriaLogs | Javob |
|---|---|---|---|
| `GET /api/query` | `limit` (≤1000, default 50), `offset`, `sort` asc/desc | `/select/logsql/query` (`limit+offset`) | `{logs, total, has_more, limit, offset}`; `offset=0` → tarixga yoziladi |
| `GET /api/fields` | — | `/select/logsql/field_names` | `{values:[{value,hits}]}` |
| `GET /api/field_values` | `field` (majburiy), `limit` (20) | `/select/logsql/field_values` | `{values}` |
| `GET /api/streams` | `limit` (50) | `/select/logsql/streams` | `{values}` |
| `GET /api/hits` | `step` (bo'sh → ~60 bucket: oraliq/60 ni "chiroyli" qadamga yaxlitlab, masalan 6h → `600s`) | `/select/logsql/hits` | `{timestamps[], values[], total, step}` |
| `GET /api/tail` | — | `/select/logsql/tail` | `application/x-ndjson` chunked oqim, har qatorda flush; client uzilsa upstream ham yopiladi |

`sort` — server `\| sort by (_time)[ desc]` qo'shadi, query'da `\|` bo'lsa qo'shmaydi.

### GET `/api/history` → `[{query, datasource_id, at}]` (eng yangisi birinchi, 50 ta). DELETE — tozalash.

## Export

### POST `/api/export`
```json
{
  "query": "...", "datasource_id": "uuid",
  "start": "2026-05-21T05:00:00Z", "end": "2026-05-21T05:30:00Z",   // ixtiyoriy
  "sort_order": "asc",                 // asc | desc | ""
  "name": "prod-errors",               // fayl nomi prefiksi (ixtiyoriy)
  "format": "tar.gz",                  // tar.gz (default, bo'sh/noma'lum ham) | log.gz | log
  "content": "ndjson",                 // ndjson (default) | msg | csv
  "fields": ["_time","level"],         // ndjson: faqat shu maydonlar; csv: ustunlar (bo'sh → _time,_stream,_msg); msg: e'tiborsiz
  "archive_split_mb": 1024,            // faqat tar.gz: siqilgan hajm chegarasi → _volNNN.tar.gz; 0 = cheksiz
  "notify": true,                      // tugaganda egasiga xabar (profil manzillari + admin kanallari)
  "auto_share": true                   // tugaganda 7 kunlik, cheksiz share link (share_url)
}
→ 200 { "job_id": "job_1714000000000000000" }
```
Job `queued` holatida yaratiladi; `datasource_id` ko'rinmasa 404. Audit `export.start`.

**Fayl nomi:** `sanitizeFileName` → `<name>_<UTC ts>` + suffix: tar.gz → `.tar.gz`; log.gz → `.log.gz` / `.csv.gz`; log → `.log` / `.csv`.

**Dvigatel (worker/):** vaqt oynalari (≤2h bitta; ≤48h 1h; ≤14d 6h; aks holda 24h; ochiq boshlanish → earliest probe), `total_estimate` (hits), har oyna 3 marta retry (5xx/tarmoq) + rollback, `rawSink` (tarkib + qism chegaralari `export_chunk_mb`), paketlash (qismlar `<nom>_partNNN.<ext>`, volumelar).

### GET `/api/jobs`
Oddiy user faqat o'z joblarini, admin hammasini; `created_at` DESC.
```json
[{
  "id", "owner_id", "owner_name", "query", "datasource_id", "datasource_name",
  "status": "queued|running|done|failed",
  "file_name": "x_20260521-090655-123.tar.gz",     // birinchi fayl
  "files": [{ "name", "size", "parts" }],           // done'da; volumelar bo'lsa bir nechta
  "created_at", "started_at", "finished_at", "error",
  "lines", "size", "parts",
  "start", "end", "sort_order", "format", "content", "fields", "archive_split_mb",
  "total_estimate": 123456, "windows_total": 24, "windows_done": 7, "retries": 1,
  "schedule_id", "notify", "auto_share", "share_url"
}]
```
Progress (`lines`, `windows_done`, `retries`, `total_estimate`) faqat xotirada yangilanadi, holat o'zgarishi diskka yoziladi.

### DELETE `/api/jobs/{id}` — owner/admin; queued → navbatdan, running → cancel; fayllar + share'lar o'chadi. Audit `job.delete`.

### POST `/api/jobs/{id}/download-token` → `{ "token", "expires_in": 300 }`
### GET `/api/jobs/{id}/download?dt=<token>&file=<nom>`
`dt` — download token (typ=download, aynan shu job uchun; Bearer header ham ishlaydi). `file` — `files[]` dagi nom (default `file_name`). `http.ServeFile` (Range bor). Audit `job.download`. Eski `?token=<jwt>` **qabul qilinmaydi**.

### GET `/api/stats`
```json
{ "exports_bytes", "exports_files", "jobs": {"queued","running","done","failed"},
  "retention": {"days", "quota_gb"}, "worker": {"max_concurrent","running","queued"} }
```

## Share links (owner/admin)

### POST `/api/jobs/{id}/share` `{ "max_downloads": 5, "expires_at": "RFC3339" }` (0 / bo'sh = cheksiz; job done bo'lishi shart)
```json
→ 201 { "token", "job_id", "created_by", "created_at", "expires_at", "max_downloads", "download_count",
        "url": "<public_url yoki so'rov host>/dl/<token>",
        "files": [{ "name", "url": ".../dl/<token>", "size" }, { "name", "url": ".../dl/<token>/2", "size" }] }
```
1 share per job. `public_url` (admin settings) bo'lsa URL shundan. Audit `share.create` / `share.revoke`.
### GET `/api/jobs/{id}/share` — joriy share yoki 404. DELETE — bekor (fayl qolmaydi o'chmaydi).

### GET `/dl/{token}` va `/dl/{token}/{n}` (public)
`n` — 1-asosli fayl indeksi (volumelar). Range yo'q (`Accept-Ranges: none`), har so'rov = 1 yuklab olish. Muddat/limit → 410, yo'q → 404; limit yetganda share+job+fayllar o'chadi. Audit `share.download` (IP bilan, user yo'q). Fon: har 1 daqiqada muddati o'tganlar tozalanadi.

## Schedules

### GET `/api/schedules` — o'ziniki (admin: hammasi).
### POST `/api/schedules`, PUT `/api/schedules/{id}` (owner/admin)
```json
{ "name": "daily-errors", "query": "level:error", "datasource_id": "uuid",
  "cron": "5 0 * * *",            // 5 maydon (min hour dom month dow; *, n, a-b, */n, ro'yxat) yoki @hourly/@daily/@weekly/@monthly
  "lookback_minutes": 1440, "align": true,   // [now-lookback, now]; align → lookback'ka tekislangan (kechagi 00:00–24:00)
  "sort_order": "asc", "format": "tar.gz", "content": "ndjson", "fields": [], "archive_split_mb": 0,
  "notify": true, "auto_share": true, "enabled": true }
→ Schedule (+ owner_id, owner_name, created_at, next_run_at, last_run_at, last_job_id, last_error)
```
Noto'g'ri cron → 400. `enabled=false` → `next_run_at` null. Audit `schedule.create/update/delete`.
### DELETE `/api/schedules/{id}`; POST `/api/schedules/{id}/run` → `{ "job_id" }` (reja vaqtiga ta'sir qilmaydi). Audit `schedule.run`.
Scheduler har 30s: `next_run_at <= now` → export (`schedule_id`, nom = reja nomi), keyingi vaqt hisoblanadi; server o'chiq paytda o'tkazib yuborilgan run bitta marta qoplanadi.

## Users (admin) — `GET/POST /api/users`, `PUT/DELETE /api/users/{id}`
POST `{username, password, role, email}`; PUT `{role, password, email}` (bo'sh = o'zgarmaydi; LDAP userga parol qo'yilsa `auth_source` → `local`). Javoblarda `auth_source`. O'zini o'chirish 400. Audit `user.create/update/delete`.

## Audit (admin)
### GET `/api/audit?limit=100&offset=0&action=&user=` → `{ entries:[{time,user_id,user,action,target,ip,details}], total, limit, offset }` (eng yangisi birinchi; filtr substring).
### GET `/api/audit/download` — joriy `audit.jsonl`.
Amallar: `login.ok|fail|blocked`, `profile.update`, `password.change`, `user.*`, `datasource.*`, `query.create|delete`, `export.start`, `job.delete|download`, `share.create|revoke|download`, `schedule.*`, `settings.update`, `audit.download`.

## Settings (admin)
### GET `/api/settings`
```json
{ "retention": {"days": 30, "quota_gb": 0}, "worker": {"max_concurrent": 2}, "public_url": "",
  "notify": { "has_telegram_token": false, "webhook_url": "",
              "smtp": {"host","port","username","has_password","from","starttls"} },
  "export_chunk_mb": 100,
  "ldap": { "enabled": false, "url": "", "starttls": false, "insecure_skip_verify": false, "bind_dn": "", "has_bind_password": false,
            "base_dn": "", "user_filter": "", "username_attr": "", "email_attr": "", "admin_group_dn": "", "allowed_group_dn": "",
            "default_role": "user", "timeout_sec": 0 } }
```
### PUT `/api/settings` — qisman: `retention`, `worker` (1..64), `public_url`, `notify.telegram_bot_token` (bo'sh = o'zgarmaydi, `clear_telegram_token: true` = o'chirish), `notify.webhook_url`, `notify.smtp` (`password` bo'sh = o'zgarmaydi, `clear_password`). Worker uyg'otiladi. Audit `settings.update`.
### POST `/api/settings/notify/test` `{ "channel": "telegram|email|webhook" }` → `{ ok, error }` (qabul qiluvchi — joriy admin profili).
`PUT /api/settings` `ldap` bo'limi: yuqoridagi maydonlar + `bind_password` (bo'sh = o'zgarmaydi), `clear_bind_password`. Defaultlar: filtr `(&(objectClass=user)(sAMAccountName=%s))`, `username_attr` sAMAccountName, `email_attr` mail, timeout 10s.
### POST `/api/settings/ldap/test` `{ "ldap": {...forma}, "username": "", "password": "" }` → `{ ok, error }` yoki `{ ok: true, result: { dn, username, email, groups[], is_admin } }`. Username bo'sh → faqat ulanish + service bind + base DN; berilsa to'liq login sinovi. Bind paroli bo'sh bo'lsa saqlangani ishlatiladi.

## System (auth'siz)
- `GET /healthz`, `/readyz` → `{ status: "ok", version, uptime, worker: {running, queued} }`
- `GET /metrics` — Prometheus text: `logdownloader_build_info`, `uptime_seconds`, `exports_total{status}`, `export_lines_total`, `export_bytes_total`, `export_window_retries_total`, `jobs{status}`, `worker_running/queued/max_concurrent`, `exports_dir_bytes/files`, `schedules`, `audit_entries`, `http_requests_total{class}`, `notifications_total{status}`, `login_failures_total`, `login_blocked_total`, `go_goroutines`.
