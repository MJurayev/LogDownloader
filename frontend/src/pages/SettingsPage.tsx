import { useState, useEffect } from "react";
import { api, type Datasource, type DatasourceInput, type LDAPSettings, type ServerSettings } from "../api";
import { useT, type TKey } from "../i18n";
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

type Tab = "datasources" | "export" | "ldap" | "notify";

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

const emptyLdap = (): LDAPSettings => ({
  enabled: false, url: "", starttls: false, insecure_skip_verify: false, bind_dn: "", bind_password: "", base_dn: "",
  user_filter: "", username_attr: "", email_attr: "", admin_group_dn: "", allowed_group_dn: "", default_role: "user", timeout_sec: 10,
});

export default function SettingsPage({ isAdmin }: { isAdmin: boolean }) {
  const { t } = useT();
  const [tab, setTab] = useState<Tab>("datasources");

  // Datasources
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [newDS, setNewDS] = useState<DSForm>(emptyForm());
  const [showNew, setShowNew] = useState(false);
  const [editingDS, setEditingDS] = useState<DSForm | null>(null);
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; text: string }>>({});
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
  const [ldap, setLdap] = useState<LDAPSettings>(emptyLdap());
  const [ldapHasPass, setLdapHasPass] = useState(false);
  const [ldapClearPass, setLdapClearPass] = useState(false);
  const [ldapTestUser, setLdapTestUser] = useState("");
  const [ldapTestPass, setLdapTestPass] = useState("");
  const [ldapTestMsg, setLdapTestMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [ldapTesting, setLdapTesting] = useState(false);
  const [adminMsg, setAdminMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [adminSaving, setAdminSaving] = useState(false);
  const [notifyTestMsg, setNotifyTestMsg] = useState<Record<string, { ok: boolean; text: string }>>({});

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
    const l = s.ldap || emptyLdap();
    setLdap({ ...emptyLdap(), ...l, bind_password: "", timeout_sec: l.timeout_sec || 10 });
    setLdapHasPass(!!l.has_bind_password);
    setLdapClearPass(false);
  };

  useEffect(() => {
    loadDatasources();
    if (isAdmin) api.getSettings().then(applySettings).catch(() => setSettings(null));
  }, [isAdmin]);

  // ----- datasource CRUD -----
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
    if (!editingDS || !expandedId || !editingDS.name.trim() || !editingDS.url.trim()) return;
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
    setTestResult((r) => ({ ...r, [key]: { ok: false, text: "" } }));
    try {
      const res = await api.testDatasource(formToBody(form, id));
      setTestResult((r) => ({ ...r, [key]: res.ok ? { ok: true, text: t("testOk", { ms: res.latency_ms }) } : { ok: false, text: t("testFail", { err: res.error || res.status || "?" }) } }));
    } catch (err) {
      setTestResult((r) => ({ ...r, [key]: { ok: false, text: t("testFail", { err: typeof err === "string" ? err : String(err) }) } }));
    } finally {
      setTesting(null);
    }
  };

  // ----- admin settings -----
  const ldapPayload = (): LDAPSettings => ({ ...ldap, bind_password: ldap.bind_password || undefined, clear_bind_password: ldapClearPass, has_bind_password: undefined });

  const saveAdmin = async () => {
    setAdminSaving(true);
    setAdminMsg(null);
    try {
      const s = await api.updateSettings({
        retention: { days: retDays, quota_gb: quotaGB },
        worker: { max_concurrent: maxConc },
        public_url: publicUrl,
        ldap: ldapPayload(),
        notify: {
          telegram_bot_token: tgToken,
          clear_telegram_token: clearTg,
          webhook_url: webhook,
          smtp: { host: smtp.host, port: Number(smtp.port) || 587, username: smtp.username, password: smtp.password || undefined, clear_password: clearSmtpPass, from: smtp.from, starttls: smtp.starttls },
        },
      });
      applySettings(s);
      setAdminMsg({ ok: true, text: t("saved") });
      setTimeout(() => setAdminMsg(null), 2000);
    } catch (err) {
      setAdminMsg({ ok: false, text: typeof err === "string" ? err : t("genericError") });
    } finally {
      setAdminSaving(false);
    }
  };

  const testNotify = async (channel: "telegram" | "email" | "webhook") => {
    setNotifyTestMsg((m) => ({ ...m, [channel]: { ok: false, text: "..." } }));
    try {
      const r = await api.testNotify(channel);
      setNotifyTestMsg((m) => ({ ...m, [channel]: r.ok ? { ok: true, text: t("testSent") } : { ok: false, text: t("testFail", { err: r.error || "?" }) } }));
    } catch (err) {
      setNotifyTestMsg((m) => ({ ...m, [channel]: { ok: false, text: t("testFail", { err: typeof err === "string" ? err : String(err) }) } }));
    }
  };

  const testLdap = async (withLogin: boolean) => {
    setLdapTesting(true);
    setLdapTestMsg(null);
    try {
      const r = await api.testLDAP(ldapPayload(), withLogin ? ldapTestUser : "", withLogin ? ldapTestPass : "");
      if (!r.ok) setLdapTestMsg({ ok: false, text: t("testFail", { err: r.error || "?" }) });
      else if (withLogin && r.result) setLdapTestMsg({ ok: true, text: t("ldapTestLoginOk", { dn: r.result.dn, groups: (r.result.groups || []).length, admin: r.result.is_admin ? t("yes") : t("no") }) });
      else setLdapTestMsg({ ok: true, text: t("ldapTestOk", { dn: r.result?.dn || "" }) });
    } catch (err) {
      setLdapTestMsg({ ok: false, text: t("testFail", { err: typeof err === "string" ? err : String(err) }) });
    } finally {
      setLdapTesting(false);
    }
  };

  // ----- renderers -----
  const status = (s?: { ok: boolean; text: string } | null) => (s && s.text ? <span className={`hint ${s.ok ? "ok" : "err"}`}>{s.text}</span> : null);

  const saveBar = () => (
    <div className="panel-actions">
      <span className={`msg ${adminMsg && !adminMsg.ok ? "err" : ""}`}>{adminMsg ? adminMsg.text : t("unsavedHint")}</span>
      <button onClick={saveAdmin} disabled={adminSaving} className="btn btn-small btn-primary">{adminSaving ? "..." : t("save")}</button>
    </div>
  );

  const renderDSForm = (form: DSForm, setForm: (f: DSForm) => void, testKey: string, id?: string) => {
    const addHeader = () => setForm({ ...form, headers: [...form.headers, { key: "", value: "" }] });
    const updateHeader = (i: number, field: "key" | "value", v: string) =>
      setForm({ ...form, headers: form.headers.map((h, idx) => (idx === i ? { ...h, [field]: v } : h)) });
    const removeHeader = (i: number) => setForm({ ...form, headers: form.headers.filter((_, idx) => idx !== i) });

    return (
      <>
        <div className="panel-sub first">{t("dsSectionConn")}</div>
        <div className="form-grid">
          <div className="field">
            <label className="label">{t("name")}</label>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Production VictoriaLogs" className="input" />
          </div>
          <div className="field">
            <label className="label">{t("url")}</label>
            <input value={form.url} onChange={(e) => setForm({ ...form, url: e.target.value })} placeholder="http://victorialogs:9428" className="input" />
          </div>
        </div>

        <div className="panel-sub">{t("dsSectionAuth")}</div>
        <div className="form-grid">
          <div className="field">
            <label className="label">{t("basicAuth")}</label>
            <div className="field-row">
              <input value={form.username} onChange={(e) => setForm({ ...form, username: e.target.value })} placeholder={t("username")} className="input" autoComplete="off" />
              <input type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} placeholder={form.hasPassword && form.username ? t("passwordSavedPlaceholder") : t("password")} className="input" autoComplete="new-password" />
            </div>
          </div>
          <div className="field">
            <label className="label">{t("tenantLabel")}</label>
            <div className="field-row">
              <input value={form.accountId} onChange={(e) => setForm({ ...form, accountId: e.target.value })} placeholder={t("accountId")} className="input" />
              <input value={form.projectId} onChange={(e) => setForm({ ...form, projectId: e.target.value })} placeholder={t("projectId")} className="input" />
            </div>
          </div>
        </div>

        <div className="panel-sub">{t("dsSectionHeaders")}</div>
        <div className="header-rows">
          {form.headers.map((h, i) => (
            <div key={i} className="field-row">
              <input value={h.key} onChange={(e) => updateHeader(i, "key", e.target.value)} placeholder={t("headerNamePlaceholder")} className="input" />
              <input value={h.value} onChange={(e) => updateHeader(i, "value", e.target.value)} placeholder={t("headerValue")} className="input" />
              <button type="button" onClick={() => removeHeader(i)} className="btn btn-small btn-danger" title={t("removeHeader")}>×</button>
            </div>
          ))}
          <div>
            <button type="button" onClick={addHeader} className="btn btn-small btn-outline">{t("addHeader")}</button>
          </div>
        </div>

        <div className="field-row" style={{ marginTop: 14, justifyContent: "space-between" }}>
          <div className="field-row">
            <button type="button" className="btn btn-small btn-outline" disabled={!form.url.trim() || testing === testKey} onClick={() => handleTest(testKey, form, id)}>
              {testing === testKey ? "..." : t("testConnection")}
            </button>
            {status(testResult[testKey])}
          </div>
          {isAdmin && (
            <label className="check">
              <input type="checkbox" checked={form.global} onChange={(e) => setForm({ ...form, global: e.target.checked })} />
              {t("globalHint")}
            </label>
          )}
        </div>
      </>
    );
  };

  const renderDatasourcesTab = () => (
    <>
      <div className="panel">
        <div className="panel-head">
          <h3 className="panel-title">{t("dsListTitle")}</h3>
          {!showNew && <button onClick={() => setShowNew(true)} className="btn btn-small btn-primary">{t("addDatasource")}</button>}
        </div>
        <p className="panel-desc">{t("dsListDesc")}</p>

        {showNew && (
          <div className="ds-card" style={{ marginBottom: 14 }}>
            <div className="ds-card-body">
              <div className="panel-head" style={{ marginBottom: 10 }}><h3 className="panel-title">{t("dsNewTitle")}</h3></div>
              {renderDSForm(newDS, setNewDS, "new")}
              <div className="panel-actions">
                <button onClick={() => { setShowNew(false); setNewDS(emptyForm()); }} className="btn btn-small btn-outline">{t("cancelShort")}</button>
                <button onClick={handleCreate} disabled={saving || !newDS.name.trim() || !newDS.url.trim()} className="btn btn-small btn-primary">{saving ? "..." : t("save")}</button>
              </div>
            </div>
          </div>
        )}

        <div className="ds-list">
          {datasources.length === 0 && !showNew && <p className="empty">{t("noDatasourceHint")}</p>}
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
                  {renderDSForm(editingDS, setEditingDS as (f: DSForm) => void, ds.id, ds.id)}
                  <div className="panel-actions">
                    <button onClick={() => { setExpandedId(null); setEditingDS(null); }} className="btn btn-small btn-outline">{t("cancelShort")}</button>
                    <button onClick={handleUpdate} disabled={saving || !editingDS.name.trim() || !editingDS.url.trim()} className="btn btn-small btn-primary">{saving ? "..." : t("save")}</button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </>
  );

  const renderExportTab = () => (
    <div className="panel">
      <div className="panel-head"><h3 className="panel-title">{t("exportTabTitle")}</h3></div>
      <p className="panel-desc">{t("exportTabDesc")}</p>
      <div className="form-grid cols-3">
        <div className="field">
          <label className="label">{t("retentionDaysLabel")}</label>
          <input type="number" min={0} value={retDays} onChange={(e) => setRetDays(Math.max(0, Number(e.target.value)))} className="input" />
        </div>
        <div className="field">
          <label className="label">{t("quotaLabel")}</label>
          <input type="number" min={0} step={0.1} value={quotaGB} onChange={(e) => setQuotaGB(Math.max(0, Number(e.target.value)))} className="input" />
        </div>
        <div className="field">
          <label className="label">{t("concurrencyLabel")}</label>
          <input type="number" min={1} max={64} value={maxConc} onChange={(e) => setMaxConc(Math.min(64, Math.max(1, Number(e.target.value))))} className="input" />
        </div>
        <div className="field span-2">
          <label className="label">{t("publicUrlLabel")}</label>
          <input value={publicUrl} onChange={(e) => setPublicUrl(e.target.value)} placeholder={t("publicUrlPlaceholder")} className="input" />
        </div>
        <div className="field">
          <label className="label">{t("chunkAuto")}</label>
          <div className="hint" style={{ paddingTop: 8 }}>{t("chunkInfo", { mb: settings?.export_chunk_mb ?? 100 })}</div>
        </div>
      </div>
      {saveBar()}
    </div>
  );

  const renderLdapTab = () => (
    <div className="panel">
      <div className="panel-head">
        <h3 className="panel-title">{t("ldapTitle")}</h3>
        <label className="check">
          <input type="checkbox" checked={ldap.enabled} onChange={(e) => setLdap({ ...ldap, enabled: e.target.checked })} />
          {t("ldapEnabled")}
        </label>
      </div>
      <p className="panel-desc">{t("ldapTabDesc")} {t("ldapOrder")}</p>

      <div className="panel-sub first">{t("ldapServerSection")}</div>
      <div className="form-grid">
        <div className="field">
          <label className="label">{t("ldapUrl")}</label>
          <input value={ldap.url} onChange={(e) => setLdap({ ...ldap, url: e.target.value })} placeholder="ldaps://dc01.example.com:636" className="input" />
          <div className="hint">{t("ldapUrlHint")}</div>
        </div>
        <div className="field">
          <label className="label">{t("ldapTimeout")}</label>
          <input type="number" min={1} max={120} value={ldap.timeout_sec} onChange={(e) => setLdap({ ...ldap, timeout_sec: Number(e.target.value) })} className="input" style={{ maxWidth: 120 }} />
          <div className="check-group" style={{ marginTop: 6 }}>
            <label className="check"><input type="checkbox" checked={ldap.starttls} onChange={(e) => setLdap({ ...ldap, starttls: e.target.checked })} />{t("ldapStartTLS")}</label>
            <label className="check"><input type="checkbox" checked={ldap.insecure_skip_verify} onChange={(e) => setLdap({ ...ldap, insecure_skip_verify: e.target.checked })} />{t("ldapSkipVerify")}</label>
          </div>
        </div>
      </div>

      <div className="panel-sub">{t("ldapServiceSection")}</div>
      <div className="form-grid">
        <div className="field">
          <label className="label">{t("ldapBindDn")}</label>
          <input value={ldap.bind_dn} onChange={(e) => setLdap({ ...ldap, bind_dn: e.target.value })} placeholder="uid=svc-logs,cn=users,cn=accounts,dc=example,dc=com" className="input" />
          <div className="hint">{t("ldapBindDnHint")}</div>
        </div>
        <div className="field">
          <label className="label">{t("ldapBindPassword")}</label>
          <div className="field-row">
            <input type="password" value={ldap.bind_password || ""} onChange={(e) => { setLdap({ ...ldap, bind_password: e.target.value }); setLdapClearPass(false); }} placeholder={ldapHasPass && !ldapClearPass ? t("tokenSaved") : t("ldapBindPassword")} className="input" autoComplete="new-password" />
            {ldapHasPass && <button type="button" className="btn btn-small btn-outline" onClick={() => { setLdapClearPass(true); setLdap({ ...ldap, bind_password: "" }); }}>{ldapClearPass ? "✓ " : ""}{t("clearValue")}</button>}
          </div>
        </div>
      </div>

      <div className="panel-sub">{t("ldapSearchSection")}</div>
      <div className="form-grid">
        <div className="field">
          <label className="label">{t("ldapBaseDn")}</label>
          <input value={ldap.base_dn} onChange={(e) => setLdap({ ...ldap, base_dn: e.target.value })} placeholder="cn=users,cn=accounts,dc=example,dc=com" className="input" />
        </div>
        <div className="field">
          <label className="label">{t("ldapUserFilter")}</label>
          <input value={ldap.user_filter} onChange={(e) => setLdap({ ...ldap, user_filter: e.target.value })} placeholder="(&(objectClass=user)(sAMAccountName=%s))" className="input" />
          <div className="hint">{t("ldapUserFilterHint")}</div>
        </div>
        <div className="field">
          <label className="label">{t("ldapUsernameAttr")}</label>
          <input value={ldap.username_attr} onChange={(e) => setLdap({ ...ldap, username_attr: e.target.value })} placeholder="sAMAccountName / uid" className="input" />
        </div>
        <div className="field">
          <label className="label">{t("ldapEmailAttr")}</label>
          <input value={ldap.email_attr} onChange={(e) => setLdap({ ...ldap, email_attr: e.target.value })} placeholder="mail" className="input" />
        </div>
      </div>

      <div className="panel-sub">{t("ldapGroupsSection")}</div>
      <div className="form-grid">
        <div className="field">
          <label className="label">{t("ldapAdminGroup")}</label>
          <input value={ldap.admin_group_dn} onChange={(e) => setLdap({ ...ldap, admin_group_dn: e.target.value })} placeholder="cn=logadmins,cn=groups,cn=accounts,dc=example,dc=com" className="input" />
        </div>
        <div className="field">
          <label className="label">{t("ldapAllowedGroup")}</label>
          <input value={ldap.allowed_group_dn} onChange={(e) => setLdap({ ...ldap, allowed_group_dn: e.target.value })} placeholder="cn=logusers,cn=groups,cn=accounts,dc=example,dc=com" className="input" />
        </div>
        <div className="field">
          <label className="label">{t("ldapDefaultRole")}</label>
          <select value={ldap.default_role} onChange={(e) => setLdap({ ...ldap, default_role: e.target.value as "admin" | "user" })} className="input ds-select" style={{ maxWidth: 220 }}>
            <option value="user">{t("roleUser")}</option>
            <option value="admin">{t("roleAdmin")}</option>
          </select>
        </div>
        <div className="field">
          <label className="label">&nbsp;</label>
          <div className="hint">{t("ldapGroupHint")}</div>
        </div>
      </div>

      <div className="panel-sub">{t("ldapTestSection")}</div>
      <div className="field-row">
        <button type="button" className="btn btn-small btn-outline" disabled={ldapTesting || !ldap.url.trim()} onClick={() => testLdap(false)}>{ldapTesting ? "..." : t("ldapTestConn")}</button>
        <input value={ldapTestUser} onChange={(e) => setLdapTestUser(e.target.value)} placeholder={t("ldapTestUser")} className="input" style={{ flex: "0 1 200px" }} autoComplete="off" />
        <input type="password" value={ldapTestPass} onChange={(e) => setLdapTestPass(e.target.value)} placeholder={t("ldapTestPass")} className="input" style={{ flex: "0 1 200px" }} autoComplete="new-password" />
        <button type="button" className="btn btn-small btn-outline" disabled={ldapTesting || !ldap.url.trim() || !ldapTestUser || !ldapTestPass} onClick={() => testLdap(true)}>{ldapTesting ? "..." : t("ldapTestLogin")}</button>
      </div>
      {ldapTestMsg && <div style={{ marginTop: 8 }}>{status(ldapTestMsg)}</div>}
      {saveBar()}
    </div>
  );

  const renderNotifyTab = () => (
    <div className="panel">
      <div className="panel-head"><h3 className="panel-title">{t("notifyTitle")}</h3></div>
      <p className="panel-desc">{t("notifyTabDesc")}</p>

      <div className="panel-sub first">{t("notifyTelegramSection")}</div>
      <div className="form-grid">
        <div className="field span-2">
          <label className="label">{t("telegramToken")}</label>
          <div className="field-row">
            <input type="password" value={tgToken} onChange={(e) => { setTgToken(e.target.value); setClearTg(false); }} placeholder={settings?.notify.has_telegram_token && !clearTg ? t("tokenSaved") : "123456:ABC..."} className="input" autoComplete="new-password" />
            {settings?.notify.has_telegram_token && (
              <button type="button" className="btn btn-small btn-outline" onClick={() => { setClearTg(true); setTgToken(""); }}>{clearTg ? "✓ " : ""}{t("clearValue")}</button>
            )}
            <button type="button" className="btn btn-small btn-outline" onClick={() => testNotify("telegram")}>{t("test")}</button>
            {status(notifyTestMsg.telegram)}
          </div>
        </div>
      </div>

      <div className="panel-sub">{t("notifyWebhookSection")}</div>
      <div className="form-grid">
        <div className="field span-2">
          <label className="label">{t("webhookUrl")}</label>
          <div className="field-row">
            <input value={webhook} onChange={(e) => setWebhook(e.target.value)} placeholder="https://..." className="input" />
            <button type="button" className="btn btn-small btn-outline" onClick={() => testNotify("webhook")}>{t("test")}</button>
            {status(notifyTestMsg.webhook)}
          </div>
        </div>
      </div>

      <div className="panel-sub">{t("notifySmtpSection")}</div>
      <div className="form-grid cols-3">
        <div className="field">
          <label className="label">{t("smtpHost")}</label>
          <input value={smtp.host} onChange={(e) => setSmtp({ ...smtp, host: e.target.value })} placeholder="smtp.example.com" className="input" />
        </div>
        <div className="field">
          <label className="label">{t("smtpPort")}</label>
          <input type="number" value={smtp.port} onChange={(e) => setSmtp({ ...smtp, port: Number(e.target.value) })} placeholder="587" className="input" />
        </div>
        <div className="field">
          <label className="label">{t("smtpFrom")}</label>
          <input value={smtp.from} onChange={(e) => setSmtp({ ...smtp, from: e.target.value })} placeholder="logs@example.com" className="input" />
        </div>
        <div className="field">
          <label className="label">{t("smtpUser")}</label>
          <input value={smtp.username} onChange={(e) => setSmtp({ ...smtp, username: e.target.value })} className="input" autoComplete="off" />
        </div>
        <div className="field">
          <label className="label">{t("smtpPassword")}</label>
          <div className="field-row">
            <input type="password" value={smtp.password} onChange={(e) => { setSmtp({ ...smtp, password: e.target.value }); setClearSmtpPass(false); }} placeholder={settings?.notify.smtp.has_password && !clearSmtpPass ? t("tokenSaved") : t("smtpPassword")} className="input" autoComplete="new-password" />
            {settings?.notify.smtp.has_password && (
              <button type="button" className="btn btn-small btn-outline" onClick={() => { setClearSmtpPass(true); setSmtp({ ...smtp, password: "" }); }}>{clearSmtpPass ? "✓ " : ""}{t("clearValue")}</button>
            )}
          </div>
        </div>
        <div className="field">
          <label className="label">&nbsp;</label>
          <div className="field-row">
            <label className="check">
              <input type="checkbox" checked={smtp.starttls} onChange={(e) => setSmtp({ ...smtp, starttls: e.target.checked })} />
              {t("smtpStartTLS")}
            </label>
            <button type="button" className="btn btn-small btn-outline" onClick={() => testNotify("email")}>{t("test")}</button>
            {status(notifyTestMsg.email)}
          </div>
        </div>
      </div>
      {saveBar()}
    </div>
  );

  if (loading) return <div className="page">{t("loading")}</div>;

  const tabs: { key: Tab; label: TKey }[] = isAdmin
    ? [
        { key: "datasources", label: "tabDatasources" },
        { key: "export", label: "tabExport" },
        { key: "ldap", label: "tabLdap" },
        { key: "notify", label: "tabNotify" },
      ]
    : [{ key: "datasources", label: "tabDatasources" }];

  return (
    <div className="page">
      <h1>{t("settingsTitle")}</h1>
      <p className="subtitle">{t("settingsSubtitle")}</p>

      {tabs.length > 1 && (
        <div className="tabs">
          {tabs.map((tb) => (
            <button key={tb.key} type="button" className={`tab ${tab === tb.key ? "active" : ""}`} onClick={() => setTab(tb.key)}>
              {t(tb.label)}
            </button>
          ))}
        </div>
      )}

      {tab === "datasources" && renderDatasourcesTab()}
      {isAdmin && settings && tab === "export" && renderExportTab()}
      {isAdmin && settings && tab === "ldap" && renderLdapTab()}
      {isAdmin && settings && tab === "notify" && renderNotifyTab()}
    </div>
  );
}
