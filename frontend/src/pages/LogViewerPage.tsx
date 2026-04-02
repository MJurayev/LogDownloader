import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { api, type SavedQuery } from "../api";
import "./Pages.css";

const PAGE_SIZE = 50;


export default function LogViewerPage() {
  const navigate = useNavigate();
  const [query, setQuery] = useState("*");

  const [logs, setLogs] = useState<Record<string, string>[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [total, setTotal] = useState(0);
  const [searched, setSearched] = useState(false);
  const [expandedRow, setExpandedRow] = useState<number | null>(null);
  const [showSaveModal, setShowSaveModal] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [saveMsg, setSaveMsg] = useState("");

  // Saved queries
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>([]);
  const [activeQueryId, setActiveQueryId] = useState<string | null>(null);

  const loadSavedQueries = async () => {
    const data = await api.getQueries();
    setSavedQueries(data || []);
  };

  useEffect(() => {
    loadSavedQueries();
  }, []);

  const handleSelectQuery = (sq: SavedQuery) => {
    setQuery(sq.query);
    setActiveQueryId(sq.id);
  };

  const handleDeleteQuery = async (id: string) => {
    await api.deleteQuery(id);
    if (activeQueryId === id) setActiveQueryId(null);
    loadSavedQueries();
  };

  const handleSaveQuery = async () => {
    if (!saveName.trim() || !query.trim()) return;
    try {
      await api.createQuery(saveName, query);
      loadSavedQueries();
      setSaveMsg("Saqlandi!");
      setTimeout(() => { setShowSaveModal(false); setSaveMsg(""); setSaveName(""); }, 1000);
    } catch {
      setSaveMsg("Xatolik yuz berdi");
    }
  };

  const fetchLogs = async (pageNum: number) => {
    setLoading(true);
    setError("");
    try {
      const res = await api.queryLogs(query, PAGE_SIZE, pageNum * PAGE_SIZE);
      setLogs(res.logs || []);
      setHasMore(res.has_more);
      setTotal(res.total);
      setPage(pageNum);
      setSearched(true);
      setExpandedRow(null);
    } catch (err) {
      setError(typeof err === "string" ? err : "So'rov xatosi");
      setLogs([]);
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = () => fetchLogs(0);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSearch();
    }
  };

  const getColumns = (): string[] => {
    if (logs.length === 0) return [];
    const priority = ["_time", "_msg", "level", "service", "host"];
    const allKeys = new Set<string>();
    logs.forEach((log) => Object.keys(log).forEach((k) => allKeys.add(k)));
    const sorted = priority.filter((k) => allKeys.has(k));
    allKeys.forEach((k) => {
      if (!priority.includes(k)) sorted.push(k);
    });
    return sorted;
  };

  const columns = getColumns();

  return (
    <div className="log-viewer-layout">
      {/* Saved Queries Panel */}
      <div className="sq-panel">
        <div className="sq-panel-header">Saved Queries</div>
        {savedQueries.length === 0 ? (
          <div className="sq-empty">Querylar yo'q</div>
        ) : (
          <div className="sq-list">
            {savedQueries.map((sq) => (
              <div
                key={sq.id}
                className={`sq-item ${activeQueryId === sq.id ? "active" : ""}`}
                onClick={() => handleSelectQuery(sq)}
              >
                <div className="sq-item-name">{sq.name}</div>
                <div className="sq-item-query">{sq.query}</div>
                <button
                  className="sq-item-delete"
                  onClick={(e) => { e.stopPropagation(); handleDeleteQuery(sq.id); }}
                  title="O'chirish"
                >
                  &times;
                </button>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Main Viewer */}
      <div className="page log-viewer">
        <h1>Log Viewer</h1>
        <p className="subtitle">LogsQL query yozing va loglarni ko'ring</p>

        <div className="viewer-controls">
          <div className="viewer-query-row">
            <textarea
              value={query}
              onChange={(e) => { setQuery(e.target.value); setActiveQueryId(null); }}
              onKeyDown={handleKeyDown}
              placeholder='Masalan: level:"error" AND service:"api-gateway"'
              rows={2}
              className="query-input viewer-query"
            />
            <button
              onClick={handleSearch}
              disabled={loading || !query.trim()}
              className="btn btn-primary btn-search"
            >
              {loading ? "..." : "Search"}
            </button>
          </div>
          <div className="viewer-secondary-actions">
            <button
              onClick={() => { setSaveName(""); setSaveMsg(""); setShowSaveModal(true); }}
              disabled={!query.trim()}
              className="btn btn-small btn-outline"
            >
              Save Query
            </button>
            <button
              onClick={async () => {
                if (!query.trim()) return;
                await api.startExport(query);
                navigate("/jobs");
              }}
              disabled={!query.trim()}
              className="btn btn-small btn-export-action"
            >
              Export
            </button>
          </div>

          {showSaveModal && (
            <div className="save-modal-overlay" onClick={() => setShowSaveModal(false)}>
              <div className="save-modal" onClick={(e) => e.stopPropagation()}>
                <h3>Query saqlash</h3>
                <code className="save-modal-query">{query}</code>
                <input
                  value={saveName}
                  onChange={(e) => setSaveName(e.target.value)}
                  placeholder="Query nomi"
                  className="input"
                  autoFocus
                  onKeyDown={(e) => { if (e.key === "Enter") handleSaveQuery(); }}
                />
                <div className="save-modal-actions">
                  <button onClick={() => setShowSaveModal(false)} className="btn btn-small btn-outline">
                    Bekor qilish
                  </button>
                  <button onClick={handleSaveQuery} disabled={!saveName.trim()} className="btn btn-small btn-primary">
                    Saqlash
                  </button>
                </div>
                {saveMsg && <div className="save-modal-msg">{saveMsg}</div>}
              </div>
            </div>
          )}
        </div>

        {error && <div className="error-banner">{error}</div>}

        {searched && !error && (
          <>
            <div className="viewer-info">
              <span>{total} ta log topildi{hasMore ? "+" : ""}</span>
              <span className="viewer-page-info">
                Sahifa {page + 1} ({page * PAGE_SIZE + 1}-{page * PAGE_SIZE + logs.length})
              </span>
            </div>

            {logs.length > 0 ? (
              <div className="log-table-wrap">
                <table className="log-table">
                  <thead>
                    <tr>
                      {columns.map((col) => (
                        <th key={col}>{col}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {logs.map((log, i) => (
                      <>
                        <tr
                          key={i}
                          className={`log-row ${expandedRow === i ? "expanded" : ""} ${log.level === "error" ? "log-error" : log.level === "warn" ? "log-warn" : ""}`}
                          onClick={() => setExpandedRow(expandedRow === i ? null : i)}
                        >
                          {columns.map((col) => (
                            <td key={col} title={log[col]}>
                              {col === "level" ? (
                                <span className={`log-level log-level-${log[col]}`}>{log[col]}</span>
                              ) : col === "_time" ? (
                                <span className="log-time">{log[col]}</span>
                              ) : (
                                <span className="log-cell">{log[col] ?? ""}</span>
                              )}
                            </td>
                          ))}
                        </tr>
                        {expandedRow === i && (
                          <tr key={`${i}-detail`} className="log-detail-row">
                            <td colSpan={columns.length}>
                              <pre className="log-detail">{JSON.stringify(log, null, 2)}</pre>
                            </td>
                          </tr>
                        )}
                      </>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="empty">Hech qanday log topilmadi</p>
            )}

            <div className="pagination">
              <button
                onClick={() => fetchLogs(page - 1)}
                disabled={page === 0 || loading}
                className="btn btn-small btn-outline"
              >
                Oldingi
              </button>
              <span className="page-num">Sahifa {page + 1}</span>
              <button
                onClick={() => fetchLogs(page + 1)}
                disabled={!hasMore || loading}
                className="btn btn-small btn-outline"
              >
                Keyingi
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
