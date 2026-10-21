# Frontend Flow

React 19 + TypeScript + Vite. Router: `react-router-dom` v7. State management library yo'q — local `useState`, token `localStorage`'da, til `LangContext`.

## i18n (uz / ru)

[`src/i18n.tsx`](../frontend/src/i18n.tsx):
- `uz` lug'at (default) va `ru: Record<TKey, string>` — kalitlar bir xil bo'lishi TypeScript bilan majburlanadi.
- `LangProvider` (`main.tsx`'da `BrowserRouter`'dan tashqarida) → `useT()` → `{ t, lang, setLang }`.
- `t("key", { n: 5 })` — `{n}` placeholder'larni almashtiradi.
- Til `localStorage("lang")` da, `<html lang>` ham yangilanadi. Logout til/theme'ni o'chirmaydi (faqat `token` + `user`).
- Almashtirgich: sidebar (`Layout.tsx`) va login kartasi (`LoginPage.tsx`) — `UZ / RU`, faol til `.lang-opt.active`.

Qoida: JSX'da literal matn yozilmaydi; har bir yangi matn ikkala lug'atga qo'shiladi. Datasource nomi, fayl nomi, LogsQL, status kodlari kabi ma'lumot qiymatlari tarjima qilinmaydi.

## Auth flow

1. `App.tsx`'da `localStorage.getItem("token")` borligini tekshiradi.
2. Yo'q bo'lsa: faqat `/login` route, boshqasi `<Navigate to="/login" />`.
3. `LoginPage`:
   - `POST /api/login` → response'dan `token`, `user_id`, `username`, `role` `localStorage`'ga yoziladi.
   - `onLogin()` callback `setAuthed(true)` qiladi va `/`'ga yo'naltiradi.
4. Layout sidebar'da `user` props orqali username va role chiqadi; admin bo'lsa `Users` link qo'shiladi.
5. Logout: `token` + `user` o'chadi, `setAuthed(false)`. Token serverda invalidate qilinmaydi (stateless JWT), lekin server har so'rovda userni store'dan tekshiradi.
6. **401 handling:** `api.ts` `authFetch` 401 qaytarsa `localStorage.removeItem("token")` + `window.location.href = "/login"`. O'chirilgan user yoki eskirgan token shu yo'l bilan login'ga tushadi.

## Route'lar

| Path | Sahifa |
|------|--------|
| `/` | LogViewerPage |
| `/jobs` | JobsPage |
| `/settings` | SettingsPage |
| `/users` | UsersPage (faqat admin) |
| `/export` | → `/` redirect (Export sahifasi olib tashlangan, funksiyasi Log Viewer'da) |
| `*` | → `/` |

## Sahifalar

### LogViewerPage
Eng katta sahifa. Ikki ustun: chap saved queries paneli, o'ng asosiy viewer.

- `useEffect`'da `getQueries()` va `getDatasources()` chaqiradi; birinchi datasource auto-tanlanadi.
- Vaqt oralig'i (`react-datepicker`, preset 5m/1h/6h/24h/Hammasi), brauzer TZ hint.
- Search → `api.queryLogs(query, dsId, 50, page*50, start, end, sort)`. Natija jadval, `_time/_msg/level/service/host` priority sortda chiqadi, qolgan har bir key qo'shimcha ustun. `_time` ustuni sort tugmasi.
- Qator bosilsa `expandedRow` orqali JSON detail ochiladi. Qatorlar `<Fragment key={i}>` ichida (ikki `<tr>`), key warning yo'q.
- "Save Query" modal — global checkbox faqat admin'da ko'rinadi. Saved query edit modal.
- **Export:** fayl nomi input + arxiv formati `<select>` (default `.tar.gz`, `DEFAULT_EXPORT_FORMAT`) + qulflangan "Bo'laklash: Avtomatik" chip (`.export-locked-chip`; format `.tar.gz` bo'lmasa `.muted` + tooltip "faqat .tar.gz") + Export tugmasi. Bo'laklash tanlovi yo'q — server hal qiladi. `api.startExport(...)` → `/jobs`'ga navigate. Xato bo'lsa `error-banner`.

### JobsPage
`useEffect`'da `setInterval(loadJobs, 2000)` — har 2 soniyada jobs ro'yxati yangilanadi (server oddiy user uchun faqat o'z joblarini qaytaradi). Running / Done / Failed bo'limlari. Job kartasida: status badge (tarjima qilinadi), datasource nomi, **egasi chip'i** (`owner_name` joriy user'dan farq qilsa — admin boshqalarnikini ko'rganda), qator soni, hajm, format chip, **qismlar chip'i** (`parts > 1` bo'lsa, "N qism").

Done bo'lganda "Share" (modal: limit + muddat, mavjud link + copy + revoke) va "Yuklab olish" (`api.downloadJob`).

### SettingsPage
- Datasource list expand/collapse → inline tahrirlash formasi (name, URL, basic auth, headers, global — admin'da).
- Server parolni qaytarmaydi: forma `password: ""` + `hasPassword` (`has_password`). Placeholder "Saqlangan (o'zgartirish uchun yangisini yozing)". Bo'sh qoldirilsa server eski parolni saqlaydi; username tozalansa parol ham o'chadi.
- Create / update / delete.

### UsersPage
Faqat admin route'i (`App.tsx`'da `user?.role === "admin"` shartida render). Server ham `RequireAdmin` middleware bilan tekshiradi. Edit modal: rol + yangi parol.

### LoginPage / Layout
Login kartasining o'ng yuqori burchagida UZ/RU tugmasi. Sidebar: nav + UZ/RU + theme toggle + Chiqish.

## API contract bilan farqlar

- `Datasource.password` — faqat yuborishda (`has_password` javobda). `SavedQuery.datasource_id` ixtiyoriy. `ExportJob.error` `omitempty`, `owner_name` ixtiyoriy.
- `api.startExport` `format` default `"tar.gz"`; `!r.ok` bo'lsa reject (matn bilan).

## Download flow

`/api/jobs/{id}/download` JWT auth talab qiladi, lekin `<a href download>` Authorization header'ni yubora olmaydi. Shu sababli ikki tomonlama yechim:

1. **Backend** `auth.Middleware`'da: agar `Authorization` header bo'sh va URL `/download` bilan tugasa, `?token=` query param'dan JWT olinadi (boshqa endpointlarda emas).
2. **Frontend** `api.downloadJob(id)` sinxron: `<a href="/api/jobs/{id}/download?token=<jwt>" download>` yaratadi va dasturiy click qiladi. Brauzer faylni xotirada bufferlamasdan to'g'ridan-to'g'ri diskka oqim qiladi (native streaming download) — multi-GB exportlar uchun ishlaydi.

**Xavfsizlik trade-off:** JWT URL'da paydo bo'ladi, ya'ni reverse-proxy access loglariga tushishi mumkin. Self-hosted ichki log vositasi uchun qabul qilingan.

## Ma'lum quirks

- `LogViewerPage` da saved query yaratganda `loadSavedQueries()` kutib, keyin 600ms'dan so'ng modal yopiladi.
- `JobsPage` 2s polling — ko'p ochiq tab'larda server yuki (kichik).
