import { useState, useEffect } from "react";
import { api, type SavedQuery } from "../api";
import "./Pages.css";

export default function QueriesPage() {
  const [queries, setQueries] = useState<SavedQuery[]>([]);
  const [name, setName] = useState("");
  const [query, setQuery] = useState("");
  const [message, setMessage] = useState("");

  const loadQueries = async () => {
    const data = await api.getQueries();
    setQueries(data || []);
  };

  useEffect(() => {
    loadQueries();
  }, []);

  const handleSave = async () => {
    if (!name.trim() || !query.trim()) return;
    await api.createQuery(name, query);
    setName("");
    setQuery("");
    loadQueries();
  };

  const handleDelete = async (id: string) => {
    await api.deleteQuery(id);
    loadQueries();
  };

  const handleExport = async (q: string) => {
    try {
      const res = await api.startExport(q);
      setMessage(`Export boshlandi! Job ID: ${res.job_id}`);
    } catch {
      setMessage("Xatolik yuz berdi");
    }
  };

  return (
    <div className="page">
      <h1>Saved Queries</h1>

      <div className="form-group">
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Query nomi"
          className="input"
        />
        <textarea
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="LogsQL query"
          rows={3}
          className="query-input"
        />
        <button
          onClick={handleSave}
          disabled={!name.trim() || !query.trim()}
          className="btn btn-primary"
        >
          Saqlash
        </button>
      </div>

      {message && <div className="message">{message}</div>}

      <div className="list">
        {queries.length === 0 && (
          <p className="empty">Hozircha saqlangan querylar yo'q</p>
        )}
        {queries.map((q) => (
          <div key={q.id} className="card">
            <div className="card-header">
              <strong>{q.name}</strong>
              <div className="card-actions">
                <button
                  onClick={() => handleExport(q.query)}
                  className="btn btn-small btn-primary"
                >
                  Export
                </button>
                <button
                  onClick={() => handleDelete(q.id)}
                  className="btn btn-small btn-danger"
                >
                  O'chirish
                </button>
              </div>
            </div>
            <code className="query-code">{q.query}</code>
          </div>
        ))}
      </div>
    </div>
  );
}
