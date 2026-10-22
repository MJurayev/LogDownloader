import { useEffect, useState } from "react";
import { api, type AuditEntry } from "../api";
import { useT } from "../i18n";
import "./Pages.css";

const PAGE = 100;

export default function AuditPage() {
  const { t } = useT();
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [action, setAction] = useState("");
  const [user, setUser] = useState("");
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");

  const load = async (off = offset, a = action, u = user) => {
    setLoading(true);
    setErr("");
    try {
      const r = await api.getAudit(PAGE, off, a, u);
      setEntries(r.entries);
      setTotal(r.total);
      setOffset(off);
    } catch (e) {
      setErr(typeof e === "string" ? e : t("genericError"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const applyFilter = () => load(0, action, user);

  return (
    <div className="page">
      <h1>{t("auditTitle")}</h1>
      <p className="subtitle">{t("auditSubtitle")}</p>

      <div className="audit-filters">
        <input value={action} onChange={(e) => setAction(e.target.value)} onKeyDown={(e) => e.key === "Enter" && applyFilter()} placeholder={t("auditFilterAction")} className="input" list="audit-actions" />
        <datalist id="audit-actions">
          {["login.ok", "login.fail", "login.blocked", "export.start", "job.delete", "job.download", "share.create", "share.revoke", "share.download", "schedule.", "user.", "datasource.", "settings.update", "profile.update", "password.change", "query."].map((a) => (
            <option key={a} value={a} />
          ))}
        </datalist>
        <input value={user} onChange={(e) => setUser(e.target.value)} onKeyDown={(e) => e.key === "Enter" && applyFilter()} placeholder={t("auditFilterUser")} className="input" />
        <button className="btn btn-small btn-primary" onClick={applyFilter} disabled={loading}>{loading ? "..." : t("search")}</button>
        <button className="btn btn-small btn-outline" onClick={() => load(offset)} disabled={loading}>{t("refresh")}</button>
        <button className="btn btn-small btn-outline" onClick={() => api.downloadAudit().catch(() => {})}>{t("auditDownload")}</button>
        <span className="audit-total">{t("auditTotal", { n: total })}</span>
      </div>

      {err && <div className="error-banner">{err}</div>}

      {entries.length === 0 ? (
        <p className="empty">{t("auditEmpty")}</p>
      ) : (
        <div className="log-table-wrap">
          <table className="log-table audit-table">
            <thead>
              <tr>
                <th>{t("auditTime")}</th>
                <th>{t("auditUser")}</th>
                <th>{t("auditAction")}</th>
                <th>{t("auditTarget")}</th>
                <th>{t("auditIp")}</th>
                <th>{t("auditDetails")}</th>
              </tr>
            </thead>
            <tbody>
              {entries.map((e, i) => (
                <tr key={i} className={`log-row ${e.action.includes("fail") || e.action.includes("blocked") ? "log-warn" : ""}`}>
                  <td><span className="log-time">{new Date(e.time).toLocaleString()}</span></td>
                  <td>{e.user || "—"}</td>
                  <td><span className={`audit-action audit-${e.action.split(".")[0]}`}>{e.action}</span></td>
                  <td className="audit-target" title={e.target}>{e.target || ""}</td>
                  <td>{e.ip || ""}</td>
                  <td className="audit-details" title={e.details}>{e.details || ""}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="pagination">
        <button onClick={() => load(Math.max(0, offset - PAGE))} disabled={offset === 0 || loading} className="btn btn-small btn-outline">{t("prev")}</button>
        <span className="page-num">{t("page", { n: Math.floor(offset / PAGE) + 1 })}</span>
        <button onClick={() => load(offset + PAGE)} disabled={offset + PAGE >= total || loading} className="btn btn-small btn-outline">{t("next")}</button>
      </div>
    </div>
  );
}
