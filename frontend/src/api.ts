const API = "/api";

function getToken(): string | null {
  return localStorage.getItem("token");
}

function authHeaders(): Record<string, string> {
  const token = getToken();
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  return headers;
}

async function authFetch(url: string, opts: RequestInit = {}): Promise<Response> {
  const headers = { ...authHeaders(), ...(opts.headers as Record<string, string> || {}) };
  const res = await fetch(url, { ...opts, headers });
  if (res.status === 401) {
    localStorage.removeItem("token");
    window.location.href = "/login";
  }
  return res;
}

// okJson — 2xx bo'lmasa server matni bilan reject.
async function okJson<T>(r: Response): Promise<T> {
  if (!r.ok) return Promise.reject(await r.text());
  return r.json();
}

export interface Datasource {
  id: string;
  name: string;
  url: string;
  username?: string;
  // Faqat yuborishda ishlatiladi; server GET javobida parolni qaytarmaydi,
  // o'rniga `has_password` beradi. Yangilashda bo'sh parol = o'zgartirilmasin.
  password?: string;
  has_password?: boolean;
  headers?: Record<string, string>;
  account_id?: string;
  project_id?: string;
  global: boolean;
  owner_id: string;
}

export type DatasourceInput = Omit<Datasource, "id" | "owner_id" | "has_password"> & { id?: string };

export interface TestResult {
  ok: boolean;
  status?: number;
  latency_ms: number;
  error?: string;
}

export interface SavedQuery {
  id: string;
  name: string;
  query: string;
  datasource_id?: string;
  global: boolean;
  owner_id: string;
}

export type ExportFormat = "log" | "log.gz" | "tar.gz";
export type ExportContent = "ndjson" | "msg" | "csv";
export type JobStatus = "queued" | "running" | "done" | "failed";

export interface ExportFile {
  name: string;
  size: number;
  parts: number;
}

export interface ExportJob {
  id: string;
  owner_id: string;
  owner_name?: string;
  query: string;
  datasource_id: string;
  datasource_name: string;
  status: JobStatus;
  file_name: string;
  created_at: string;
  error?: string;
  lines: number;
  start?: string;
  end?: string;
  sort_order?: string;
  format?: ExportFormat;
  content?: ExportContent;
  fields?: string[];
  archive_split_mb?: number;
  size: number;
  parts?: number;
  files?: ExportFile[];
  total_estimate?: number;
  windows_total?: number;
  windows_done?: number;
  retries?: number;
  started_at?: string;
  finished_at?: string;
  schedule_id?: string;
  notify?: boolean;
  auto_share?: boolean;
  share_url?: string;
}

export interface ExportOptions {
  query: string;
  datasourceId: string;
  start?: string;
  end?: string;
  sortOrder?: "asc" | "desc";
  name?: string;
  format?: ExportFormat;
  content?: ExportContent;
  fields?: string[];
  archiveSplitMB?: number;
  notify?: boolean;
  autoShare?: boolean;
}

export interface QueryResult {
  logs: Record<string, string>[];
  total: number;
  has_more: boolean;
  limit: number;
  offset: number;
}

export interface ValueHits {
  value: string;
  hits: number;
}

export interface HitsResult {
  timestamps: string[];
  values: number[];
  total: number;
  step: string;
}

export interface HistoryEntry {
  query: string;
  datasource_id?: string;
  at: string;
}

export interface AuthUser {
  user_id: string;
  username: string;
  role: "admin" | "user";
}

export interface Profile extends AuthUser {
  email?: string;
  telegram_chat_id?: string;
  notify_on_done: boolean;
}

export interface UserInfo {
  id: string;
  username: string;
  role: "admin" | "user";
  email?: string;
  telegram_chat_id?: string;
  notify_on_done?: boolean;
}

export interface ShareFile {
  name: string;
  url: string;
  size: number;
}

export interface ShareLink {
  token: string;
  job_id: string;
  created_by: string;
  created_at: string;
  expires_at: string | null;
  max_downloads: number;
  download_count: number;
  url: string;
  files: ShareFile[];
}

export interface Schedule {
  id: string;
  owner_id: string;
  owner_name?: string;
  name: string;
  query: string;
  datasource_id: string;
  cron: string;
  lookback_minutes: number;
  align: boolean;
  sort_order?: string;
  format?: ExportFormat;
  content?: ExportContent;
  fields?: string[];
  archive_split_mb?: number;
  notify: boolean;
  auto_share: boolean;
  enabled: boolean;
  created_at: string;
  last_run_at?: string;
  last_job_id?: string;
  last_error?: string;
  next_run_at?: string;
}

