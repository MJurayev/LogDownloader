import { useState, useEffect, useRef } from "react";
import { api, type ExportJob } from "../api";
import "./Pages.css";

export default function JobsPage() {
  const [jobs, setJobs] = useState<ExportJob[]>([]);
  const intervalRef = useRef<number>();

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

  const statusBadge = (status: string) => {
    const cls =
      status === "running"
        ? "badge badge-running"
        : status === "done"
        ? "badge badge-done"
        : "badge badge-failed";
    return <span className={cls}>{status.toUpperCase()}</span>;
  };

  return (
    <div className="page">
      <h1>Active Jobs</h1>
      <p className="subtitle">Export jarayonlari holati</p>

      <div className="list">
        {jobs.length === 0 && (
          <p className="empty">Hozircha hech qanday job yo'q</p>
        )}
        {jobs.map((job) => (
          <div key={job.id} className="card">
            <div className="card-header">
              <div>
                {statusBadge(job.status)}
                <span className="job-lines">{job.lines} qator</span>
              </div>
              <div className="card-actions">
                {job.status === "done" && (
                  <a
                    href={api.downloadJob(job.id)}
                    className="btn btn-small btn-primary"
                    download
                  >
                    Yuklab olish
                  </a>
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
            {job.error && <div className="error-text">{job.error}</div>}
            <div className="job-time">
              {new Date(job.created_at).toLocaleString()}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
