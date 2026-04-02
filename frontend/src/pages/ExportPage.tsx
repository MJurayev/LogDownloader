import { useState, useEffect } from "react";
import { api, type Datasource } from "../api";
import "./Pages.css";

export default function ExportPage() {
  const [query, setQuery] = useState("");
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [selectedDS, setSelectedDS] = useState("");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => {
    api.getSettings().then((s) => {
      const ds = s.datasources || [];
      setDatasources(ds);
      if (ds.length > 0) setSelectedDS(ds[0].id);
    });
  }, []);

  const handleExport = async () => {
    if (!query.trim() || !selectedDS) return;
    setLoading(true);
    setMessage("");
    try {
      const res = await api.startExport(query, selectedDS);
      setMessage(`Export boshlandi! Job ID: ${res.job_id}`);
      setQuery("");
    } catch {
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
        {datasources.length > 0 && (
          <div className="ds-selector">
            <label className="filter-label">Datasource</label>
            <select
              value={selectedDS}
              onChange={(e) => setSelectedDS(e.target.value)}
              className="input ds-select"
            >
              {datasources.map((ds) => (
                <option key={ds.id} value={ds.id}>{ds.name || ds.url}</option>
              ))}
            </select>
          </div>
        )}
        <textarea
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder='Masalan: _time:5m AND level:"error"'
          rows={5}
          className="query-input"
        />
        <button
          onClick={handleExport}
          disabled={loading || !query.trim() || !selectedDS}
          className="btn btn-primary"
        >
          {loading ? "Yuklanmoqda..." : "Export"}
        </button>
      </div>

      {message && <div className="message">{message}</div>}
    </div>
  );
}
