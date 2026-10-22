# Data Model & Storage

Storage — disk ustida JSON fayllar (`encoding/json` + `os.WriteFile`), [backend/internal/store/](../backend/internal/store/) da bitta `sync.RWMutex` ostida (audit uchun alohida mutex). Getter'lar **nusxa** qaytaradi.

## Disk layout

```
<DataDir>/                  (default: /var/lib/logdownloader)
├── settings.json           # model.Settings { datasources[], retention, worker, notify, public_url }
├── users.json              # map[id]User (bcrypt + email/telegram/notify_on_done)
├── queries.json            # map[id]SavedQuery
├── jobs.json               # map[id]ExportJob (holatlar; progress yozilmaydi)
├── shares.json             # map[token]ShareLink
├── schedules.json          # map[id]Schedule
├── history.json            # map[userID][]HistoryEntry (50 ta)
├── audit.jsonl             # append-only; 20 MB da → audit-<ts>.jsonl (3 ta eski saqlanadi)
├── jwt_secret              # 32 bayt hex, 0600
└── exports/
    ├── <nom>_<UTC ts>.tar.gz             # ichida <nom>.log|csv yoki <nom>_partNNN.log|csv
    ├── <nom>_<UTC ts>_volNNN.tar.gz      # archive_split_mb bo'lsa (qismlar global raqamlanadi)
    ├── <nom>_<UTC ts>.log.gz | .csv.gz | .log | .csv
    └── *.tmp                             # running paytida xom oqim (recovery o'chiradi)
```

## Tip ta'riflari — [`model/models.go`](../backend/internal/model/models.go)

- **User**: `ID, Username, Password(bcrypt), Role, Email, TelegramChatID, NotifyOnDone`.
- **Datasource**: `ID, Name, URL, Username, Password(plain), Headers, AccountID, ProjectID, Global, OwnerID`. API javobi `datasourceView` — `Password` tashlanadi, `HasPassword` qo'shiladi.
- **Settings**: `Datasources[]`, `Retention{Days, QuotaGB}` (0 = o'chiq; birinchi ishga tushishda Days=30), `Worker{MaxConcurrent}` (default 2), `Notify{TelegramBotToken, SMTP{Host,Port,Username,Password,From,StartTLS}, WebhookURL}`, `PublicURL`.
- **SavedQuery**: `ID, Name, Query, DatasourceID?, Global, OwnerID`.
- **ExportJob**: `ID("job_<unixnano>"), OwnerID, OwnerName, Query, DatasourceID, DatasourceName, Status(queued|running|done|failed), FileName(birinchi fayl), Files[]{Name,Size,Parts}, CreatedAt, StartedAt, FinishedAt, Error, Lines, Start, End, SortOrder, Format(tar.gz|log.gz|log), Content(ndjson|msg|csv), Fields[], ArchiveSplitMB, Size(jami), Parts(jami), TotalEstimate, WindowsTotal, WindowsDone, Retries, ScheduleID, Notify, AutoShare, ShareURL`.
- **ShareLink**: `Token, JobID, CreatedBy, CreatedAt, ExpiresAt?, MaxDownloads(0=∞), DownloadCount`.
- **Schedule**: `ID, OwnerID, OwnerName, Name, Query, DatasourceID, Cron, LookbackMinutes, Align, SortOrder, Format, Content, Fields, ArchiveSplitMB, Notify, AutoShare, Enabled, CreatedAt, LastRunAt, LastJobID, LastError, NextRunAt`.
- **AuditEntry**: `Time, UserID, User, Action, Target, IP, Details`.
- **HistoryEntry**: `Query, DatasourceID, At`.

## Invariantlar

- **Getter'lar nusxa qaytaradi** (`GetSettings` chuqur: slice + Headers map; `GetDatasource`/`GetJob`/`GetShare*`/`GetSchedule` struct nusxa; ro'yxatlar `[]T` nusxa). `SaveSettings`/`SaveShare`/`SaveSchedule` ham nusxa saqlaydi. Handler store'ning ichki ma'lumotiga mutex'siz tegmaydi — ichki pointer qaytaruvchi metod qo'shmang (`-race` smoke testi buni ushlaydi).
- **Yozuvchi mutatsiya** — `UpdateJob(id, persist, fn)`, `UpdateSchedule(id, fn)`, `UpdateSettings(fn)`: mutex ostida `fn` chaqiriladi.
- **`Global` faqat admin'da `true`**; owner-based access: datasource/query filter (owner || global), job/schedule — `canAccessJob` / `loadScheduleForUser` (owner || admin), `canUseDatasource` (global || owner || admin).
- **Jobs persist siyosati:** `AddJob`, `DeleteJob`, `UpdateJob(persist=true)` (queued→running, done/failed, share_url), `purgeShareLocked`, `PruneExports` → `jobs.json`. Progress (`Lines`, `WindowsDone`, `Retries`, `TotalEstimate`) `persist=false`.
- **`RecoverJobs()` (startup):** running → failed (`server qayta ishga tushdi...`) + fayllar o'chadi; queued qoladi (worker `QueuedJobIDs` bilan tiklaydi); `exports/` dagi hech qaysi jobga tegishli bo'lmagan fayllar (orphan, `.tmp` ham) o'chadi; job'i yo'q/done bo'lmagan share'lar o'chadi.
- **`PruneExports(days, quota)`:** done/failed `CreatedAt < now-days` → o'chadi; `Σ job.Size > quota` → eng eski done/failed'lar o'chadi (running/queued tegilmaydi). Har 10 daqiqa + startup.
- **Share**: 1 share per job; `ConsumeShare(token, fileIndex)` atomik (tekshir → count++ → limit yetsa serve'dan keyin purge). Purge = share + job + fayllar.
- **Audit**: `Audit()` xatoni yutadi (asosiy ishni to'xtatmaydi); `ReadAudit` joriy faylni to'liq o'qiydi (20 MB chegara).
- **History**: dublikat query yuqoriga ko'chadi, 50 ta.
- **Worker HTTP — timeout'siz, context bilan.** `DoLongRunning(ctx)`; `Cancel(jobID)` navbatdan oladi yoki ctx'ni bekor qiladi. Bekor qilingan job fayllari (`.tmp`, final, `_vol*`) worker chiqishida o'chadi.
- **Sink rollback invarianti:** `mark()` bufferni flush qiladi → diskdagi hajm = mantiqiy offset; `rollback` = `bufW.Reset` + `Truncate(offset)` + `Seek` + tracker/lines restore. Shunda faqat joriy oyna tashlanadi (oldin flush bo'lmagani uchun oldingi oyna ma'lumoti yo'qolardi — tuzatilgan).
- **Volumelar**: faqat `tar.gz`; chegara siqilgan baytlar (`countingWriter` faylga tushganlar, bufio/gzip ichidagilarsiz) ≥ limit bo'lganda **qism tugagach** keyingi volume; birinchi volume oxirida `_vol001` ga rename (agar >1).
- **Store `saveFile` xatosi in-memory holatni qaytarmaydi** (atomar emas) — ma'lum trade-off.

## Migration / backward compat

Version field yo'q. Yangi maydonlar `omitempty`/nol qiymat bilan eski JSON'dan o'qiladi: eski `jobs.json`'siz o'rnatmada `exports/` orphan deb tozalanadi; eski jobda `Files` bo'sh → `FileName` ishlatiladi (`jobFile`, `ConsumeShare` fallback); `Content` bo'sh → ndjson; `Retention.Days` 0 va `settings.json` mavjud bo'lsa 0 (o'chiq) qoladi — faqat birinchi yaratilishda 30.
