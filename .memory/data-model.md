# Data Model & Storage

Storage — disk ustida JSON fayllar. Library yo'q, faqat `encoding/json` + `os.WriteFile`. Hammasi [store.go](../backend/internal/store/store.go)'da `sync.RWMutex` ostida.

## Disk layout

```
<DataDir>/                  (default: /var/lib/logdownloader)
├── settings.json           # model.Settings  { datasources: [...] }
├── queries.json            # map[id]SavedQuery
├── users.json              # map[id]User (parol bcrypt hash bilan!)
├── jobs.json               # map[id]ExportJob (yakuniy holatlar; progress yozilmaydi)
├── shares.json             # map[token]ShareLink
├── jwt_secret              # 32 bayt hex, 0600 (auto-generate)
└── exports/
    └── <nom>_<UTC ts>.tar.gz   # ichida <nom>.log yoki <nom>_partNNN.log qismlar (avtomatik bo'laklash)
```

Eslatma: `settings.json` array shape'da (Settings struct ichida slice), qolganlari map shape'da. Bu nomuvofiqlik tarixiy.

## Tip ta'riflari

[`backend/internal/model/models.go`](../backend/internal/model/models.go) — to'liq ta'rif.

### User
```go
type User struct {
    ID       string  // uuid
    Username string  // unique
    Password string  // bcrypt hash; JSON response'da `omitempty`
    Role     Role    // "admin" | "user"
}
```

### Datasource
```go
type Datasource struct {
    ID       string
    Name     string
    URL      string             // VictoriaLogs base (e.g. http://...:9428)
    Username string             // basic auth
    Password string             // plain (!) — disk'da shifrlanmagan; API javobida qaytmaydi
    Headers  map[string]string  // custom HTTP headers
    Global   bool               // true bo'lsa hammaga ko'rinadi (faqat admin yarata oladi)
    OwnerID  string
}
```
API javobi `handler.datasourceView` — `Password` tashlab, `HasPassword bool` qo'shadi.

### SavedQuery
```go
type SavedQuery struct {
    ID           string
    Name         string
    Query        string
    DatasourceID string  // ixtiyoriy
    Global       bool
    OwnerID      string
}
```

### ExportJob (jobs.json'da persist)
```go
type ExportJob struct {
    ID             string     // "job_<unixnano>"
    OwnerID        string     // kim boshlagan
    OwnerName      string     // username snapshot
    Query          string
    DatasourceID   string
    DatasourceName string     // snapshot
    Status         JobStatus  // "running" | "done" | "failed"
    FileName       string     // "<nom>_<UTC ts>.tar.gz"
    CreatedAt      time.Time
    Error          string
    Lines          int64      // har 1000'da incremental update (faqat xotirada)
    Start, End     string     // RFC3339 UTC, ixtiyoriy
    SortOrder      string
    Format         string     // "tar.gz" (default) | "log.gz" | "log" — StartExport'da normalizatsiya
    Size           int64      // yakuniy fayl hajmi
    Parts          int        // .tar.gz ichidagi qismlar soni (omitempty); log/log.gz = 1
}
```

### ShareLink (shares.json)
```go
type ShareLink struct {
    Token         string     // URL-safe base64, 16 bayt
    JobID         string
    CreatedBy     string
    CreatedAt     time.Time
    ExpiresAt     *time.Time // nil = cheksiz
    MaxDownloads  int        // 0 = cheksiz
    DownloadCount int
}
```

## Invariantlar

- **Getter'lar nusxa qaytaradi.** `GetSettings`/`GetDatasourcesForUser` — chuqur nusxa (slice + Headers map), `GetDatasource`/`GetJob`/`GetShare*` — struct nusxasiga pointer, `GetJobsForUser` — `[]ExportJob` nusxalar. `SaveSettings` ham nusxa saqlaydi. Shu sababli handler store'ning ichki ma'lumotiga mutex'siz tegmaydi (oldin `settings.Datasources[i] = input` real data race edi). Store'ga ichki pointer qaytaruvchi metod qo'shmang.
- **Settings file** doim Settings struct shape'da. Datasource'lar slice ichida, ID bo'yicha indexing yo'q.
- **Map keys = ID** (queries, users, jobs); shares — token.
- **`Global` flag faqat admin'da `true` bo'lib qoladi.** Handler oddiy user kelganda majburan `false`'ga qaytaradi.
- **Owner-based access:** datasource/query — `GetDatasourcesForUser`/`GetQueriesForUser` filter (owner==user || global); job — `GetJobsForUser(ownerID, all)` (admin `all=true`), yozuvchi handlerlar `canAccessJob`.
- **JobID format** — `fmt.Sprintf("job_%d", time.Now().UnixNano())`. Bir nano-second ichida ikkita export bo'lsa to'qnashishi mumkin (amaliyotda kam uchraydi).
- **Jobs persist siyosati:** `AddJob`, `DeleteJob`, `UpdateJobStatus(done|failed)`, `purgeShareLocked` → `jobs.json`. `UpdateJobStatus(running, lines)` (progress) va `UpdateJobSize` diskka yozmaydi — yakuniy `done` yozuvi ularni qamrab oladi.
- **`RecoverJobs()` (startup, main.go):** running qolgan joblar → failed (`"server qayta ishga tushdi, export uzildi"`), ularning fayli + `.tmp` o'chadi; `exports/` dagi hech qaysi jobga tegishli bo'lmagan fayllar (orphan) o'chadi; job'i yo'q yoki done bo'lmagan share'lar o'chadi.
- **Worker HTTP — timeout'siz, lekin context bilan.** `vlclient.DoLongRunning(ctx, ...)` dedicated `http.Client{Timeout: 0}`; dial/TLS 30s. `worker.Cancel(jobID)` ctx'ni bekor qiladi — `DeleteJob` oldidan handler chaqiradi; worker chiqishda ctx bekor bo'lgan bo'lsa chala faylni o'chiradi.
- **Store `saveFile` xatosi in-memory holatni qaytarmaydi** (atomar emas) — ma'lum trade-off.
- **Avtomatik bo'laklash faqat `tar.gz`'da.** `worker.chunkBytes` (config `export_chunk_mb`, 0 = o'chiq). `UpdateJobResult(id, size, parts)` yakunda; `Parts` persist `done` yozuvi bilan.

## Migration / backward compat

Hech qanday version field yo'q. Schema'ni o'zgartirsangiz mavjud installlardagi JSON o'qishni sindirishi mumkin. Eski (`jobs.json`'siz) o'rnatmada birinchi startup `exports/` dagi barcha fayllarni orphan deb o'chiradi — joblar avval xotirada edi, baribir yuklab bo'lmasdi. `owner_id`'siz eski job yozuvi bo'lsa (bo'lishi mumkin emas, lekin) faqat admin ko'radi.