export type ScheduleInput = Omit<Schedule, "id" | "owner_id" | "owner_name" | "created_at" | "last_run_at" | "last_job_id" | "last_error" | "next_run_at">;

export interface AuditEntry {
  time: string;
  user_id?: string;
  user?: string;
  action: string;
  target?: string;
  ip?: string;
  details?: string;
}

export interface Stats {
  exports_bytes: number;
  exports_files: number;
  jobs: Record<JobStatus, number>;
  retention: { days: number; quota_gb: number };
  worker: { max_concurrent: number; running: number; queued: number };
}

export interface ServerSettings {
  retention: { days: number; quota_gb: number };
  worker: { max_concurrent: number };
  public_url: string;
  notify: {
    has_telegram_token: boolean;
    webhook_url: string;
    smtp: { host: string; port: number; username: string; has_password: boolean; from: string; starttls: boolean };
  };
  export_chunk_mb: number;
}

export interface ServerSettingsInput {
  retention?: { days: number; quota_gb: number };
  worker?: { max_concurrent: number };
  public_url?: string;
  notify?: {
    telegram_bot_token?: string;
    clear_telegram_token?: boolean;
    webhook_url?: string;
    smtp?: { host: string; port: number; username: string; password?: string; clear_password?: boolean; from: string; starttls: boolean };
  };
}

function timeParams(datasourceId: string, query: string, start?: string, end?: string): URLSearchParams {
  const params = new URLSearchParams({ query, datasource_id: datasourceId });
  if (start) params.set("start", start);
  if (end) params.set("end", end);
  return params;
}

