import { useState, useEffect, useRef } from "react";
import { api, type ExportJob } from "../api";
import "./Pages.css";

export default function JobsPage() {
  const [jobs, setJobs] = useState<ExportJob[]>([]);
  const intervalRef = useRef<number | undefined>(undefined);

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

  const handleDownload = async (id: string) => {
    try {
      await api.downloadJob(id);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Yuklab olishda xatolik");
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
            <button
              onClick={() => handleDownload(job.id)}
              className="btn btn-small btn-primary"
            >
              Yuklab olish
            </button>
          )}
          <button
            onClick={() => handleDelete(job.id)}
            className="btn btn-small btn-danger"
          >
            O'chirish
          </button>
        </div>
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
    </div>
  );
}
