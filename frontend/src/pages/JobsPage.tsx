import { useState, useEffect, useRef } from "react";
import { api, type ExportJob, type ShareLink, type Stats } from "../api";
import { useT, formatBytes, formatDuration, type TKey } from "../i18n";
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

// progress — foiz (0..100) yoki null (taxmin yo'q).
function progressOf(job: ExportJob): number | null {
  if (job.windows_total && job.windows_total > 1) {
    const w = (job.windows_done || 0) / job.windows_total;
    if (job.total_estimate && job.total_estimate > 0) {
      return Math.min(99, Math.max(w * 100, (job.lines / job.total_estimate) * 100));
    }
    return Math.min(99, w * 100);
  }
  if (job.total_estimate && job.total_estimate > 0) return Math.min(99, (job.lines / job.total_estimate) * 100);
  return null;
}

function etaSeconds(job: ExportJob, now: number): number | null {
  if (!job.started_at || !job.total_estimate || job.lines <= 0) return null;
  const elapsed = (now - new Date(job.started_at).getTime()) / 1000;
  if (elapsed <= 0) return null;
  const rate = job.lines / elapsed;
  const remaining = Math.max(0, job.total_estimate - job.lines);
  return remaining / rate;
}

export default function JobsPage() {
  const { t } = useT();
  const [jobs, setJobs] = useState<ExportJob[]>([]);
  const [stats, setStats] = useState<Stats | null>(null);
  const [now, setNow] = useState(Date.now());
  const intervalRef = useRef<number | undefined>(undefined);
  const currentUser = JSON.parse(localStorage.getItem("user") || "{}") as { username?: string; role?: string };
  const [downloadErr, setDownloadErr] = useState("");

  // Share modal state
  const [shareJob, setShareJob] = useState<ExportJob | null>(null);
  const [shareExisting, setShareExisting] = useState<ShareLink | null>(null);
  const [shareLoading, setShareLoading] = useState(false);
  const [shareError, setShareError] = useState("");
  const [shareCopied, setShareCopied] = useState<string | null>(null);
  const [maxDownloads, setMaxDownloads] = useState<number>(5);
  const [expiryMinutes, setExpiryMinutes] = useState<number | null>(24 * 60);

  const loadJobs = async () => {
    const [data, st] = await Promise.all([api.getJobs(), api.getStats().catch(() => null)]);
    setJobs(data || []);
    if (st) setStats(st);
    setNow(Date.now());
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

  const handleDownload = async (job: ExportJob, fileName?: string) => {
    setDownloadErr("");
    try {
      await api.downloadJob(job.id, fileName || job.file_name);
    } catch (err) {
      setDownloadErr(err instanceof Error ? err.message : String(err));
    }
  };

  const openShare = async (job: ExportJob) => {
    setShareJob(job);
    setShareError("");
    setShareCopied(null);
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
    setShareCopied(null);
  };

  const handleCreateShare = async () => {
    if (!shareJob) return;
    setShareLoading(true);
    setShareError("");
    try {
      const expiresAt = expiryMinutes === null ? null : new Date(Date.now() + expiryMinutes * 60 * 1000).toISOString();
      const share = await api.createShare(shareJob.id, { maxDownloads, expiresAt });
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

  const copyText = async (text: string, key: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setShareCopied(key);
      setTimeout(() => setShareCopied(null), 1500);
    } catch {
      setShareError(t("clipboardError"));
    }
  };

  const statusBadge = (status: string) => {
    const cls = status === "running" ? "badge badge-running" : status === "done" ? "badge badge-done" : status === "queued" ? "badge badge-queued" : "badge badge-failed";
    const label = status === "running" ? t("statusRunning") : status === "done" ? t("statusDone") : status === "queued" ? t("statusQueued") : t("statusFailed");
    return <span className={cls}>{label}</span>;
  };

  const queued = jobs.filter((j) => j.status === "queued");
  const running = jobs.filter((j) => j.status === "running");
  const done = jobs.filter((j) => j.status === "done");
  const failed = jobs.filter((j) => j.status === "failed");

  const renderProgress = (job: ExportJob) => {
    const pct = progressOf(job);
    const eta = etaSeconds(job, now);
    const elapsed = job.started_at ? (now - new Date(job.started_at).getTime()) / 1000 : null;
    return (
      <div className="job-progress">
        <div className="progress-bar">
          <div className={`progress-fill ${pct === null ? "indeterminate" : ""}`} style={pct === null ? undefined : { width: `${pct}%` }} />
        </div>
        <div className="job-progress-meta">
          {job.total_estimate ? <span>{t("progressLines", { done: job.lines.toLocaleString(), total: job.total_estimate.toLocaleString() })}</span> : <span>{t("linesCount", { n: job.lines.toLocaleString() })}</span>}
          {pct !== null && <span>{Math.floor(pct)}%</span>}
          {job.windows_total ? <span>{t("windowsProgress", { done: job.windows_done || 0, total: job.windows_total })}</span> : null}
          {elapsed !== null && <span>{t("elapsedLabel", { t: formatDuration(elapsed) })}</span>}
          {eta !== null && <span>{t("etaLabel", { t: formatDuration(eta) })}</span>}
          {job.retries ? <span className="job-retries">{t("retriesCount", { n: job.retries })}</span> : null}
        </div>
      </div>
    );
  };

  const renderJob = (job: ExportJob) => (
    <div key={job.id} className="card">
      <div className="card-header">
        <div className="job-header-left">
          {statusBadge(job.status)}
          {job.datasource_name && <span className="job-ds-name">{job.datasource_name}</span>}
          {job.owner_name && job.owner_name !== currentUser.username && (
            <span className="job-owner" title={t("owner")}>{job.owner_name}</span>
          )}
          {job.schedule_id && <span className="job-parts-chip" title={job.schedule_id}>{t("scheduleBadge")}</span>}
          {job.status !== "running" && <span className="job-lines">{t("linesCount", { n: job.lines.toLocaleString() })}</span>}
          {job.size > 0 && <span className="job-size">{formatBytes(job.size)}</span>}
          {job.format && job.format !== "log" && <span className="job-format-chip">{job.format}</span>}
          {job.content && job.content !== "ndjson" && <span className="job-format-chip">{job.content}</span>}
          {(job.parts ?? 0) > 1 && <span className="job-parts-chip" title={t("chunkAuto")}>{t("partsCount", { n: job.parts! })}</span>}
          {(job.files?.length ?? 0) > 1 && <span className="job-parts-chip">{t("filesCount", { n: job.files!.length })}</span>}
        </div>
        <div className="card-actions">
          {job.status === "done" && (
            <>
              <button onClick={() => openShare(job)} className="btn btn-small btn-outline" title={t("shareButtonTitle")}>{t("share")}</button>
              {(job.files?.length ?? 0) <= 1 && (
                <button onClick={() => handleDownload(job)} className="btn btn-small btn-primary">{t("download")}</button>
              )}
            </>
          )}
          <button onClick={() => handleDelete(job.id)} className="btn btn-small btn-danger">{t("delete")}</button>
        </div>
      </div>
      <div className="job-filename" title={t("fileNameTitle")}>{job.file_name}</div>
      {job.status === "done" && (job.files?.length ?? 0) > 1 && (
        <div className="job-files">
          {job.files!.map((f) => (
            <button key={f.name} type="button" className="btn btn-small btn-outline job-file-btn" onClick={() => handleDownload(job, f.name)} title={t("download")}>
              ⤓ {f.name} <span className="job-size">{formatBytes(f.size)}</span>
            </button>
          ))}
        </div>
      )}
      {job.status === "running" && renderProgress(job)}
      <code className="query-code">{job.query}</code>
      {(job.start || job.end) && (
        <div className="job-range">
          <span title={job.start ? `UTC: ${job.start}` : ""}>{t("jobFrom")} {job.start ? new Date(job.start).toLocaleString() : "—"}</span>
          <span title={job.end ? `UTC: ${job.end}` : ""}>{t("jobTo")} {job.end ? new Date(job.end).toLocaleString() : "—"}</span>
        </div>
      )}
      {job.share_url && (
        <div className="job-share-url">
          <span className="share-stat-label">{t("autoShareLink")}:</span> <a href={job.share_url} target="_blank" rel="noreferrer">{job.share_url}</a>
        </div>
      )}
      {job.error && <div className="error-text">{job.error}</div>}
      <div className="job-time">
        {new Date(job.created_at).toLocaleString()}
        {job.finished_at && job.started_at && <span className="job-duration"> · {formatDuration((new Date(job.finished_at).getTime() - new Date(job.started_at).getTime()) / 1000)}</span>}
      </div>
    </div>
  );

  const section = (titleKey: TKey, dot: string, list: ExportJob[]) =>
    list.length > 0 && (
      <div className="jobs-section">
        <h3 className="jobs-section-title">
          <span className={`section-dot ${dot}`} />
          {t(titleKey)} ({list.length})
        </h3>
        <div className="list">{list.map(renderJob)}</div>
      </div>
    );

  return (
    <div className="page">
      <h1>{t("jobsTitle")}</h1>
      <p className="subtitle">
        {t("jobsTotal", { n: jobs.length })}
        {queued.length > 0 && <span className="jobs-counter queued">{t("queuedCount", { n: queued.length })}</span>}
        {running.length > 0 && <span className="jobs-counter running">{t("runningCount", { n: running.length })}</span>}
        {done.length > 0 && <span className="jobs-counter done">{t("doneCount", { n: done.length })}</span>}
        {failed.length > 0 && <span className="jobs-counter failed">{t("failedCount", { n: failed.length })}</span>}
      </p>

      {stats && (
        <div className="stats-bar">
          <span>
            {t("diskUsage", { used: formatBytes(stats.exports_bytes) })}
            {stats.retention.quota_gb > 0 && t("quotaSuffix", { quota: `${stats.retention.quota_gb} GB` })}
          </span>
          {stats.retention.days > 0 && <span>{t("retentionInfo", { n: stats.retention.days })}</span>}
          <span>{t("queueInfo", { running: stats.worker.running, queued: stats.worker.queued, max: stats.worker.max_concurrent })}</span>
        </div>
      )}
      {downloadErr && <div className="error-banner">{downloadErr}</div>}

      {jobs.length === 0 && <p className="empty">{t("noJobs")}</p>}

      {section("sectionRunning", "dot-running", running)}
      {section("sectionQueued", "dot-queued", queued)}
      {section("sectionDone", "dot-done", done)}
      {section("sectionFailed", "dot-failed", failed)}

      {shareJob && (
        <div className="save-modal-overlay" onClick={closeShare}>
          <div className="save-modal share-modal" onClick={(e) => e.stopPropagation()}>
            <h3>{t("shareTitle")}</h3>
            <p className="share-modal-sub"><code>{shareJob.file_name}</code></p>

            {shareExisting ? (
              <>
                {shareExisting.files.length <= 1 ? (
                  <div className="form-group">
                    <label className="label">{t("link")}</label>
                    <div className="share-url-row">
                      <input value={shareExisting.url} readOnly className="input" />
                      <button onClick={() => copyText(shareExisting.url, "main")} className="btn btn-small btn-primary">
                        {shareCopied === "main" ? t("copied") : t("copy")}
                      </button>
                    </div>
                  </div>
                ) : (
                  <div className="form-group">
                    <label className="label">{t("link")} ({t("filesCount", { n: shareExisting.files.length })})</label>
                    {shareExisting.files.map((f) => (
                      <div key={f.url} className="share-url-row">
                        <input value={f.url} readOnly className="input" title={f.name} />
                        <button onClick={() => copyText(f.url, f.url)} className="btn btn-small btn-primary">
                          {shareCopied === f.url ? t("copied") : t("copy")}
                        </button>
                      </div>
                    ))}
                  </div>
                )}
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
                    <span className="share-stat-value">{shareExisting.expires_at ? new Date(shareExisting.expires_at).toLocaleString() : t("unlimited")}</span>
                  </div>
                </div>
                <div className="share-warning">{t("shareWarning")}</div>
                {shareError && <div className="error-text">{shareError}</div>}
                <div className="save-modal-actions">
                  <button onClick={closeShare} className="btn btn-small btn-outline">{t("close")}</button>
                  <button onClick={handleRevokeShare} disabled={shareLoading} className="btn btn-small btn-danger">{shareLoading ? "..." : t("revoke")}</button>
                </div>
              </>
            ) : (
              <>
                <div className="form-group">
                  <label className="label">{t("maxDownloads")}</label>
                  <select value={maxDownloads} onChange={(e) => setMaxDownloads(Number(e.target.value))} className="input ds-select">
                    {LIMIT_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>{o.labelKey ? t(o.labelKey) : o.label}</option>
                    ))}
                  </select>
                </div>
                <div className="form-group">
                  <label className="label">{t("expiryStat")}</label>
                  <select value={String(expiryMinutes)} onChange={(e) => { const v = e.target.value; setExpiryMinutes(v === "null" ? null : Number(v)); }} className="input ds-select">
                    {EXPIRY_OPTIONS.map((o) => (
                      <option key={o.labelKey} value={o.minutes === null ? "null" : String(o.minutes)}>{t(o.labelKey)}</option>
                    ))}
                  </select>
                </div>
                <div className="share-warning">{t("shareWarningAuto")}</div>
                {shareError && <div className="error-text">{shareError}</div>}
                <div className="save-modal-actions">
                  <button onClick={closeShare} disabled={shareLoading} className="btn btn-small btn-outline">{t("cancel")}</button>
                  <button onClick={handleCreateShare} disabled={shareLoading} className="btn btn-small btn-primary">{shareLoading ? "..." : t("create")}</button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
