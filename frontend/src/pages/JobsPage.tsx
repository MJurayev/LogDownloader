import { useState, useEffect, useRef } from "react";
import { api, type ExportJob, type ShareLink } from "../api";
import { useT, type TKey } from "../i18n";
import "./Pages.css";

const EXPIRY_OPTIONS: { labelKey: TKey; minutes: number | null }[] = [
  { labelKey: "hours1", minutes: 60 },
  { labelKey: "hours6", minutes: 6 * 60 },
  { labelKey: "hours24", minutes: 24 * 60 },
  { labelKey: "days7", minutes: 7 * 24 * 60 },
  { labelKey: "unlimited", minutes: null },
];

const LIMIT_OPTIONS: { label: string; labelKey?: TKey; value: number }[] = [
  { label: "1", value: 1 },
  { label: "5", value: 5 },
  { label: "10", value: 10 },
  { label: "50", value: 50 },
  { label: "", labelKey: "unlimited", value: 0 },
];

export default function JobsPage() {
  const { t } = useT();
  const [jobs, setJobs] = useState<ExportJob[]>([]);
  const intervalRef = useRef<number | undefined>(undefined);
  const currentUser = JSON.parse(localStorage.getItem("user") || "{}") as { username?: string; role?: string };

  // Share modal state
  const [shareJob, setShareJob] = useState<ExportJob | null>(null);
  const [shareExisting, setShareExisting] = useState<ShareLink | null>(null);
  const [shareLoading, setShareLoading] = useState(false);
  const [shareError, setShareError] = useState("");
  const [shareCopied, setShareCopied] = useState(false);
  const [maxDownloads, setMaxDownloads] = useState<number>(5);
  const [expiryMinutes, setExpiryMinutes] = useState<number | null>(24 * 60);

  const loadJobs = async () => {
    const data = await api.getJobs();
    setJobs(data || []);
  };

  useEffect(() => {
    loadJobs();
    intervalRef.current = window.setInterval(loadJobs, 2000);
    return () => clearInterval(intervalRef.current);
  }, []);

  const handleDelete = async (id: string) => {
    await api.deleteJob(id);
    loadJobs();
  };

  const openShare = async (job: ExportJob) => {
    setShareJob(job);
    setShareError("");
    setShareCopied(false);
    setShareLoading(true);
    try {
      const existing = await api.getShare(job.id);
      setShareExisting(existing);
    } catch {
      setShareExisting(null);
    } finally {
      setShareLoading(false);
    }
  };

  const closeShare = () => {
    setShareJob(null);
    setShareExisting(null);
    setShareError("");
    setShareCopied(false);
  };

  const handleCreateShare = async () => {
    if (!shareJob) return;
    setShareLoading(true);
    setShareError("");
    try {
      const expiresAt =
        expiryMinutes === null
          ? null
          : new Date(Date.now() + expiryMinutes * 60 * 1000).toISOString();
      const share = await api.createShare(shareJob.id, {
        maxDownloads,
        expiresAt,
      });
      setShareExisting(share);
    } catch (err) {
      setShareError(typeof err === "string" ? err : t("shareError"));
    } finally {
      setShareLoading(false);
    }
  };

  const handleRevokeShare = async () => {
    if (!shareJob) return;
    setShareLoading(true);
    try {
      await api.revokeShare(shareJob.id);
      setShareExisting(null);
    } finally {
      setShareLoading(false);
    }
  };

  const copyShareUrl = async () => {
    if (!shareExisting) return;
    try {
      await navigator.clipboard.writeText(shareExisting.url);
      setShareCopied(true);
      setTimeout(() => setShareCopied(false), 1500);
    } catch {
      setShareError(t("clipboardError"));
    }
  };

  const formatBytes = (n: number): string => {
    if (!n) return "—";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(v < 10 && i > 0 ? 1 : 0)} ${units[i]}`;
  };

  const statusBadge = (status: string) => {
    const cls =
      status === "running"
        ? "badge badge-running"
        : status === "done"
        ? "badge badge-done"
        : "badge badge-failed";
    const label =
      status === "running" ? t("statusRunning") : status === "done" ? t("statusDone") : t("statusFailed");
    return <span className={cls}>{label}</span>;
  };

  const running = jobs.filter((j) => j.status === "running");
  const done = jobs.filter((j) => j.status === "done");
  const failed = jobs.filter((j) => j.status === "failed");

  const renderJob = (job: ExportJob) => (
    <div key={job.id} className="card">
      <div className="card-header">
        <div className="job-header-left">
          {statusBadge(job.status)}
          {job.datasource_name && <span className="job-ds-name">{job.datasource_name}</span>}
          {job.owner_name && job.owner_name !== currentUser.username && (
            <span className="job-owner" title={t("owner")}>{job.owner_name}</span>
          )}
          <span className="job-lines">{t("linesCount", { n: job.lines.toLocaleString() })}</span>
          {job.size > 0 && <span className="job-size">{formatBytes(job.size)}</span>}
          {job.format && job.format !== "log" && (
            <span className="job-format-chip">{job.format}</span>
          )}
        </div>
        <div className="card-actions">
          {job.status === "done" && (
            <>
              <button
                onClick={() => openShare(job)}
                className="btn btn-small btn-outline"
                title={t("shareButtonTitle")}
              >
                {t("share")}
              </button>
              <button
                onClick={() => api.downloadJob(job.id, job.file_name)}
                className="btn btn-small btn-primary"
              >
                {t("download")}
              </button>
            </>
          )}
          <button
            onClick={() => handleDelete(job.id)}
            className="btn btn-small btn-danger"
          >
            {t("delete")}
          </button>
        </div>
      </div>
      <div className="job-filename" title={t("fileNameTitle")}>
        {job.file_name}
      </div>
      <code className="query-code">{job.query}</code>
      {(job.start || job.end) && (
        <div className="job-range">
          <span title={job.start ? `UTC: ${job.start}` : ""}>{t("jobFrom")} {job.start ? new Date(job.start).toLocaleString() : "—"}</span>
          <span title={job.end ? `UTC: ${job.end}` : ""}>{t("jobTo")} {job.end ? new Date(job.end).toLocaleString() : "—"}</span>
        </div>
      )}
      {job.error && <div className="error-text">{job.error}</div>}
      <div className="job-time">
        {new Date(job.created_at).toLocaleString()}
      </div>
    </div>
  );

  return (
    <div className="page">
      <h1>{t("jobsTitle")}</h1>
      <p className="subtitle">
        {t("jobsTotal", { n: jobs.length })}
        {running.length > 0 && <span className="jobs-counter running">{t("runningCount", { n: running.length })}</span>}
        {done.length > 0 && <span className="jobs-counter done">{t("doneCount", { n: done.length })}</span>}
        {failed.length > 0 && <span className="jobs-counter failed">{t("failedCount", { n: failed.length })}</span>}
      </p>

      {jobs.length === 0 && (
        <p className="empty">{t("noJobs")}</p>
      )}

      {running.length > 0 && (
        <div className="jobs-section">
          <h3 className="jobs-section-title">
            <span className="section-dot dot-running" />
            {t("sectionRunning")} ({running.length})
          </h3>
          <div className="list">{running.map(renderJob)}</div>
        </div>
      )}

      {done.length > 0 && (
        <div className="jobs-section">
          <h3 className="jobs-section-title">
            <span className="section-dot dot-done" />
            {t("sectionDone")} ({done.length})
          </h3>
          <div className="list">{done.map(renderJob)}</div>
        </div>
      )}

      {failed.length > 0 && (
        <div className="jobs-section">
          <h3 className="jobs-section-title">
            <span className="section-dot dot-failed" />
            {t("sectionFailed")} ({failed.length})
          </h3>
          <div className="list">{failed.map(renderJob)}</div>
        </div>
      )}

      {shareJob && (
        <div className="save-modal-overlay" onClick={closeShare}>
          <div className="save-modal share-modal" onClick={(e) => e.stopPropagation()}>
            <h3>{t("shareTitle")}</h3>
            <p className="share-modal-sub">
              <code>{shareJob.file_name}</code>
            </p>

            {shareExisting ? (
              <>
                <div className="form-group">
                  <label className="label">{t("link")}</label>
                  <div className="share-url-row">
                    <input value={shareExisting.url} readOnly className="input" />
                    <button onClick={copyShareUrl} className="btn btn-small btn-primary">
                      {shareCopied ? t("copied") : t("copy")}
                    </button>
                  </div>
                </div>
                <div className="share-stats">
                  <div>
                    <span className="share-stat-label">{t("downloadsStat")}</span>
                    <span className="share-stat-value">
                      {shareExisting.download_count}
                      {shareExisting.max_downloads > 0 ? ` / ${shareExisting.max_downloads}` : " / ∞"}
                    </span>
                  </div>
                  <div>
                    <span className="share-stat-label">{t("expiryStat")}</span>
                    <span className="share-stat-value">
                      {shareExisting.expires_at
                        ? new Date(shareExisting.expires_at).toLocaleString()
                        : t("unlimited")}
                    </span>
                  </div>
                </div>
                <div className="share-warning">
                  {t("shareWarning")}
                </div>
                {shareError && <div className="error-text">{shareError}</div>}
                <div className="save-modal-actions">
                  <button onClick={closeShare} className="btn btn-small btn-outline">
                    {t("close")}
                  </button>
                  <button
                    onClick={handleRevokeShare}
                    disabled={shareLoading}
                    className="btn btn-small btn-danger"
                  >
                    {shareLoading ? "..." : t("revoke")}
                  </button>
                </div>
              </>
            ) : (
              <>
                <div className="form-group">
                  <label className="label">{t("maxDownloads")}</label>
                  <select
                    value={maxDownloads}
                    onChange={(e) => setMaxDownloads(Number(e.target.value))}
                    className="input ds-select"
                  >
                    {LIMIT_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>{o.labelKey ? t(o.labelKey) : o.label}</option>
                    ))}
                  </select>
                </div>
                <div className="form-group">
                  <label className="label">{t("expiryStat")}</label>
                  <select
                    value={String(expiryMinutes)}
                    onChange={(e) => {
                      const v = e.target.value;
                      setExpiryMinutes(v === "null" ? null : Number(v));
                    }}
                    className="input ds-select"
                  >
                    {EXPIRY_OPTIONS.map((o) => (
                      <option key={o.labelKey} value={o.minutes === null ? "null" : String(o.minutes)}>
                        {t(o.labelKey)}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="share-warning">
                  {t("shareWarningAuto")}
                </div>
                {shareError && <div className="error-text">{shareError}</div>}
                <div className="save-modal-actions">
                  <button onClick={closeShare} disabled={shareLoading} className="btn btn-small btn-outline">
                    {t("cancel")}
                  </button>
                  <button
                    onClick={handleCreateShare}
                    disabled={shareLoading}
                    className="btn btn-small btn-primary"
                  >
                    {shareLoading ? "..." : t("create")}
                  </button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
