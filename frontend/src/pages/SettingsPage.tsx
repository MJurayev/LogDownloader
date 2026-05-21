import { useState, useEffect } from "react";
import { api, type Datasource } from "../api";
import "./Pages.css";

type HeaderRow = { key: string; value: string };
type DSForm = {
  name: string;
  url: string;
  username: string;
  password: string;
  global: boolean;
  headers: HeaderRow[];
};

const emptyForm = (): DSForm => ({
  name: "",
  url: "",
  username: "",
  password: "",
  global: false,
  headers: [],
});

const formFromDS = (ds: Datasource): DSForm => ({
  name: ds.name,
  url: ds.url,
  username: ds.username || "",
  password: ds.password || "",
  global: ds.global,
  headers: Object.entries(ds.headers || {}).map(([key, value]) => ({ key, value })),
});

const formToBody = (f: DSForm): Omit<Datasource, "id" | "owner_id"> => {
  const headersMap: Record<string, string> = {};
  f.headers.forEach((h) => {
    const k = h.key.trim();
    if (k) headersMap[k] = h.value;
  });
  return {
    name: f.name,
    url: f.url,
    username: f.username || undefined,
    password: f.password || undefined,
    headers: Object.keys(headersMap).length > 0 ? headersMap : undefined,
    global: f.global,
  };
};

export default function SettingsPage({ isAdmin }: { isAdmin: boolean }) {
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  const [newDS, setNewDS] = useState<DSForm>(emptyForm());
  const [showNew, setShowNew] = useState(false);

  const [editingDS, setEditingDS] = useState<DSForm | null>(null);

  const loadDatasources = async () => {
    const data = await api.getDatasources();
    setDatasources(data || []);
    setLoading(false);
  };

  useEffect(() => { loadDatasources(); }, []);

  const toggleExpand = (ds: Datasource) => {
    if (expandedId === ds.id) {
      setExpandedId(null);
      setEditingDS(null);
    } else {
      setExpandedId(ds.id);
      setEditingDS(formFromDS(ds));
    }
  };

  const handleCreate = async () => {
    if (!newDS.name.trim() || !newDS.url.trim()) return;
    setSaving(true);
    await api.createDatasource(formToBody(newDS));
    setNewDS(emptyForm());
    setShowNew(false);
    await loadDatasources();
    setSaving(false);
  };

  const handleUpdate = async () => {
    if (!editingDS || !expandedId) return;
    if (!editingDS.name.trim() || !editingDS.url.trim()) return;
    setSaving(true);
    await api.updateDatasource(expandedId, formToBody(editingDS));
    setEditingDS(null);
    setExpandedId(null);
    await loadDatasources();
    setSaving(false);
  };

  const handleDelete = async (id: string) => {
    await api.deleteDatasource(id);
    if (expandedId === id) {
      setExpandedId(null);
      setEditingDS(null);
    }
    loadDatasources();
  };

  const renderForm = (form: DSForm, setForm: (f: DSForm) => void) => {
    const addHeader = () => setForm({ ...form, headers: [...form.headers, { key: "", value: "" }] });
    const updateHeader = (i: number, field: "key" | "value", v: string) =>
      setForm({ ...form, headers: form.headers.map((h, idx) => (idx === i ? { ...h, [field]: v } : h)) });
    const removeHeader = (i: number) =>
      setForm({ ...form, headers: form.headers.filter((_, idx) => idx !== i) });

    return (
      <>
        <div className="form-group">
          <label className="label">Nomi</label>
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Production VictoriaLogs" className="input" />
        </div>
        <div className="form-group">
          <label className="label">URL</label>
          <input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="http://victorialogs:9428" className="input" />
        </div>
        <div className="form-group">
          <label className="label">Basic Auth (ixtiyoriy)</label>
          <div className="settings-auth-row">
            <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} placeholder="Username" className="input" />
            <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} placeholder="Password" className="input" />
          </div>
        </div>
        <div className="form-group">
          <label className="label">Headers (ixtiyoriy)</label>
          {form.headers.map((h, i) => (
            <div key={i} className="settings-header-row" style={{ marginBottom: 8 }}>
              <input
                value={h.key}
                onChange={(e) => updateHeader(i, "key", e.target.value)}
                placeholder="Header name (masalan: X-Tenant)"
                className="input"
              />
              <input
                value={h.value}
                onChange={(e) => updateHeader(i, "value", e.target.value)}
                placeholder="Value"
                className="input"
              />
              <button
                type="button"
                onClick={() => removeHeader(i)}
                className="btn btn-small btn-danger"
                title="Header o'chirish"
              >
                ×
              </button>
            </div>
          ))}
          <button type="button" onClick={addHeader} className="btn btn-small btn-outline">
            + Header qo'shish
          </button>
        </div>
        {isAdmin && (
          <label className="global-check">
            <input type="checkbox" checked={form.global} onChange={(e) => setForm({ ...form, global: e.target.checked })} />
            Global (barcha userlarga ko'rinadi)
          </label>
        )}
      </>
    );
  };

  if (loading) return <div className="page">Yuklanmoqda...</div>;

  return (
    <div className="page">
      <h1>Settings</h1>
      <p className="subtitle">VictoriaLogs datasource'larni boshqarish</p>

      <div className="ds-list">
        {datasources.map((ds) => (
          <div key={ds.id} className="ds-card">
            <div className="ds-card-header" onClick={() => toggleExpand(ds)}>
              <div className="ds-card-title">
                <span className="ds-dot" />
                <strong>{ds.name || "Nomsiz"}</strong>
                {ds.global && <span className="global-badge">Global</span>}
                <span className="ds-url-hint">{ds.url}</span>
              </div>
              <div className="ds-card-actions">
                <button
                  onClick={(e) => { e.stopPropagation(); handleDelete(ds.id); }}
                  className="btn btn-small btn-danger"
                >
                  O'chirish
                </button>
                <span className="ds-chevron">{expandedId === ds.id ? "▲" : "▼"}</span>
              </div>
            </div>

            {expandedId === ds.id && editingDS && (
              <div className="ds-card-body">
                {renderForm(editingDS, setEditingDS as (f: DSForm) => void)}
                <div className="ds-bottom-actions" style={{ marginTop: 14 }}>
                  <button
                    onClick={() => { setExpandedId(null); setEditingDS(null); }}
                    className="btn btn-small btn-outline"
                  >
                    Bekor
                  </button>
                  <button
                    onClick={handleUpdate}
                    disabled={saving || !editingDS.name.trim() || !editingDS.url.trim()}
                    className="btn btn-small btn-primary"
                  >
                    {saving ? "..." : "Saqlash"}
                  </button>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      {showNew ? (
        <div className="ds-card" style={{ marginBottom: 16 }}>
          <div className="ds-card-body" style={{ paddingTop: 16 }}>
            {renderForm(newDS, setNewDS)}
            <div className="ds-bottom-actions" style={{ marginTop: 14 }}>
              <button
                onClick={() => { setShowNew(false); setNewDS(emptyForm()); }}
                className="btn btn-small btn-outline"
              >
                Bekor
              </button>
              <button
                onClick={handleCreate}
                disabled={saving || !newDS.name.trim() || !newDS.url.trim()}
                className="btn btn-small btn-primary"
              >
                {saving ? "..." : "Saqlash"}
              </button>
            </div>
          </div>
        </div>
      ) : (
        <button onClick={() => setShowNew(true)} className="btn btn-outline">
          + Datasource qo'shish
        </button>
      )}
    </div>
  );
}
