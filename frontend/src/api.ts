const API = "/api";

export interface Datasource {
  id: string;
  name: string;
  url: string;
  username?: string;
  password?: string;
  headers?: Record<string, string>;
}

export interface Settings {
  datasources: Datasource[];
}

export interface SavedQuery {
  id: string;
  name: string;
  query: string;
  datasource_id?: string;
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
}

export interface QueryResult {
  logs: Record<string, string>[];
  total: number;
  has_more: boolean;
  limit: number;
  offset: number;
}

export const api = {
  // Settings
  getSettings: (): Promise<Settings> =>
    fetch(`${API}/settings`).then((r) => r.json()),

  updateSettings: (s: Settings): Promise<Settings> =>
    fetch(`${API}/settings`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(s),
    }).then((r) => r.json()),

  // Queries
  getQueries: (): Promise<SavedQuery[]> =>
    fetch(`${API}/queries`).then((r) => r.json()),

  createQuery: (name: string, query: string, datasourceId?: string): Promise<SavedQuery> =>
    fetch(`${API}/queries`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, query, datasource_id: datasourceId }),
    }).then((r) => r.json()),

  deleteQuery: (id: string): Promise<void> =>
    fetch(`${API}/queries/${id}`, { method: "DELETE" }).then(() => {}),

  // Query logs
  queryLogs: (
    query: string,
    datasourceId: string,
    limit: number,
    offset: number,
    start?: string,
    end?: string
  ): Promise<QueryResult> => {
    const params = new URLSearchParams({
      query,
      datasource_id: datasourceId,
      limit: String(limit),
      offset: String(offset),
    });
    if (start) params.set("start", start);
    if (end) params.set("end", end);
    return fetch(`${API}/query?${params}`).then((r) => {
      if (!r.ok) return r.text().then((t) => Promise.reject(t));
      return r.json();
    });
  },

  // Export & Jobs
  startExport: (query: string, datasourceId: string): Promise<{ job_id: string }> =>
    fetch(`${API}/export`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ query, datasource_id: datasourceId }),
    }).then((r) => r.json()),

  getJobs: (): Promise<ExportJob[]> =>
    fetch(`${API}/jobs`).then((r) => r.json()),

  deleteJob: (id: string): Promise<void> =>
    fetch(`${API}/jobs/${id}`, { method: "DELETE" }).then(() => {}),

  downloadJob: (id: string): string => `${API}/jobs/${id}/download`,
};
