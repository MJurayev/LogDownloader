# Frontend Flow

React 19 + TypeScript + Vite. Router: `react-router-dom` v7. State management library yo'q — local `useState`, token `localStorage`'da.

## Auth flow

1. `App.tsx`'da `localStorage.getItem("token")` borligini tekshiradi.
2. Yo'q bo'lsa: faqat `/login` route, boshqasi `<Navigate to="/login" />`.
3. `LoginPage`:
   - `POST /api/login` → response'dan `token`, `user_id`, `username`, `role` `localStorage`'ga yoziladi.
   - `onLogin()` callback `setAuthed(true)` qiladi va `/`'ga yo'naltiradi.
4. Layout sidebar'da `user` props orqali username va role chiqadi; admin bo'lsa `Users` link qo'shiladi.
5. Logout: `localStorage.clear()` + `setAuthed(false)`. Token serverda invalidate qilinmaydi (stateless JWT).
6. **401 handling:** [api.ts:17](../frontend/src/api.ts#L17)'da har `authFetch` 401 qaytarsa `localStorage.removeItem("token")` + `window.location.href = "/login"`.

## Sahifalar

### LogViewerPage
Eng katta sahifa. Ikki ustun: chap saved queries paneli, o'ng asosiy viewer.

- `useEffect`'da `getDatasources()` va `getQueries()` paralel chaqiradi; birinchi datasource auto-tanlanadi.
- Search → `api.queryLogs(query, dsId, 50, page*50)`. Natija jadval, `_time/_msg/level/service/host` priority sortda chiqadi, qolgan har bir key qo'shimcha ustun.
- Qator bosilsa `expandedRow` orqali JSON detail ochiladi.
- "Save Query" modal — global checkbox faqat admin'da ko'rinadi.
- "Export" tugmasi `api.startExport(...)` chaqirib `/jobs`'ga navigate qiladi.

### JobsPage
`useEffect`'da `setInterval(loadJobs, 2000)` — har 2 soniyada jobs ro'yxati yangilanadi. Done bo'lganda "Yuklab olish" link `api.downloadJob(id)` URL'iga ishora qiladi (anchor tag, `download` attr).

> Yuklab olish anchor'da JWT header bo'lmaydi — bu backend talab qilmagani uchun ishlaydi: `/api/jobs/{id}/download` ham `auth.Middleware` ichidan o'tadi va token kerak. Lekin browser anchor'ida header qo'shilmaydi. Hozir bu **ishlamasligi kerak** — ehtimol token cookie yoki query param orqali yuborilmasa, 401 qaytaradi. Test qilib ko'ring.

### SettingsPage
- Datasource list expand/collapse.
- "Datasource qo'shish" form: name, URL, basic auth, global (faqat admin'da chekbox).
- Edit yo'q — faqat create/delete.
- `dsHeaders` state mavjud datasource'ning header'larini map → array qilib saqlaydi, lekin UI'da hozir faqat read-only ko'rsatiladi.

### UsersPage
Faqat admin route'i (`App.tsx`'da `user?.role === "admin"` shartida render). Server ham `RequireAdmin` middleware bilan tekshiradi.

### ExportPage
LogViewer'dagi Export tugmasi yetarli, lekin alohida sahifa ham bor. Faqat datasource selector + query + Export tugmasi.

### LoginPage / Layout
Quyi ahamiyatda; default sidebar nav.

## API contract bilan farqlar

- `api.ts`'da `Datasource.password` JSON'ga qaytariladi (server `omitempty` qilmagan — model'da `Password string \`json:"password,omitempty"\``, ya'ni bo'sh string'da omit). Yangi yaratilgan datasource response'ida password chiqadi — frontend uni react state'da saqlaydi.
- `SavedQuery.datasource_id` ixtiyoriy — `optional` qilingan TypeScript'da.
- `ExportJob.error` `omitempty` — frontend `error?: string`.

## Download flow

`/api/jobs/{id}/download` JWT auth talab qiladi, lekin `<a href download>` Authorization header'ni yubora olmaydi. Shu sababli `api.downloadJob(id)` endi async funksiya: `authFetch` orqali blob oladi, `Content-Disposition`'dan filename'ni ajratib oladi va dasturiy click bilan yuklab oladi. Katta exportlar (> 1-2 GB) bu yondashuvda butunlay xotirada bufferlanadi — kelajakda muammoga aylansa, server tomondan signed short-lived URL pattern'iga o'tish kerak.

## Ma'lum quirks

- [LogViewerPage.tsx:260-287](../frontend/src/pages/LogViewerPage.tsx#L260) — `<>` fragment ichida ikkita `<tr>` qaytariladi. React 19'da bu OK, lekin `key` faqat birinchi `<tr>`'da; React warning bo'lishi mumkin.
- Saved query yaratganda `setSavedQueries`'ni darhol yangilamasdan `loadSavedQueries()` chaqiriladi — 1s delay'dan keyin modal yopiladi. UX'da kichkina laggy his etiladi.
