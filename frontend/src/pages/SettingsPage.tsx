import { useState, useEffect } from "react";
import { api, type Datasource } from "../api";
import "./Pages.css";

function genId(): string {
  return Date.now().toString(36) + Math.random().toString(36).slice(2, 10);
}

function newDatasource(): Datasource {
  return { id: genId(), name: "", url: "", username: "", password: "", headers: {} };
}

export default function SettingsPage() {
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [saved, setSaved] = useState(false);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);

  useEffect(() => {
    api.getSettings().then((s) => {
      setDatasources(s.datasources || []);
      setLoading(false);
    });
  }, []);

  const handleSave = async () => {
    const cleaned = datasources.map((ds) => ({
      ...ds,
      username: ds.username || undefined,
      password: ds.password || undefined,
      headers: ds.headers && Object.keys(ds.headers).length > 0 ? ds.headers : undefined,
    }));
    await api.updateSettings({ datasources: cleaned });
    setSaved(true);
    setTimeout(() => setSaved(false), 2000);
  };

  const addDatasource = () => {
    const ds = newDatasource();
    setDatasources([...datasources, ds]);
    setExpandedId(ds.id);
  };

  const removeDatasource = (id: string) => {
    setDatasources(datasources.filter((d) => d.id !== id));
    if (expandedId === id) setExpandedId(null);
  };

  const updateDS = (id: string, field: string, value: string) => {
    setDatasources(datasources.map((d) =>
      d.id === id ? { ...d, [field]: value } : d
    ));
  };

  const [dsHeaders, setDsHeaders] = useState<Record<string, { key: string; value: string }[]>>({});

  useEffect(() => {
    const map: Record<string, { key: string; value: string }[]> = {};
    datasources.forEach((ds) => {
      if (!dsHeaders[ds.id]) {
        map[ds.id] = Object.entries(ds.headers || {}).map(([key, value]) => ({ key, value }));
      } else {
        map[ds.id] = dsHeaders[ds.id];
      }
    });
    if (Object.keys(map).length > 0 && Object.keys(dsHeaders).length === 0) {
      setDsHeaders(map);
    }
  }, [datasources]);

  const getHeaders = (ds: Datasource) => dsHeaders[ds.id] || [];

  const updateHeaderRow = (dsId: string, idx: number, field: "key" | "value", val: string) => {
    const arr = [...(dsHeaders[dsId] || [])];
    arr[idx] = { ...arr[idx], [field]: val };
    setDsHeaders({ ...dsHeaders, [dsId]: arr });
    // Sync to datasources
    const map: Record<string, string> = {};
    arr.forEach((h) => { if (h.key.trim()) map[h.key.trim()] = h.value; });
    setDatasources(datasources.map((d) => d.id === dsId ? { ...d, headers: map } : d));
  };

  const addHeaderRow = (dsId: string) => {
    const arr = [...(dsHeaders[dsId] || []), { key: "", value: "" }];
    setDsHeaders({ ...dsHeaders, [dsId]: arr });
  };

  const removeHeaderRow = (dsId: string, idx: number) => {
    const arr = (dsHeaders[dsId] || []).filter((_, i) => i !== idx);
    setDsHeaders({ ...dsHeaders, [dsId]: arr });
    const map: Record<string, string> = {};
    arr.forEach((h) => { if (h.key.trim()) map[h.key.trim()] = h.value; });
    setDatasources(datasources.map((d) => d.id === dsId ? { ...d, headers: map } : d));
  };

  if (loading) return <div className="page">Yuklanmoqda...</div>;

  return (
    <div className="page">
      <h1>Settings</h1>
      <p className="subtitle">VictoriaLogs datasource'larni boshqarish</p>

      <div className="ds-list">
        {datasources.map((ds) => (
          <div key={ds.id} className="ds-card">
            <div className="ds-card-header" onClick={() => setExpandedId(expandedId === ds.id ? null : ds.id)}>
              <div className="ds-card-title">
                <span className="ds-dot" />
                <strong>{ds.name || "Nomsiz datasource"}</strong>
                <span className="ds-url-hint">{ds.url || "URL kiritilmagan"}</span>
              </div>
              <div className="ds-card-actions">
                <button
                  onClick={(e) => { e.stopPropagation(); removeDatasource(ds.id); }}
                  className="btn btn-small btn-danger"
                >
                  O'chirish
                </button>
                <span className="ds-chevron">{expandedId === ds.id ? "▲" : "▼"}</span>
              </div>
            </div>

            {expandedId === ds.id && (
              <div className="ds-card-body">
                <div className="form-group">
                  <label className="label">Nomi</label>
                  <input
                    value={ds.name}
                    onChange={(e) => updateDS(ds.id, "name", e.target.value)}
                    placeholder="Masalan: Production VictoriaLogs"
                    className="input"
                  />
                </div>
                <div className="form-group">
                  <label className="label">URL</label>
                  <input
                    value={ds.url}
                    onChange={(e) => updateDS(ds.id, "url", e.target.value)}
                    placeholder="http://victorialogs:9428"
                    className="input"
                  />
                </div>
                <div className="form-group">
                  <label className="label">Basic Auth (ixtiyoriy)</label>
                  <div className="settings-auth-row">
                    <input
                      value={ds.username || ""}
                      onChange={(e) => updateDS(ds.id, "username", e.target.value)}
                      placeholder="Username"
                      className="input"
                    />
                    <input
                      type="password"
                      value={ds.password || ""}
                      onChange={(e) => updateDS(ds.id, "password", e.target.value)}
                      placeholder="Password"
                      className="input"
                    />
                  </div>
                </div>
                <div className="form-group">
                  <label className="label">Headers (ixtiyoriy)</label>
                  {getHeaders(ds).map((h, i) => (
                    <div key={i} className="settings-header-row">
                      <input
                        value={h.key}
                        onChange={(e) => updateHeaderRow(ds.id, i, "key", e.target.value)}
                        placeholder="Header nomi"
                        className="input"
                      />
                      <input
                        value={h.value}
                        onChange={(e) => updateHeaderRow(ds.id, i, "value", e.target.value)}
                        placeholder="Qiymati"
                        className="input"
                      />
                      <button
                        onClick={() => removeHeaderRow(ds.id, i)}
                        className="btn btn-small btn-danger"
                      >
                        &times;
                      </button>
                    </div>
                  ))}
                  <button
                    onClick={() => addHeaderRow(ds.id)}
                    className="btn btn-small btn-outline"
                  >
                    + Header
                  </button>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      <div className="ds-bottom-actions">
        <button onClick={addDatasource} className="btn btn-outline">
          + Datasource qo'shish
        </button>
        <button onClick={handleSave} className="btn btn-primary">
          Saqlash
        </button>
        {saved && <span className="saved-msg">Saqlandi!</span>}
      </div>
    </div>
  );
}
