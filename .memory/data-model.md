# Data Model & Storage

Storage — disk ustida JSON fayllar. Library yo'q, faqat `encoding/json` + `os.WriteFile`. Hammasi [store.go](../backend/internal/store/store.go)'da `sync.RWMutex` ostida.

## Disk layout

```
<DataDir>/                  (default: /var/lib/logdownloader)
├── settings.json           # model.Settings  { datasources: [...] }
├── queries.json            # map[id]SavedQuery
├── users.json              # map[id]User (parol bcrypt hash bilan!)
└── exports/
    └── export_job_*.log    # raw NDJSON, VictoriaLogs'dan stream qilingan
```

Eslatma: `settings.json` array shape'da (Settings struct ichida slice), `queries.json` va `users.json` map shape'da. Bu nomuvofiqlik tarixiy — yangi store loadda farqlanadi.

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
    Password string             // plain (!) — disk'da shifrlanmagan
    Headers  map[string]string  // custom HTTP headers
    Global   bool               // true bo'lsa hammaga ko'rinadi (faqat admin yarata oladi)
    OwnerID  string
}
```

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

### ExportJob (in-memory only, restart'da yo'qoladi)
```go
type ExportJob struct {
    ID             string     // "job_<unixnano>"
    Query          string
    DatasourceID   string
    DatasourceName string     // snapshot
    Status         JobStatus  // "running" | "done" | "failed"
    FileName       string     // "export_<id>.log"
    CreatedAt      time.Time
    Error          string
    Lines          int64      // har 1000'da incremental update
}
```

## Invariantlar

- **Settings file** doim Settings struct shape'da. Datasource'lar slice ichida, ID bo'yicha indexing yo'q.
- **Map keys = ID** (queries, users). `SaveQuery(q)` doim `s.queries[q.ID] = q` qiladi — eskisini overwrite qilish ham shu yo'l bilan.
- **`Global` flag faqat admin'da `true` bo'lib qoladi.** Handler oddiy user kelganda majburan `false`'ga qaytaradi (handler.go:191, 245).
- **Owner-based access:** `GetDatasourcesForUser` va `GetQueriesForUser` filter qiladi (owner==user || global). Delete handler ham shu qoidaga bo'ysunadi, admin esa hammaga ega.
- **JobID format** — `fmt.Sprintf("job_%d", time.Now().UnixNano())`. Bu ID file name'ga ham kiradi. Bir nano-second ichida ikkita export bo'lsa to'qnashishi mumkin (amaliyotda kam uchraydi).
- **Job worker'da progress update** har 1000 qatorda store'ni `UpdateJobStatus(... running, "", lines)` deb chaqiradi — bu in-memory'gina, faylga yozilmaydi (chunki jobs persist qilinmaydi).
- **Worker HTTP — timeout'siz.** `vlclient.DoLongRunning` (worker.go ichida) dedicated `http.Client{Timeout: 0}` + `Transport{IdleConnTimeout: 0, ResponseHeaderTimeout: 0}` ishlatadi. Faqat dial/TLS handshake'da 30s limit (ulanmagan endpoint'da abadiy osmaslik uchun). Soatlar davom etadigan exportlar uchun mo'ljallangan.
- **DeleteJob** ham file'ni va in-memory entry'ni o'chiradi. Worker hali yozayotgan bo'lsa — race. Hozircha cancellation yo'q.

## Migration / backward compat

Hech qanday version field yo'q. Schema'ni o'zgartirsangiz mavjud installlardagi JSON o'qishni sindirishi mumkin — bunga e'tibor bering. Misol uchun yangi majburiy field qo'shsangiz, eski fayllarda `null/""` bo'lib qoladi.
