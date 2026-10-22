# Frontend Flow

React 19 + TypeScript + Vite. Router: `react-router-dom` v7. State: lokal `useState`, token `localStorage`, til `LangContext`. Kutubxonalar: faqat `react-datepicker` (chart yo'q — inline SVG).

## i18n (uz / ru)

[`src/i18n.tsx`](../frontend/src/i18n.tsx): `uz` lug'at (default) va `ru: Record<TKey, string>`; `LangProvider` → `useT()` → `{ t, lang, setLang }`; `t("key", { n })` placeholder. Til `localStorage("lang")`, `<html lang>`. Logout til/theme'ni o'chirmaydi. Almashtirgich sidebar va login'da. Yordamchilar: `formatBytes`, `formatDuration`. Qoida: JSX'da literal matn yo'q; ma'lumot qiymatlari (fayl nomi, status kodi, cron) tarjima qilinmaydi.

## Auth flow

1. `App.tsx` token bor-yo'qligiga qarab `LoginPage` yoki `Layout`+route'lar.
2. Login: `POST /api/login` → token + user localStorage'ga. 429 → `loginBlocked` matni.
3. `authFetch` 401 → token o'chadi, `/login`. Server userni har so'rovda tekshiradi.
4. Download: `api.downloadJob(id, file)` → `POST /download-token` → `<a href="/api/jobs/{id}/download?dt=…&file=…" download>` click (native streaming). Sessiya JWT URL'ga tushmaydi.

## Route'lar

| Path | Sahifa |
|------|--------|
| `/` | LogViewerPage |
| `/jobs` | JobsPage |
| `/schedules` | SchedulesPage |
| `/settings` | SettingsPage (admin bo'lsa server sozlamalari ham) |
| `/profile` | ProfilePage |
| `/users`, `/audit` | admin |
| `/export`, `*` | → `/` |

## Sahifalar

### LogViewerPage
- **Chap panel** (`lv-tabs`): Saqlangan (CRUD, G badge) / Maydonlar (`FieldsPanel`: `/api/fields` + `/api/streams`, maydon → `/api/field_values`, qiymat → `addFilterToQuery` → `AND field:"value"` filtr qismiga, pipe'dan oldin; avtomatik Search) / Tarix (`/api/history`, bosilsa query + datasource).
- **Vaqt oralig'i** (`react-datepicker`, presetlar), **datasource**, **query** (`query-wrap`: textarea + `suggest-list` autocomplete — maydon nomlari `/api/fields?query=*` dan, kursor oldidagi so'z bo'yicha, Tab/Enter/↑↓/Esc), **Search**, **Live tail** (`api.tail` NDJSON oqim, AbortController; yangi qatorlar tepaga, 1000 ta; tail paytida gistogramma/pagination yo'q).
- **Search** → `queryLogs` + parallel `getHits` (page 0) → gistogramma (`Histogram`: SVG, hover qiymat, ustun bosilsa `zoomTo` → oraliq = bucket). `offset=0` da `fieldsRefresh++` (panel yangilanadi) va URL sync (`?q&ds&from&to&sort`, replace). URL'da `q` bo'lsa datasource yuklangach avtomatik qidiradi.
- **Jadval**: `allColumns` (priority + qolganlar + user tartibi) − yashirilganlar; `cols-popover` (checkbox, ◀▶, reset) → `localStorage lv_cols_<ds>`; qator detail'da "JSON nusxalash".
- **Export qatori**: fayl nomi, format `<select>` (default `.tar.gz`), "Bo'laklash: Avtomatik" chip (`.muted` agar tar.gz emas), Export → `/jobs`. **Export sozlamalari** `<details>`: tarkib (ndjson/msg/csv), maydonlar (datalist bilan), arxiv hajmi (faqat tar.gz), "Tugaganda xabar" (profilda manzil bo'lmasa disabled + hint — `api.me()` mount'da), "Auto-share".

### JobsPage
2s polling: `getJobs` + `getStats` (stats panel: disk/kvota, saqlash kuni, navbat). Bo'limlar: Running (progress bar — `progressOf`: oynalar va/yoki `lines/total_estimate`, indeterminate agar taxmin yo'q; meta: qatorlar, %, oynalar, o'tgan vaqt, ETA `etaSeconds`, retry) → Navbatda → Done → Failed. Chip'lar: datasource, egasi (boshqa user), Reja, format, content, qismlar, fayllar. Done: Share + Yuklab olish (ko'p faylli bo'lsa har fayl tugmasi `job-files`), `share_url` (auto-share) link. Share modal: ko'p faylli bo'lsa har fayl URL'i + copy.

### SchedulesPage
10s polling. Karta: enabled badge, nom, datasource, egasi, cron, lookback (`1d ⌁` = align), format/content chip'lar; "Hozir ishga tushirish" (`runSchedule` → job id), yoqish/o'chirish (to'liq PUT), tahrirlash, o'chirish; keyingi/oxirgi run, oxirgi job (→ /jobs), xato. Modal forma: nom, datasource, query, cron preset (har soat / har kuni 00:05 / har dushanba / custom input), lookback preset, align, format/content/fields/arxiv hajmi, notify (profil manzili bo'lsa), auto-share, enabled.

### SettingsPage
Datasource karta: tenant chip (`account:project`), inline forma (nom, URL, basic auth — parol placeholder "Saqlangan", **tenant AccountID/ProjectID**, header'lar, global, **"Ulanishni tekshirish"** → `/api/datasources/test` natija). Admin bo'lsa pastda **Server sozlamalari**: saqlash kuni, kvota GB, bir vaqtda exportlar, chunk info, tashqi manzil; bildirishnoma kanallari — Telegram token (saqlangan placeholder, Tozalash, Test), webhook (Test), SMTP (host/port/from/user/parol/STARTTLS, Tozalash parol, Test). Saqlash → `updateSettings` (bo'sh sirlar = o'zgarmaydi; clear flaglari).

### ProfilePage
`api.me()` → email, Telegram chat ID (hint), "har export tugaganda xabar"; parol o'zgartirish (joriy + yangi ≥ 6, xatolar tarjima).

### AuditPage (admin)
Filtr (amal — datalist, user), Qidirish/Yangilash/JSONL yuklab olish, jami; jadval (vaqt, user, amal chip rangli prefiks bo'yicha, obyekt, IP, tafsilot), 100 talik pagination. `fail`/`blocked` qatorlari `log-warn`.

### UsersPage (admin)
Yaratish (username, parol, email, rol), karta (rol badge, email, TG chip), tahrirlash (rol, email, parol).

### LoginPage / Layout
Login kartasida UZ/RU; sidebar: Log Viewer, Jobs, Rejalar, Settings, (Users, Audit — admin), Profil + UZ/RU + tema + Chiqish.

## API contract eslatmalari

- `Datasource.password` faqat yuborishda; `has_password` javobda; `account_id`/`project_id` ixtiyoriy.
- `ExportJob.status` `"queued"` ham; `files[]` done'da; `share_url` auto-share'da.
- `api.startExport(ExportOptions)` — obyekt; `format` default `tar.gz`, `content` default `ndjson`.
- `ShareLink.files[]` — har fayl URL'i (`/dl/<token>/<n>`).
- `api.tail` — `fetch` + `ReadableStream`, `authHeaders()` bilan (authFetch emas — 401 redirect oqimni buzmasin).

## Ma'lum quirks

- Tail paytida `/api/tail` VictoriaLogs'ning tail cheklovlariga bo'ysunadi (ba'zi pipe'lar ishlamaydi) — xato banner'da.
- Autocomplete faqat maydon nomlari; qiymatlar panel orqali.
- Ustun tartibi datasource bo'yicha saqlanadi; yangi maydonlar oxiriga qo'shiladi.
- Jobs 2s polling — ko'p ochiq tab'larda server yuki (kichik).
