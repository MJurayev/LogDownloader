import { useState, useEffect, useRef } from "react";
import { api, type ExportJob, type ShareLink } from "../api";
import "./Pages.css";

const EXPIRY_OPTIONS: { label: string; minutes: number | null }[] = [
  { label: "1 soat", minutes: 60 },
  { label: "6 soat", minutes: 6 * 60 },
  { label: "24 soat", minutes: 24 * 60 },
  { label: "7 kun", minutes: 7 * 24 * 60 },
  { label: "Cheksiz", minutes: null },
];

const LIMIT_OPTIONS: { label: string; value: number }[] = [
  { label: "1", value: 1 },
  { label: "5", value: 5 },
  { label: "10", value: 10 },
  { label: "50", value: 50 },
  { label: "Cheksiz", value: 0 },
];

export default function JobsPage() {
  const [jobs, setJobs] = useState<ExportJob[]>([]);
  const intervalRef = useRef<number | undefined>(undefined);

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
      setShareError(typeof err === "string" ? err : "Share yaratishda xatolik");
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
      setShareError("Clipboard'ga ko'chirib bo'lmadi");
    }
  };

  const statusBadge = (status: string) => {
    const cls =
      status === "running"
        ? "badge badge-running"
        : status === "done"
        ? "badge badge-done"
        : "badge badge-failed";
    return <span className={cls}>{status.toUpperCase()}</span>;
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
          <span className="job-lines">{job.lines.toLocaleString()} qator</span>
        </div>
        <div className="card-actions">
          {job.status === "done" && (
            <>
              <button
                onClick={() => openShare(job)}
                className="btn btn-small btn-outline"
                title="Share link yaratish / boshqarish"
              >
                Share
              </button>
              <button
                onClick={() => api.downloadJob(job.id, job.file_name)}
                className="btn btn-small btn-primary"
              >
                Yuklab olish
              </button>
            </>
          )}
          <button
            onClick={() => handleDelete(job.id)}
            className="btn btn-small btn-danger"
          >
            O'chirish
          </button>
        </div>
      </div>
      <div className="job-filename" title="Yuklab olinadigan fayl nomi">
        {job.file_name}
      </div>
      <code className="query-code">{job.query}</code>
      {(job.start || job.end) && (
        <div className="job-range">
          <span title={job.start ? `UTC: ${job.start}` : ""}>From: {job.start ? new Date(job.start).toLocaleString() : "—"}</span>
          <span title={job.end ? `UTC: ${job.end}` : ""}>To: {job.end ? new Date(job.end).toLocaleString() : "—"}</span>
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
      <h1>Jobs</h1>
      <p className="subtitle">
        Jami {jobs.length} ta job
        {running.length > 0 && <span className="jobs-counter running">{running.length} running</span>}
        {done.length > 0 && <span className="jobs-counter done">{done.length} done</span>}
        {failed.length > 0 && <span className="jobs-counter failed">{failed.length} failed</span>}
      </p>

      {jobs.length === 0 && (
        <p className="empty">Hozircha hech qanday job yo'q</p>
      )}

      {running.length > 0 && (
        <div className="jobs-section">
          <h3 className="jobs-section-title">
            <span className="section-dot dot-running" />
            Running ({running.length})
          </h3>
          <div className="list">{running.map(renderJob)}</div>
        </div>
      )}

      {done.length > 0 && (
        <div className="jobs-section">
          <h3 className="jobs-section-title">
            <span className="section-dot dot-done" />
            Done ({done.length})
          </h3>
          <div className="list">{done.map(renderJob)}</div>
        </div>
      )}

      {failed.length > 0 && (
        <div className="jobs-section">
          <h3 className="jobs-section-title">
            <span className="section-dot dot-failed" />
            Failed ({failed.length})
          </h3>
          <div className="list">{failed.map(renderJob)}</div>
        </div>
      )}

      {shareJob && (
        <div className="save-modal-overlay" onClick={closeShare}>
          <div className="save-modal share-modal" onClick={(e) => e.stopPropagation()}>
            <h3>Share link</h3>
            <p className="share-modal-sub">
              <code>{shareJob.file_name}</code>
            </p>

            {shareExisting ? (
              <>
                <div className="form-group">
                  <label className="label">Link</label>
                  <div className="share-url-row">
                    <input value={shareExisting.url} readOnly className="input" />
                    <button onClick={copyShareUrl} className="btn btn-small btn-primary">
                      {shareCopied ? "Ko'chirildi!" : "Copy"}
                    </button>
                  </div>
                </div>
                <div className="share-stats">
                  <div>
                    <span className="share-stat-label">Yuklab olishlar</span>
                    <span className="share-stat-value">
                      {shareExisting.download_count}
                      {shareExisting.max_downloads > 0 ? ` / ${shareExisting.max_downloads}` : " / ∞"}
                    </span>
                  </div>
                  <div>
                    <span className="share-stat-label">Muddati</span>
                    <span className="share-stat-value">
                      {shareExisting.expires_at
                        ? new Date(shareExisting.expires_at).toLocaleString()
                        : "Cheksiz"}
                    </span>
                  </div>
                </div>
                <div className="share-warning">
                  Limit yetganda yoki muddat o'tganda fayl o'chiriladi.
                </div>
                {shareError && <div className="error-text">{shareError}</div>}
                <div className="save-modal-actions">
                  <button onClick={closeShare} className="btn btn-small btn-outline">
                    Yopish
                  </button>
                  <button
                    onClick={handleRevokeShare}
                    disabled={shareLoading}
                    className="btn btn-small btn-danger"
                  >
                    {shareLoading ? "..." : "Bekor qilish (Revoke)"}
                  </button>
                </div>
              </>
            ) : (
              <>
                <div className="form-group">
                  <label className="label">Maksimal yuklab olishlar</label>
                  <select
                    value={maxDownloads}
                    onChange={(e) => setMaxDownloads(Number(e.target.value))}
                    className="input ds-select"
                  >
                    {LIMIT_OPTIONS.map((o) => (
                      <option key={o.value} value={o.value}>{o.label}</option>
                    ))}
                  </select>
                </div>
                <div className="form-group">
                  <label className="label">Muddati</label>
                  <select
                    value={String(expiryMinutes)}
                    onChange={(e) => {
                      const v = e.target.value;
                      setExpiryMinutes(v === "null" ? null : Number(v));
                    }}
                    className="input ds-select"
                  >
                    {EXPIRY_OPTIONS.map((o) => (
                      <option key={o.label} value={o.minutes === null ? "null" : String(o.minutes)}>
                        {o.label}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="share-warning">
                  Limit yetganda yoki muddat o'tganda fayl avtomatik o'chiriladi.
                </div>
                {shareError && <div className="error-text">{shareError}</div>}
                <div className="save-modal-actions">
                  <button onClick={closeShare} disabled={shareLoading} className="btn btn-small btn-outline">
                    Bekor qilish
                  </button>
                  <button
                    onClick={handleCreateShare}
                    disabled={shareLoading}
                    className="btn btn-small btn-primary"
                  >
                    {shareLoading ? "..." : "Yaratish"}
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
