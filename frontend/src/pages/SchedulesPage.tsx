import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, type Datasource, type ExportContent, type ExportFormat, type Profile, type Schedule, type ScheduleInput } from "../api";
import { useT, type TKey } from "../i18n";
import "./Pages.css";

const CRON_PRESETS: { key: TKey; cron: string }[] = [
  { key: "cronHourly", cron: "0 * * * *" },
  { key: "cronDaily", cron: "5 0 * * *" },
  { key: "cronWeekly", cron: "5 0 * * 1" },
];

const LOOKBACK_OPTIONS: { labelKey: TKey; minutes: number }[] = [
  { labelKey: "hours1", minutes: 60 },
  { labelKey: "hours6", minutes: 360 },
  { labelKey: "hours24", minutes: 1440 },
  { labelKey: "days7", minutes: 10080 },
];

const ARCHIVE_SPLIT_OPTIONS = [0, 500, 1024, 2048, 4096];

const emptyForm = (dsId: string): ScheduleInput => ({
  name: "",
  query: "*",
  datasource_id: dsId,
  cron: "5 0 * * *",
  lookback_minutes: 1440,
  align: true,
  sort_order: "asc",
  format: "tar.gz",
  content: "ndjson",
  fields: [],
  archive_split_mb: 0,
  notify: false,
  auto_share: false,
  enabled: true,
});

