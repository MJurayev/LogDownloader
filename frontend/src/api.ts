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

export interface Datasource {
  id: string;
  name: string;
  url: string;
  username?: string;
  password?: string;
  headers?: Record<string, string>;
  global: boolean;
  owner_id: string;
}

export interface SavedQuery {
  id: string;
  name: string;
  query: string;
  datasource_id?: string;
  global: boolean;
  owner_id: string;
}

export interface ExportJob {
  id: string;
  query: string;
  datasource_id: string;
  datasource_name: string;
  status: "running" | "done" | "failed";
  file_name: string;
  created_at: string;
  error?: string;
  lines: number;
  start?: string;
  end?: string;
}

export interface QueryResult {
  logs: Record<string, string>[];
  total: number;
  has_more: boolean;
  limit: number;
  offset: number;
}

export interface AuthUser {
  user_id: string;
  username: string;
  role: "admin" | "user";
}

export interface UserInfo {
  id: string;
  username: string;
  role: "admin" | "user";
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
}

export const api = {
  // Auth
  login: async (username: string, password: string): Promise<AuthUser & { token: string }> => {
    const res = await fetch(`${API}/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    if (!res.ok) throw new Error("Invalid credentials");
    return res.json();
  },

  me: (): Promise<AuthUser> => authFetch(`${API}/me`).then((r) => r.json()),

  // Datasources
  getDatasources: (): Promise<Datasource[]> =>
    authFetch(`${API}/datasources`).then((r) => r.json()),

  createDatasource: (ds: Omit<Datasource, "id" | "owner_id">): Promise<Datasource> =>
    authFetch(`${API}/datasources`, {
      method: "POST",
      body: JSON.stringify(ds),
    }).then((r) => r.json()),

  updateDatasource: (id: string, ds: Omit<Datasource, "id" | "owner_id">): Promise<Datasource> =>
    authFetch(`${API}/datasources/${id}`, {
      method: "PUT",
      body: JSON.stringify(ds),
    }).then((r) => r.json()),

  deleteDatasource: (id: string): Promise<void> =>
    authFetch(`${API}/datasources/${id}`, { method: "DELETE" }).then(() => {}),

  // Queries
  getQueries: (): Promise<SavedQuery[]> =>
    authFetch(`${API}/queries`).then((r) => r.json()),

  createQuery: (name: string, query: string, datasourceId?: string, global?: boolean): Promise<SavedQuery> =>
    authFetch(`${API}/queries`, {
      method: "POST",
      body: JSON.stringify({ name, query, datasource_id: datasourceId, global: global || false }),
    }).then((r) => r.json()),

  updateQuery: (id: string, name: string, query: string, datasourceId?: string, global?: boolean): Promise<SavedQuery> =>
    authFetch(`${API}/queries/${id}`, {
      method: "PUT",
      body: JSON.stringify({ name, query, datasource_id: datasourceId, global: global || false }),
    }).then((r) => r.json()),

  deleteQuery: (id: string): Promise<void> =>
    authFetch(`${API}/queries/${id}`, { method: "DELETE" }).then(() => {}),

  // Query logs
  queryLogs: (
    query: string,
    datasourceId: string,
    limit: number,
    offset: number,
    start?: string,
    end?: string,
    sortOrder?: "asc" | "desc",
  ): Promise<QueryResult> => {
    const params = new URLSearchParams({
      query,
      datasource_id: datasourceId,
      limit: String(limit),
      offset: String(offset),
    });
    if (start) params.set("start", start);
    if (end) params.set("end", end);
    if (sortOrder) params.set("sort", sortOrder);
    return authFetch(`${API}/query?${params}`).then((r) => {
      if (!r.ok) return r.text().then((t) => Promise.reject(t));
      return r.json();
    });
  },

  // Export & Jobs
  startExport: (
    query: string,
    datasourceId: string,
    start?: string,
    end?: string,
    sortOrder?: "asc" | "desc",
    name?: string,
  ): Promise<{ job_id: string }> =>
    authFetch(`${API}/export`, {
      method: "POST",
      body: JSON.stringify({
        query,
        datasource_id: datasourceId,
        start: start || "",
        end: end || "",
        sort_order: sortOrder || "",
        name: name || "",
      }),
    }).then((r) => r.json()),

  getJobs: (): Promise<ExportJob[]> =>
    authFetch(`${API}/jobs`).then((r) => r.json()),

  deleteJob: (id: string): Promise<void> =>
    authFetch(`${API}/jobs/${id}`, { method: "DELETE" }).then(() => {}),

  // Brauzer'ning native streaming download'i — fayl xotiraga to'liq yuklanmasdan
  // to'g'ridan-to'g'ri diskka oqim qiladi. Bu katta exportlarni qo'llab-quvvatlaydi
  // (oldingi blob yondashuvi ~1GB+ da OOM bo'lardi). JWT URL'ga query param sifatida
  // qo'shiladi — backend auth middleware'da /download endpoint'i uchun maxsus qabul.
  // `fileName` server `Content-Disposition`'dan oldin fallback sifatida ishlatiladi.
  downloadJob: (id: string, fileName?: string): void => {
    const token = getToken();
    if (!token) {
      window.location.href = "/login";
      return;
    }
    const url = `${API}/jobs/${id}/download?token=${encodeURIComponent(token)}`;
    const a = document.createElement("a");
    a.href = url;
    a.download = fileName || `export_${id}.log`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  },

  // Share links
  createShare: (
    jobId: string,
    opts: { maxDownloads?: number; expiresAt?: string | null },
  ): Promise<ShareLink> =>
    authFetch(`${API}/jobs/${jobId}/share`, {
      method: "POST",
      body: JSON.stringify({
        max_downloads: opts.maxDownloads ?? 0,
        expires_at: opts.expiresAt ?? "",
      }),
    }).then((r) => {
      if (!r.ok) return r.text().then((t) => Promise.reject(t));
      return r.json();
    }),

  getShare: (jobId: string): Promise<ShareLink | null> =>
    authFetch(`${API}/jobs/${jobId}/share`).then((r) => {
      if (r.status === 404) return null;
      if (!r.ok) throw new Error("share lookup failed");
      return r.json();
    }),

  revokeShare: (jobId: string): Promise<void> =>
    authFetch(`${API}/jobs/${jobId}/share`, { method: "DELETE" }).then(() => {}),

  // Users (admin)
  getUsers: (): Promise<UserInfo[]> =>
    authFetch(`${API}/users`).then((r) => r.json()),

  createUser: (username: string, password: string, role: string): Promise<UserInfo> =>
    authFetch(`${API}/users`, {
      method: "POST",
      body: JSON.stringify({ username, password, role }),
    }).then((r) => r.json()),

  updateUser: (id: string, role: string, password?: string): Promise<UserInfo> =>
    authFetch(`${API}/users/${id}`, {
      method: "PUT",
      body: JSON.stringify({ role, password: password || "" }),
    }).then((r) => r.json()),

  deleteUser: (id: string): Promise<void> =>
    authFetch(`${API}/users/${id}`, { method: "DELETE" }).then(() => {}),
};
