import { useState, useEffect } from "react";
import { api, type Datasource, type DatasourceInput, type ServerSettings } from "../api";
import { useT } from "../i18n";
import "./Pages.css";

type HeaderRow = { key: string; value: string };
type DSForm = {
  name: string;
  url: string;
  username: string;
  password: string;
  hasPassword: boolean; // serverda parol saqlangan (GET javobida parol qaytmaydi)
  accountId: string;
  projectId: string;
  global: boolean;
  headers: HeaderRow[];
};

const emptyForm = (): DSForm => ({
  name: "",
  url: "",
  username: "",
  password: "",
  hasPassword: false,
  accountId: "",
  projectId: "",
  global: false,
  headers: [],
});

const formFromDS = (ds: Datasource): DSForm => ({
  name: ds.name,
  url: ds.url,
  username: ds.username || "",
  password: "",
  hasPassword: !!ds.has_password,
  accountId: ds.account_id || "",
  projectId: ds.project_id || "",
  global: ds.global,
  headers: Object.entries(ds.headers || {}).map(([key, value]) => ({ key, value })),
});

// Bo'sh parol yuborilsa server mavjud parolni saqlab qoladi (username bo'sh
// bo'lmasa). Shu sababli tahrirlashda parol maydonini bo'sh qoldirish xavfsiz.
const formToBody = (f: DSForm, id?: string): DatasourceInput => {
  const headersMap: Record<string, string> = {};
  f.headers.forEach((h) => {
    const k = h.key.trim();
    if (k) headersMap[k] = h.value;
  });
  return {
    id,
    name: f.name,
    url: f.url,
    username: f.username || undefined,
    password: f.password || undefined,
    account_id: f.accountId || undefined,
    project_id: f.projectId || undefined,
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
  const [testResult, setTestResult] = useState<Record<string, string>>({});
  const [testing, setTesting] = useState<string | null>(null);

  // Admin: server sozlamalari
  const [settings, setSettings] = useState<ServerSettings | null>(null);
  const [retDays, setRetDays] = useState(0);
  const [quotaGB, setQuotaGB] = useState(0);
  const [maxConc, setMaxConc] = useState(2);
  const [publicUrl, setPublicUrl] = useState("");
  const [tgToken, setTgToken] = useState("");
  const [clearTg, setClearTg] = useState(false);
  const [webhook, setWebhook] = useState("");
  const [smtp, setSmtp] = useState({ host: "", port: 587, username: "", password: "", from: "", starttls: true });
  const [clearSmtpPass, setClearSmtpPass] = useState(false);
  const [adminMsg, setAdminMsg] = useState("");
  const [adminSaving, setAdminSaving] = useState(false);
  const [notifyTestMsg, setNotifyTestMsg] = useState<Record<string, string>>({});

  const loadDatasources = async () => {
    const data = await api.getDatasources();
    setDatasources(data || []);
    setLoading(false);
  };

  const applySettings = (s: ServerSettings) => {
    setSettings(s);
    setRetDays(s.retention.days);
    setQuotaGB(s.retention.quota_gb);
    setMaxConc(s.worker.max_concurrent);
    setPublicUrl(s.public_url || "");
    setWebhook(s.notify.webhook_url || "");
    setSmtp({ host: s.notify.smtp.host || "", port: s.notify.smtp.port || 587, username: s.notify.smtp.username || "", password: "", from: s.notify.smtp.from || "", starttls: !!s.notify.smtp.starttls });
    setTgToken("");
    setClearTg(false);
    setClearSmtpPass(false);
  };

  useEffect(() => {
    loadDatasources();
    if (isAdmin) api.getSettings().then(applySettings).catch(() => setSettings(null));
  }, [isAdmin]);

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
    try {
      await api.createDatasource(formToBody(newDS));
      setNewDS(emptyForm());
      setShowNew(false);
      await loadDatasources();
    } finally {
      setSaving(false);
    }
  };

  const handleUpdate = async () => {
    if (!editingDS || !expandedId) return;
    if (!editingDS.name.trim() || !editingDS.url.trim()) return;
    setSaving(true);
    try {
      await api.updateDatasource(expandedId, formToBody(editingDS));
      setEditingDS(null);
      setExpandedId(null);
      await loadDatasources();
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (id: string) => {
    await api.deleteDatasource(id);
    if (expandedId === id) {
      setExpandedId(null);
      setEditingDS(null);
    }
    loadDatasources();
  };

  const handleTest = async (key: string, form: DSForm, id?: string) => {
    setTesting(key);
    setTestResult((r) => ({ ...r, [key]: "" }));
    try {
      const res = await api.testDatasource(formToBody(form, id));
      setTestResult((r) => ({ ...r, [key]: res.ok ? t("testOk", { ms: res.latency_ms }) : t("testFail", { err: res.error || res.status || "?" }) }));
    } catch (err) {
      setTestResult((r) => ({ ...r, [key]: t("testFail", { err: typeof err === "string" ? err : String(err) }) }));
    } finally {
      setTesting(null);
    }
  };

  const saveAdmin = async () => {
    setAdminSaving(true);
    setAdminMsg("");
    try {
      const s = await api.updateSettings({
        retention: { days: retDays, quota_gb: quotaGB },
        worker: { max_concurrent: maxConc },
        public_url: publicUrl,
        notify: {
          telegram_bot_token: tgToken,
          clear_telegram_token: clearTg,
          webhook_url: webhook,
          smtp: { host: smtp.host, port: Number(smtp.port) || 587, username: smtp.username, password: smtp.password || undefined, clear_password: clearSmtpPass, from: smtp.from, starttls: smtp.starttls },
        },
      });
      applySettings(s);
      setAdminMsg(t("saved"));
      setTimeout(() => setAdminMsg(""), 1500);
    } catch (err) {
      setAdminMsg(typeof err === "string" ? err : t("genericError"));
    } finally {
      setAdminSaving(false);
    }
  };

  const testNotify = async (channel: "telegram" | "email" | "webhook") => {
    setNotifyTestMsg((m) => ({ ...m, [channel]: "..." }));
    try {
      const r = await api.testNotify(channel);
      setNotifyTestMsg((m) => ({ ...m, [channel]: r.ok ? t("testSent") : t("testFail", { err: r.error || "?" }) }));
    } catch (err) {
      setNotifyTestMsg((m) => ({ ...m, [channel]: t("testFail", { err: typeof err === "string" ? err : String(err) }) }));
    }
  };

  const renderForm = (form: DSForm, setForm: (f: DSForm) => void, testKey: string, id?: string) => {
    const addHeader = () => setForm({ ...form, headers: [...form.headers, { key: "", value: "" }] });
    const updateHeader = (i: number, field: "key" | "value", v: string) =>
      setForm({ ...form, headers: form.headers.map((h, idx) => (idx === i ? { ...h, [field]: v } : h)) });
    const removeHeader = (i: number) => setForm({ ...form, headers: form.headers.filter((_, idx) => idx !== i) });

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
          <label className="label">{t("tenantLabel")}</label>
          <div className="settings-auth-row">
            <input value={form.accountId} onChange={(e) => setForm({ ...form, accountId: e.target.value })} placeholder={t("accountId")} className="input" />
            <input value={form.projectId} onChange={(e) => setForm({ ...form, projectId: e.target.value })} placeholder={t("projectId")} className="input" />
          </div>
        </div>
        <div className="form-group">
          <label className="label">{t("headers")}</label>
          {form.headers.map((h, i) => (
            <div key={i} className="settings-header-row" style={{ marginBottom: 8 }}>
              <input value={h.key} onChange={(e) => updateHeader(i, "key", e.target.value)} placeholder={t("headerNamePlaceholder")} className="input" />
              <input value={h.value} onChange={(e) => updateHeader(i, "value", e.target.value)} placeholder={t("headerValue")} className="input" />
              <button type="button" onClick={() => removeHeader(i)} className="btn btn-small btn-danger" title={t("removeHeader")}>×</button>
            </div>
          ))}
          <button type="button" onClick={addHeader} className="btn btn-small btn-outline">{t("addHeader")}</button>
        </div>
        {isAdmin && (
          <label className="global-check">
            <input type="checkbox" checked={form.global} onChange={(e) => setForm({ ...form, global: e.target.checked })} />
            {t("globalHint")}
          </label>
        )}
        <div className="ds-test-row">
          <button type="button" className="btn btn-small btn-outline" disabled={!form.url.trim() || testing === testKey} onClick={() => handleTest(testKey, form, id)}>
            {testing === testKey ? "..." : t("testConnection")}
          </button>
          {testResult[testKey] && <span className={`ds-test-result ${testResult[testKey].startsWith(t("testFail", { err: "" }).slice(0, 4)) ? "error-text" : "ok-text"}`}>{testResult[testKey]}</span>}
        </div>
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
                {(ds.account_id || ds.project_id) && <span className="job-parts-chip" title={t("tenantLabel")}>{ds.account_id || "·"}:{ds.project_id || "·"}</span>}
                <span className="ds-url-hint">{ds.url}</span>
              </div>
              <div className="ds-card-actions">
                <button onClick={(e) => { e.stopPropagation(); handleDelete(ds.id); }} className="btn btn-small btn-danger">{t("delete")}</button>
                <span className="ds-chevron">{expandedId === ds.id ? "▲" : "▼"}</span>
              </div>
            </div>

            {expandedId === ds.id && editingDS && (
              <div className="ds-card-body">
                {renderForm(editingDS, setEditingDS as (f: DSForm) => void, ds.id, ds.id)}
                <div className="ds-bottom-actions" style={{ marginTop: 14 }}>
                  <button onClick={() => { setExpandedId(null); setEditingDS(null); }} className="btn btn-small btn-outline">{t("cancelShort")}</button>
                  <button onClick={handleUpdate} disabled={saving || !editingDS.name.trim() || !editingDS.url.trim()} className="btn btn-small btn-primary">{saving ? "..." : t("save")}</button>
                </div>
              </div>
            )}
          </div>
        ))}
      </div>

      {showNew ? (
        <div className="ds-card" style={{ marginBottom: 16 }}>
          <div className="ds-card-body" style={{ paddingTop: 16 }}>
            {renderForm(newDS, setNewDS, "new")}
            <div className="ds-bottom-actions" style={{ marginTop: 14 }}>
              <button onClick={() => { setShowNew(false); setNewDS(emptyForm()); }} className="btn btn-small btn-outline">{t("cancelShort")}</button>
              <button onClick={handleCreate} disabled={saving || !newDS.name.trim() || !newDS.url.trim()} className="btn btn-small btn-primary">{saving ? "..." : t("save")}</button>
            </div>
          </div>
        </div>
      ) : (
        <button onClick={() => setShowNew(true)} className="btn btn-outline">{t("addDatasource")}</button>
      )}

      {isAdmin && settings && (
        <div className="admin-settings">
          <h2 className="section-title">{t("adminSection")}</h2>

          <div className="ds-card">
            <div className="ds-card-body" style={{ paddingTop: 16 }}>
              <h3 className="subsection-title">{t("retentionTitle")}</h3>
              <div className="settings-grid">
                <div className="form-group">
                  <label className="label">{t("retentionDaysLabel")}</label>
                  <input type="number" min={0} value={retDays} onChange={(e) => setRetDays(Math.max(0, Number(e.target.value)))} className="input" />
                </div>
                <div className="form-group">
                  <label className="label">{t("quotaLabel")}</label>
                  <input type="number" min={0} step={0.1} value={quotaGB} onChange={(e) => setQuotaGB(Math.max(0, Number(e.target.value)))} className="input" />
                </div>
                <div className="form-group">
                  <label className="label">{t("concurrencyLabel")}</label>
                  <input type="number" min={1} max={64} value={maxConc} onChange={(e) => setMaxConc(Math.min(64, Math.max(1, Number(e.target.value))))} className="input" />
                </div>
              </div>
              <div className="field-hint">{t("chunkInfo", { mb: settings.export_chunk_mb })}</div>
              <div className="form-group" style={{ marginTop: 12 }}>
                <label className="label">{t("publicUrlLabel")}</label>
                <input value={publicUrl} onChange={(e) => setPublicUrl(e.target.value)} placeholder={t("publicUrlPlaceholder")} className="input" />
              </div>

              <h3 className="subsection-title">{t("notifyTitle")}</h3>
              <div className="field-hint">{t("notifyRecipientsHint")}</div>
              <div className="form-group">
                <label className="label">{t("telegramToken")}</label>
                <div className="settings-auth-row">
                  <input type="password" value={tgToken} onChange={(e) => { setTgToken(e.target.value); setClearTg(false); }} placeholder={settings.notify.has_telegram_token && !clearTg ? t("tokenSaved") : "123456:ABC..."} className="input" />
                  {settings.notify.has_telegram_token && (
                    <button type="button" className="btn btn-small btn-outline" onClick={() => { setClearTg(true); setTgToken(""); }}>{clearTg ? "✓ " : ""}{t("clearValue")}</button>
                  )}
                  <button type="button" className="btn btn-small btn-outline" onClick={() => testNotify("telegram")}>{t("testTelegram")}</button>
                </div>
                {notifyTestMsg.telegram && <div className="field-hint">{notifyTestMsg.telegram}</div>}
              </div>
              <div className="form-group">
                <label className="label">{t("webhookUrl")}</label>
                <div className="settings-auth-row">
                  <input value={webhook} onChange={(e) => setWebhook(e.target.value)} placeholder="https://..." className="input" />
                  <button type="button" className="btn btn-small btn-outline" onClick={() => testNotify("webhook")}>{t("testWebhook")}</button>
                </div>
                {notifyTestMsg.webhook && <div className="field-hint">{notifyTestMsg.webhook}</div>}
              </div>
              <div className="form-group">
                <label className="label">{t("smtpTitle")}</label>
                <div className="settings-grid">
                  <input value={smtp.host} onChange={(e) => setSmtp({ ...smtp, host: e.target.value })} placeholder={t("smtpHost")} className="input" />
                  <input type="number" value={smtp.port} onChange={(e) => setSmtp({ ...smtp, port: Number(e.target.value) })} placeholder={t("smtpPort")} className="input" />
                  <input value={smtp.from} onChange={(e) => setSmtp({ ...smtp, from: e.target.value })} placeholder={t("smtpFrom")} className="input" />
                  <input value={smtp.username} onChange={(e) => setSmtp({ ...smtp, username: e.target.value })} placeholder={t("smtpUser")} className="input" />
                  <input type="password" value={smtp.password} onChange={(e) => { setSmtp({ ...smtp, password: e.target.value }); setClearSmtpPass(false); }} placeholder={settings.notify.smtp.has_password && !clearSmtpPass ? t("tokenSaved") : t("smtpPassword")} className="input" />
                  <label className="global-check">
                    <input type="checkbox" checked={smtp.starttls} onChange={(e) => setSmtp({ ...smtp, starttls: e.target.checked })} />
                    {t("smtpStartTLS")}
                  </label>
                </div>
                <div className="settings-auth-row" style={{ marginTop: 8 }}>
                  {settings.notify.smtp.has_password && (
                    <button type="button" className="btn btn-small btn-outline" onClick={() => { setClearSmtpPass(true); setSmtp({ ...smtp, password: "" }); }}>{clearSmtpPass ? "✓ " : ""}{t("clearValue")} ({t("smtpPassword")})</button>
                  )}
                  <button type="button" className="btn btn-small btn-outline" onClick={() => testNotify("email")}>{t("testEmail")}</button>
                  {notifyTestMsg.email && <span className="field-hint">{notifyTestMsg.email}</span>}
                </div>
              </div>

              <div className="ds-bottom-actions" style={{ marginTop: 14 }}>
                {adminMsg && <span className="save-modal-msg">{adminMsg}</span>}
                <button onClick={saveAdmin} disabled={adminSaving} className="btn btn-small btn-primary">{adminSaving ? "..." : t("save")}</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