export default function SchedulesPage() {
  const { t } = useT();
  const navigate = useNavigate();
  const currentUser = JSON.parse(localStorage.getItem("user") || "{}") as { username?: string; role?: string };
  const [list, setList] = useState<Schedule[]>([]);
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [profile, setProfile] = useState<Profile | null>(null);
  const [editing, setEditing] = useState<ScheduleInput | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [fieldsText, setFieldsText] = useState("");
  const [cronMode, setCronMode] = useState<string>("5 0 * * *");
  const [msg, setMsg] = useState("");
  const [saving, setSaving] = useState(false);
  const [runMsg, setRunMsg] = useState<Record<string, string>>({});

  const load = async () => {
    const [s, ds] = await Promise.all([api.getSchedules(), api.getDatasources()]);
    setList(s || []);
    setDatasources(ds || []);
  };

  useEffect(() => {
    load();
    api.me().then(setProfile).catch(() => setProfile(null));
    const id = window.setInterval(load, 10000);
    return () => clearInterval(id);
  }, []);

  const openNew = () => {
    const f = emptyForm(datasources[0]?.id || "");
    setEditing(f);
    setEditingId(null);
    setFieldsText("");
    setCronMode(f.cron);
    setMsg("");
  };

  const openEdit = (s: Schedule) => {
    setEditing({
      name: s.name,
      query: s.query,
      datasource_id: s.datasource_id,
      cron: s.cron,
      lookback_minutes: s.lookback_minutes,
      align: s.align,
      sort_order: s.sort_order || "asc",
      format: s.format || "tar.gz",
      content: s.content || "ndjson",
      fields: s.fields || [],
      archive_split_mb: s.archive_split_mb || 0,
      notify: s.notify,
      auto_share: s.auto_share,
      enabled: s.enabled,
    });
    setEditingId(s.id);
    setFieldsText((s.fields || []).join(", "));
    setCronMode(CRON_PRESETS.some((p) => p.cron === s.cron) ? s.cron : "custom");
    setMsg("");
  };

  const save = async () => {
    if (!editing) return;
    setSaving(true);
    setMsg("");
    const body: ScheduleInput = {
      ...editing,
      fields: fieldsText.split(",").map((s) => s.trim()).filter(Boolean),
      archive_split_mb: editing.format === "tar.gz" ? editing.archive_split_mb : 0,
    };
    try {
      if (editingId) await api.updateSchedule(editingId, body);
      else await api.createSchedule(body);
      setEditing(null);
      setEditingId(null);
      await load();
    } catch (err) {
      setMsg(typeof err === "string" ? err : t("genericError"));
    } finally {
      setSaving(false);
    }
  };

  const remove = async (id: string) => {
    await api.deleteSchedule(id);
    load();
  };

  const runNow = async (id: string) => {
    setRunMsg((m) => ({ ...m, [id]: "..." }));
    try {
      const r = await api.runSchedule(id);
      setRunMsg((m) => ({ ...m, [id]: t("runStarted", { id: r.job_id }) }));
      load();
    } catch (err) {
      setRunMsg((m) => ({ ...m, [id]: typeof err === "string" ? err : t("genericError") }));
    }
  };

  const toggleEnabled = async (s: Schedule) => {
    await api.updateSchedule(s.id, {
      name: s.name, query: s.query, datasource_id: s.datasource_id, cron: s.cron, lookback_minutes: s.lookback_minutes, align: s.align,
      sort_order: s.sort_order, format: s.format, content: s.content, fields: s.fields || [], archive_split_mb: s.archive_split_mb || 0,
      notify: s.notify, auto_share: s.auto_share, enabled: !s.enabled,
    });
    load();
  };

  const dsName = (id: string) => datasources.find((d) => d.id === id)?.name || id;
  const canNotify = !!(profile && (profile.email || profile.telegram_chat_id));
  const fmt = (s?: string) => (s ? new Date(s).toLocaleString() : t("never"));

  return (
    <div className="page">
      <h1>{t("schedulesTitle")}</h1>
      <p className="subtitle">{t("schedulesSubtitle")}</p>

      {list.length === 0 && !editing && <p className="empty">{t("noSchedules")}</p>}

      <div className="list">
        {list.map((s) => (
          <div key={s.id} className="card">
            <div className="card-header">
              <div className="job-header-left">
                <span className={`badge ${s.enabled ? "badge-done" : "badge-failed"}`}>{s.enabled ? t("enabled") : t("disabled")}</span>
                <strong>{s.name}</strong>
                <span className="job-ds-name">{dsName(s.datasource_id)}</span>
                {s.owner_name && s.owner_name !== currentUser.username && <span className="job-owner" title={t("owner")}>{s.owner_name}</span>}
                <code className="sched-cron">{s.cron}</code>
                <span className="job-size">{s.lookback_minutes >= 1440 ? `${s.lookback_minutes / 1440}d` : `${s.lookback_minutes / 60}h`}{s.align ? " ⌁" : ""}</span>
                {s.format && s.format !== "log" && <span className="job-format-chip">{s.format}</span>}
                {s.content && s.content !== "ndjson" && <span className="job-format-chip">{s.content}</span>}
              </div>
              <div className="card-actions">
                <button onClick={() => runNow(s.id)} className="btn btn-small btn-primary">{t("runNow")}</button>
                <button onClick={() => toggleEnabled(s)} className="btn btn-small btn-outline">{s.enabled ? t("disabled") : t("enabled")}</button>
                <button onClick={() => openEdit(s)} className="btn btn-small btn-outline">{t("edit")}</button>
                <button onClick={() => remove(s.id)} className="btn btn-small btn-danger">{t("delete")}</button>
              </div>
            </div>
            <code className="query-code">{s.query}</code>
            <div className="job-range">
              <span>{t("nextRun")}: {s.enabled ? fmt(s.next_run_at) : "—"}</span>
              <span>{t("lastRun")}: {fmt(s.last_run_at)}</span>
              {s.last_job_id && (
                <span>
                  {t("lastJob")}: <a href="/jobs" onClick={(e) => { e.preventDefault(); navigate("/jobs"); }}>{s.last_job_id}</a>
                </span>
              )}
            </div>
            {s.last_error && <div className="error-text">{t("lastError")}: {s.last_error}</div>}
            {runMsg[s.id] && <div className="field-hint">{runMsg[s.id]}</div>}
          </div>
        ))}
      </div>

      {!editing && <button onClick={openNew} className="btn btn-outline" style={{ marginTop: 12 }} disabled={datasources.length === 0}>{t("addSchedule")}</button>}

      {editing && (
        <div className="save-modal-overlay" onClick={() => setEditing(null)}>
          <div className="save-modal sched-modal" onClick={(e) => e.stopPropagation()}>
            <h3>{editingId ? t("editSchedule") : t("addSchedule")}</h3>
            <div className="form-group">
              <label className="label">{t("scheduleName")}</label>
              <input value={editing.name} onChange={(e) => setEditing({ ...editing, name: e.target.value })} className="input" autoFocus />
            </div>
            <div className="form-group">
              <label className="label">{t("datasource")}</label>
              <select value={editing.datasource_id} onChange={(e) => setEditing({ ...editing, datasource_id: e.target.value })} className="input ds-select">
                {datasources.map((d) => (
                  <option key={d.id} value={d.id}>{d.name || d.url}</option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <label className="label">{t("query")}</label>
              <textarea value={editing.query} onChange={(e) => setEditing({ ...editing, query: e.target.value })} rows={2} className="query-input" />
            </div>
            <div className="export-options-grid">
              <div className="form-group">
                <label className="label">{t("cronLabel")}</label>
                <select value={cronMode} onChange={(e) => { const v = e.target.value; setCronMode(v); if (v !== "custom") setEditing({ ...editing, cron: v }); }} className="input ds-select">
                  {CRON_PRESETS.map((p) => (
                    <option key={p.cron} value={p.cron}>{t(p.key)}</option>
                  ))}
                  <option value="custom">{t("cronCustom")}</option>
                </select>
                {cronMode === "custom" && (
                  <input value={editing.cron} onChange={(e) => setEditing({ ...editing, cron: e.target.value })} placeholder="*/30 * * * *" className="input" style={{ marginTop: 6 }} />
                )}
                <div className="field-hint">{t("cronHint")}</div>
              </div>
              <div className="form-group">
                <label className="label">{t("lookbackLabel")}</label>
                <select value={editing.lookback_minutes} onChange={(e) => setEditing({ ...editing, lookback_minutes: Number(e.target.value) })} className="input ds-select">
                  {LOOKBACK_OPTIONS.map((o) => (
                    <option key={o.minutes} value={o.minutes}>{t(o.labelKey)}</option>
                  ))}
                </select>
                <label className="global-check" style={{ marginTop: 6 }}>
                  <input type="checkbox" checked={editing.align} onChange={(e) => setEditing({ ...editing, align: e.target.checked })} />
                  {t("alignLabel")}
                </label>
                <div className="field-hint">{t("alignHint")}</div>
              </div>
              <div className="form-group">
                <label className="label">{t("formatSelectTitle")}</label>
                <select value={editing.format} onChange={(e) => setEditing({ ...editing, format: e.target.value as ExportFormat })} className="input ds-select">
                  <option value="tar.gz">.tar.gz</option>
                  <option value="log.gz">.log.gz</option>
                  <option value="log">{t("formatRaw")}</option>
                </select>
              </div>
              <div className="form-group">
                <label className="label">{t("contentLabel")}</label>
                <select value={editing.content} onChange={(e) => setEditing({ ...editing, content: e.target.value as ExportContent })} className="input ds-select">
                  <option value="ndjson">{t("contentNdjson")}</option>
                  <option value="msg">{t("contentMsg")}</option>
                  <option value="csv">{t("contentCsv")}</option>
                </select>
              </div>
              <div className="form-group">
                <label className="label">{t("fieldsLabel")}</label>
                <input value={fieldsText} onChange={(e) => setFieldsText(e.target.value)} placeholder={t("fieldsPlaceholder")} className="input" disabled={editing.content === "msg"} />
              </div>
              <div className="form-group">
                <label className="label">{t("archiveSplitLabel")}</label>
                <select value={editing.archive_split_mb} onChange={(e) => setEditing({ ...editing, archive_split_mb: Number(e.target.value) })} className="input ds-select" disabled={editing.format !== "tar.gz"}>
                  {ARCHIVE_SPLIT_OPTIONS.map((mb) => (
                    <option key={mb} value={mb}>{mb === 0 ? t("archiveSplitNone") : mb >= 1024 ? `${mb / 1024} GB` : `${mb} MB`}</option>
                  ))}
                </select>
              </div>
            </div>
            <div className="export-checks">
              <label className="global-check" title={canNotify ? "" : t("notifyNoTarget")}>
                <input type="checkbox" checked={editing.notify && canNotify} disabled={!canNotify} onChange={(e) => setEditing({ ...editing, notify: e.target.checked })} />
                {t("notifyLabel")}
              </label>
              <label className="global-check">
                <input type="checkbox" checked={editing.auto_share} onChange={(e) => setEditing({ ...editing, auto_share: e.target.checked })} />
                {t("autoShareLabel")}
              </label>
              <label className="global-check">
                <input type="checkbox" checked={editing.enabled} onChange={(e) => setEditing({ ...editing, enabled: e.target.checked })} />
                {t("enabledLabel")}
              </label>
            </div>
            {msg && <div className="error-text">{msg}</div>}
            <div className="save-modal-actions">
              <button onClick={() => setEditing(null)} disabled={saving} className="btn btn-small btn-outline">{t("cancel")}</button>
              <button onClick={save} disabled={saving || !editing.name.trim() || !editing.query.trim() || !editing.datasource_id || !editing.cron.trim()} className="btn btn-small btn-primary">
                {saving ? "..." : t("save")}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
