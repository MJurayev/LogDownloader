import { useState, useEffect } from "react";
import { api, type Datasource } from "../api";
import { useT } from "../i18n";
import "./Pages.css";

type HeaderRow = { key: string; value: string };
type DSForm = {
  name: string;
  url: string;
  username: string;
  password: string;
  hasPassword: boolean; // serverda parol saqlangan (GET javobida parol qaytmaydi)
  global: boolean;
  headers: HeaderRow[];
};

const emptyForm = (): DSForm => ({
  name: "",
  url: "",
  username: "",
  password: "",
  hasPassword: false,
  global: false,
  headers: [],
});

const formFromDS = (ds: Datasource): DSForm => ({
  name: ds.name,
  url: ds.url,
  username: ds.username || "",
  password: "",
  hasPassword: !!ds.has_password,
  global: ds.global,
  headers: Object.entries(ds.headers || {}).map(([key, value]) => ({ key, value })),
});

// Bo'sh parol yuborilsa server mavjud parolni saqlab qoladi (username bo'sh
// bo'lmasa). Shu sababli tahrirlashda parol maydonini bo'sh qoldirish xavfsiz.
const formToBody = (f: DSForm): Omit<Datasource, "id" | "owner_id" | "has_password"> => {
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
  const { t } = useT();
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
          <label className="label">{t("name")}</label>
          <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Production VictoriaLogs" className="input" />
        </div>
        <div className="form-group">
          <label className="label">{t("url")}</label>
          <input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="http://victorialogs:9428" className="input" />
        </div>
        <div className="form-group">
          <label className="label">{t("basicAuth")}</label>
          <div className="settings-auth-row">
            <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} placeholder={t("username")} className="input" />
            <input
              type="password"
              value={form.password}
              onChange={(e) => setForm({ ...form, password: e.target.value })}
              placeholder={form.hasPassword && form.username ? t("passwordSavedPlaceholder") : t("password")}
              className="input"
            />
          </div>
        </div>
        <div className="form-group">
          <label className="label">{t("headers")}</label>
          {form.headers.map((h, i) => (
            <div key={i} className="settings-header-row" style={{ marginBottom: 8 }}>
              <input
                value={h.key}
                onChange={(e) => updateHeader(i, "key", e.target.value)}
                placeholder={t("headerNamePlaceholder")}
                className="input"
              />
              <input
                value={h.value}
                onChange={(e) => updateHeader(i, "value", e.target.value)}
                placeholder={t("headerValue")}
                className="input"
              />
              <button
                type="button"
                onClick={() => removeHeader(i)}
                className="btn btn-small btn-danger"
                title={t("removeHeader")}
              >
                ×
              </button>
            </div>
          ))}
          <button type="button" onClick={addHeader} className="btn btn-small btn-outline">
            {t("addHeader")}
          </button>
        </div>
        {isAdmin && (
          <label className="global-check">
            <input type="checkbox" checked={form.global} onChange={(e) => setForm({ ...form, global: e.target.checked })} />
            {t("globalHint")}
          </label>
        )}
      </>
    );
  };

  if (loading) return <div className="page">{t("loading")}</div>;

  return (
    <div className="page">
      <h1>{t("settingsTitle")}</h1>
      <p className="subtitle">{t("settingsSubtitle")}</p>

      <div className="ds-list">
        {datasources.map((ds) => (
          <div key={ds.id} className="ds-card">
            <div className="ds-card-header" onClick={() => toggleExpand(ds)}>
              <div className="ds-card-title">
                <span className="ds-dot" />
                <strong>{ds.name || t("unnamed")}</strong>
                {ds.global && <span className="global-badge">{t("globalBadge")}</span>}
                <span className="ds-url-hint">{ds.url}</span>
              </div>
              <div className="ds-card-actions">
                <button
                  onClick={(e) => { e.stopPropagation(); handleDelete(ds.id); }}
                  className="btn btn-small btn-danger"
                >
                  {t("delete")}
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
                    {t("cancelShort")}
                  </button>
                  <button
                    onClick={handleUpdate}
                    disabled={saving || !editingDS.name.trim() || !editingDS.url.trim()}
                    className="btn btn-small btn-primary"
                  >
                    {saving ? "..." : t("save")}
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
                {t("cancelShort")}
              </button>
              <button
                onClick={handleCreate}
                disabled={saving || !newDS.name.trim() || !newDS.url.trim()}
                className="btn btn-small btn-primary"
              >
                {saving ? "..." : t("save")}
              </button>
            </div>
          </div>
        </div>
      ) : (
        <button onClick={() => setShowNew(true)} className="btn btn-outline">
          {t("addDatasource")}
        </button>
      )}
    </div>
  );
}