export const api = {
  // Auth / profil
  login: async (username: string, password: string): Promise<AuthUser & { token: string }> => {
    const res = await fetch(`${API}/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 429) throw new Error("blocked");
    if (!res.ok) throw new Error("Invalid credentials");
    return res.json();
  },

  me: (): Promise<Profile> => authFetch(`${API}/me`).then((r) => okJson<Profile>(r)),

  updateMe: (p: { email?: string; telegram_chat_id?: string; notify_on_done?: boolean; current_password?: string; new_password?: string }): Promise<UserInfo> =>
    authFetch(`${API}/me`, { method: "PUT", body: JSON.stringify(p) }).then((r) => okJson<UserInfo>(r)),

  // Datasources
  getDatasources: (): Promise<Datasource[]> =>
    authFetch(`${API}/datasources`).then((r) => r.json()),

  createDatasource: (ds: DatasourceInput): Promise<Datasource> =>
    authFetch(`${API}/datasources`, { method: "POST", body: JSON.stringify(ds) }).then((r) => okJson<Datasource>(r)),

  updateDatasource: (id: string, ds: DatasourceInput): Promise<Datasource> =>
    authFetch(`${API}/datasources/${id}`, { method: "PUT", body: JSON.stringify(ds) }).then((r) => okJson<Datasource>(r)),

  deleteDatasource: (id: string): Promise<void> =>
    authFetch(`${API}/datasources/${id}`, { method: "DELETE" }).then(() => {}),

  testDatasource: (ds: DatasourceInput): Promise<TestResult> =>
    authFetch(`${API}/datasources/test`, { method: "POST", body: JSON.stringify(ds) }).then((r) => okJson<TestResult>(r)),

  // Queries
  getQueries: (): Promise<SavedQuery[]> =>
    authFetch(`${API}/queries`).then((r) => r.json()),

  createQuery: (name: string, query: string, datasourceId?: string, global?: boolean): Promise<SavedQuery> =>
    authFetch(`${API}/queries`, {
      method: "POST",
      body: JSON.stringify({ name, query, datasource_id: datasourceId, global: global || false }),
    }).then((r) => okJson<SavedQuery>(r)),

  updateQuery: (id: string, name: string, query: string, datasourceId?: string, global?: boolean): Promise<SavedQuery> =>
    authFetch(`${API}/queries/${id}`, {
      method: "PUT",
      body: JSON.stringify({ name, query, datasource_id: datasourceId, global: global || false }),
    }).then((r) => okJson<SavedQuery>(r)),

  deleteQuery: (id: string): Promise<void> =>
    authFetch(`${API}/queries/${id}`, { method: "DELETE" }).then(() => {}),

  // Log viewer
  queryLogs: (
    query: string,
    datasourceId: string,
    limit: number,
    offset: number,
    start?: string,
    end?: string,
    sortOrder?: "asc" | "desc",
  ): Promise<QueryResult> => {
    const params = timeParams(datasourceId, query, start, end);
    params.set("limit", String(limit));
    params.set("offset", String(offset));
    if (sortOrder) params.set("sort", sortOrder);
    return authFetch(`${API}/query?${params}`).then((r) => okJson<QueryResult>(r));
  },

  getFields: (datasourceId: string, query: string, start?: string, end?: string): Promise<ValueHits[]> =>
    authFetch(`${API}/fields?${timeParams(datasourceId, query, start, end)}`).then((r) => okJson<{ values: ValueHits[] }>(r)).then((d) => d.values),

  getFieldValues: (datasourceId: string, query: string, field: string, limit = 20, start?: string, end?: string): Promise<ValueHits[]> => {
    const params = timeParams(datasourceId, query, start, end);
    params.set("field", field);
    params.set("limit", String(limit));
    return authFetch(`${API}/field_values?${params}`).then((r) => okJson<{ values: ValueHits[] }>(r)).then((d) => d.values);
  },

  getStreams: (datasourceId: string, query: string, start?: string, end?: string, limit = 50): Promise<ValueHits[]> => {
    const params = timeParams(datasourceId, query, start, end);
    params.set("limit", String(limit));
    return authFetch(`${API}/streams?${params}`).then((r) => okJson<{ values: ValueHits[] }>(r)).then((d) => d.values);
  },

  getHits: (datasourceId: string, query: string, start?: string, end?: string, step?: string): Promise<HitsResult> => {
    const params = timeParams(datasourceId, query, start, end);
    if (step) params.set("step", step);
    return authFetch(`${API}/hits?${params}`).then((r) => okJson<HitsResult>(r));
  },

  // tail — NDJSON oqim; har qator uchun onLine. AbortController bilan to'xtatiladi.
  tail: async (datasourceId: string, query: string, onLine: (row: Record<string, string>) => void, signal: AbortSignal): Promise<void> => {
    const params = new URLSearchParams({ query, datasource_id: datasourceId });
    const res = await fetch(`${API}/tail?${params}`, { headers: authHeaders(), signal });
    if (!res.ok) throw new Error(await res.text());
    const reader = res.body!.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let idx: number;
      while ((idx = buf.indexOf("\n")) >= 0) {
        const line = buf.slice(0, idx).trim();
        buf = buf.slice(idx + 1);
        if (!line) continue;
        try {
          onLine(JSON.parse(line));
        } catch {
          /* skip malformed */
        }
      }
    }
  },

  getHistory: (): Promise<HistoryEntry[]> => authFetch(`${API}/history`).then((r) => r.json()),
  clearHistory: (): Promise<void> => authFetch(`${API}/history`, { method: "DELETE" }).then(() => {}),

  // Export & Jobs
  startExport: (o: ExportOptions): Promise<{ job_id: string }> =>
    authFetch(`${API}/export`, {
      method: "POST",
      body: JSON.stringify({
        query: o.query,
        datasource_id: o.datasourceId,
        start: o.start || "",
        end: o.end || "",
        sort_order: o.sortOrder || "",
        name: o.name || "",
        format: o.format || "tar.gz",
        content: o.content || "ndjson",
        fields: o.fields || [],
        archive_split_mb: o.archiveSplitMB || 0,
        notify: !!o.notify,
        auto_share: !!o.autoShare,
      }),
    }).then((r) => okJson<{ job_id: string }>(r)),

  getJobs: (): Promise<ExportJob[]> =>
    authFetch(`${API}/jobs`).then((r) => r.json()),

  deleteJob: (id: string): Promise<void> =>
    authFetch(`${API}/jobs/${id}`, { method: "DELETE" }).then(() => {}),

  getStats: (): Promise<Stats> => authFetch(`${API}/stats`).then((r) => okJson<Stats>(r)),

  // downloadJob — qisqa muddatli, shu job'ga bog'langan token olib brauzerning
  // native streaming download'ini ishga tushiradi (fayl xotiraga yuklanmaydi).
  downloadJob: async (id: string, fileName?: string): Promise<void> => {
    const res = await authFetch(`${API}/jobs/${id}/download-token`, { method: "POST" });
    if (!res.ok) throw new Error(await res.text());
    const { token } = await res.json();
    const params = new URLSearchParams({ dt: token });
    if (fileName) params.set("file", fileName);
    const a = document.createElement("a");
    a.href = `${API}/jobs/${id}/download?${params}`;
    a.download = fileName || `export_${id}.tar.gz`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  },

  // Share links
  createShare: (jobId: string, opts: { maxDownloads?: number; expiresAt?: string | null }): Promise<ShareLink> =>
    authFetch(`${API}/jobs/${jobId}/share`, {
      method: "POST",
      body: JSON.stringify({ max_downloads: opts.maxDownloads ?? 0, expires_at: opts.expiresAt ?? "" }),
    }).then((r) => okJson<ShareLink>(r)),

  getShare: (jobId: string): Promise<ShareLink | null> =>
    authFetch(`${API}/jobs/${jobId}/share`).then((r) => {
      if (r.status === 404) return null;
      if (!r.ok) throw new Error("share lookup failed");
      return r.json();
    }),

  revokeShare: (jobId: string): Promise<void> =>
    authFetch(`${API}/jobs/${jobId}/share`, { method: "DELETE" }).then(() => {}),

  // Schedules
  getSchedules: (): Promise<Schedule[]> => authFetch(`${API}/schedules`).then((r) => r.json()),
  createSchedule: (s: ScheduleInput): Promise<Schedule> =>
    authFetch(`${API}/schedules`, { method: "POST", body: JSON.stringify(s) }).then((r) => okJson<Schedule>(r)),
  updateSchedule: (id: string, s: ScheduleInput): Promise<Schedule> =>
    authFetch(`${API}/schedules/${id}`, { method: "PUT", body: JSON.stringify(s) }).then((r) => okJson<Schedule>(r)),
  deleteSchedule: (id: string): Promise<void> =>
    authFetch(`${API}/schedules/${id}`, { method: "DELETE" }).then(() => {}),
  runSchedule: (id: string): Promise<{ job_id: string }> =>
    authFetch(`${API}/schedules/${id}/run`, { method: "POST" }).then((r) => okJson<{ job_id: string }>(r)),

  // Admin: users
  getUsers: (): Promise<UserInfo[]> =>
    authFetch(`${API}/users`).then((r) => r.json()),

  createUser: (username: string, password: string, role: string, email?: string): Promise<UserInfo> =>
    authFetch(`${API}/users`, { method: "POST", body: JSON.stringify({ username, password, role, email: email || "" }) }).then((r) => okJson<UserInfo>(r)),

  updateUser: (id: string, role: string, password?: string, email?: string): Promise<UserInfo> =>
    authFetch(`${API}/users/${id}`, { method: "PUT", body: JSON.stringify({ role, password: password || "", email }) }).then((r) => okJson<UserInfo>(r)),

  deleteUser: (id: string): Promise<void> =>
    authFetch(`${API}/users/${id}`, { method: "DELETE" }).then(() => {}),

  // Admin: audit
  getAudit: (limit: number, offset: number, action?: string, user?: string): Promise<{ entries: AuditEntry[]; total: number }> => {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    if (action) params.set("action", action);
    if (user) params.set("user", user);
    return authFetch(`${API}/audit?${params}`).then((r) => okJson(r));
  },
  downloadAudit: async (): Promise<void> => {
    const res = await authFetch(`${API}/audit/download`);
    if (!res.ok) throw new Error(await res.text());
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "audit.jsonl";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  },

  // Admin: server settings
  getSettings: (): Promise<ServerSettings> => authFetch(`${API}/settings`).then((r) => okJson<ServerSettings>(r)),
  updateSettings: (s: ServerSettingsInput): Promise<ServerSettings> =>
    authFetch(`${API}/settings`, { method: "PUT", body: JSON.stringify(s) }).then((r) => okJson<ServerSettings>(r)),
  testNotify: (channel: "telegram" | "email" | "webhook"): Promise<{ ok: boolean; error?: string }> =>
    authFetch(`${API}/settings/notify/test`, { method: "POST", body: JSON.stringify({ channel }) }).then((r) => okJson(r)),
};
