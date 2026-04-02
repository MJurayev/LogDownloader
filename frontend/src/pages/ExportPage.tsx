import { useState } from "react";
import { api } from "../api";
import "./Pages.css";

export default function ExportPage() {
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  const handleExport = async () => {
    if (!query.trim()) return;
    setLoading(true);
    setMessage("");
    try {
      const res = await api.startExport(query);
      setMessage(`Export boshlandi! Job ID: ${res.job_id}`);
      setQuery("");
    } catch (err) {
      setMessage("Xatolik yuz berdi");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="page">
      <h1>Log Export</h1>
      <p className="subtitle">LogsQL query yozing va exportni boshlang</p>

      <div className="export-form">
        <textarea
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder='Masalan: _time:5m AND level:"error"'
          rows={5}
          className="query-input"
        />
        <button
          onClick={handleExport}
          disabled={loading || !query.trim()}
          className="btn btn-primary"
        >
          {loading ? "Yuklanmoqda..." : "Export"}
        </button>
      </div>

      {message && <div className="message">{message}</div>}
    </div>
  );
}
