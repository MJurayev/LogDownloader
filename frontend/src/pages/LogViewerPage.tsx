import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import DatePicker from "react-datepicker";
import "react-datepicker/dist/react-datepicker.css";
import { api, type SavedQuery, type Datasource } from "../api";
import "./Pages.css";

const PAGE_SIZE = 50;

// Brauzer TZ qisqartmasi (masalan: "GMT+05:00")
const browserTZ = (() => {
  try {
    const parts = new Intl.DateTimeFormat("en", { timeZoneName: "shortOffset" }).formatToParts(new Date());
    return parts.find((p) => p.type === "timeZoneName")?.value || "";
  } catch {
    return "";
  }
})();

// UTC ISO yoki shunga o'xshash string'ni mahalliy YYYY-MM-DD HH:mm:ss formatga aylantiradi.
const fmtLocalTime = (utc: string): string => {
  if (!utc) return "";
  const d = new Date(utc);
  if (isNaN(d.getTime())) return utc;
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
};


export default function LogViewerPage() {
  const navigate = useNavigate();
  const [query, setQuery] = useState("*");
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [selectedDS, setSelectedDS] = useState("");

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
  const [saveGlobal, setSaveGlobal] = useState(false);
  const [savingQuery, setSavingQuery] = useState(false);
  const isAdmin = JSON.parse(localStorage.getItem("user") || "{}").role === "admin";

  // Saved queries
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>([]);
  const [activeQueryId, setActiveQueryId] = useState<string | null>(null);

  // Time range. null = cheklov yo'q (barcha loglar).
  const [timeStart, setTimeStart] = useState<Date | null>(null);
  const [timeEnd, setTimeEnd] = useState<Date | null>(null);

  // Sort tartibi (_time bo'yicha). Default: asc — eski loglar yuqorida.
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">("asc");

  // Edit saved query modal
  const [editingQuery, setEditingQuery] = useState<SavedQuery | null>(null);
  const [editQueryName, setEditQueryName] = useState("");
  const [editQueryText, setEditQueryText] = useState("");
  const [editQueryDS, setEditQueryDS] = useState("");
  const [editQueryGlobal, setEditQueryGlobal] = useState(false);
  const [editQuerySaving, setEditQuerySaving] = useState(false);
  const [editQueryMsg, setEditQueryMsg] = useState("");

  const loadSavedQueries = async () => {
    const data = await api.getQueries();
    setSavedQueries(data || []);
  };

  useEffect(() => {
    loadSavedQueries();
    api.getDatasources().then((ds) => {
      setDatasources(ds || []);
      if (ds && ds.length > 0) setSelectedDS(ds[0].id);
    });
  }, []);

  const handleSelectQuery = (sq: SavedQuery) => {
    setQuery(sq.query);
    setActiveQueryId(sq.id);
    if (sq.datasource_id) setSelectedDS(sq.datasource_id);
  };

  const handleDeleteQuery = async (id: string) => {
    await api.deleteQuery(id);
    if (activeQueryId === id) setActiveQueryId(null);
    loadSavedQueries();
  };

  const openEditQuery = (sq: SavedQuery) => {
    setEditingQuery(sq);
    setEditQueryName(sq.name);
    setEditQueryText(sq.query);
    setEditQueryDS(sq.datasource_id || "");
    setEditQueryGlobal(sq.global);
    setEditQueryMsg("");
  };

  const closeEditQuery = () => {
    setEditingQuery(null);
    setEditQueryMsg("");
  };

  const handleUpdateQuery = async () => {
    if (!editingQuery || !editQueryName.trim() || !editQueryText.trim()) return;
    setEditQuerySaving(true);
    try {
      await api.updateQuery(
        editingQuery.id,
        editQueryName,
        editQueryText,
        editQueryDS || undefined,
        editQueryGlobal,
      );
      await loadSavedQueries();
      setEditQueryMsg("Saqlandi!");
      setTimeout(() => { closeEditQuery(); setEditQuerySaving(false); }, 800);
    } catch {
      setEditQueryMsg("Xatolik yuz berdi");
      setEditQuerySaving(false);
    }
  };

  const handleSaveQuery = async () => {
    if (!saveName.trim() || !query.trim()) return;
    setSavingQuery(true);
    try {
      await api.createQuery(saveName, query, selectedDS, saveGlobal);
      loadSavedQueries();
      setSaveMsg("Saqlandi!");
      setTimeout(() => { setShowSaveModal(false); setSaveMsg(""); setSaveName(""); setSaveGlobal(false); setSavingQuery(false); }, 1000);
    } catch {
      setSaveMsg("Xatolik yuz berdi");
      setSavingQuery(false);
    }
  };

  const toISO = (d: Date | null): string | undefined => d?.toISOString();

  const applyPreset = (minutes: number) => {
    const now = new Date();
    setTimeStart(new Date(now.getTime() - minutes * 60 * 1000));
    setTimeEnd(now);
  };

  const clearTimeRange = () => {
    setTimeStart(null);
    setTimeEnd(null);
  };

  const fetchLogs = async (pageNum: number) => {
    setLoading(true);
    setError("");
    try {
      const res = await api.queryLogs(
        query,
        selectedDS,
        PAGE_SIZE,
        pageNum * PAGE_SIZE,
        toISO(timeStart),
        toISO(timeEnd),
        sortOrder,
      );
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

  const toggleSortOrder = () => {
    const next: "asc" | "desc" = sortOrder === "asc" ? "desc" : "asc";
    setSortOrder(next);
    // Agar qidiruv allaqachon bajarilgan bo'lsa, yangi tartibda birinchi page'ni qayta yuklash.
    if (searched) {
      (async () => {
        setLoading(true);
        setError("");
        try {
          const res = await api.queryLogs(
            query,
            selectedDS,
            PAGE_SIZE,
            0,
            toISO(timeStart),
            toISO(timeEnd),
            next,
          );
          setLogs(res.logs || []);
          setHasMore(res.has_more);
          setTotal(res.total);
          setPage(0);
          setExpandedRow(null);
        } catch (err) {
          setError(typeof err === "string" ? err : "So'rov xatosi");
          setLogs([]);
        } finally {
          setLoading(false);
        }
      })();
    }
  };

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
                <div className="sq-item-name">{sq.name}{sq.global && <span className="global-badge">G</span>}</div>
                <div className="sq-item-query">{sq.query}</div>
                <div className="sq-item-actions">
                  <button
                    className="sq-item-edit"
                    onClick={(e) => { e.stopPropagation(); openEditQuery(sq); }}
                    title="Tahrirlash"
                  >
                    ✎
                  </button>
                  <button
                    className="sq-item-delete"
                    onClick={(e) => { e.stopPropagation(); handleDeleteQuery(sq.id); }}
                    title="O'chirish"
                  >
                    &times;
                  </button>
                </div>
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
          <div className="time-range-row">
            <div className="time-range-inputs">
              <div className="time-range-field">
                <label className="filter-label">From</label>
                <DatePicker
                  selected={timeStart}
                  onChange={(d: Date | null) => setTimeStart(d)}
                  selectsStart
                  startDate={timeStart}
                  endDate={timeEnd}
                  maxDate={timeEnd ?? undefined}
                  showTimeSelect
                  timeIntervals={5}
                  timeFormat="HH:mm"
                  dateFormat="yyyy-MM-dd HH:mm"
                  placeholderText="Hammasi (cheksiz)"
                  isClearable
                  className="input time-input"
                  popperClassName="time-popper"
                />
              </div>
              <div className="time-range-field">
                <label className="filter-label">To</label>
                <DatePicker
                  selected={timeEnd}
                  onChange={(d: Date | null) => setTimeEnd(d)}
                  selectsEnd
                  startDate={timeStart}
                  endDate={timeEnd}
                  minDate={timeStart ?? undefined}
                  showTimeSelect
                  timeIntervals={5}
                  timeFormat="HH:mm"
                  dateFormat="yyyy-MM-dd HH:mm"
                  placeholderText="Hammasi (hozirgacha)"
                  isClearable
                  className="input time-input"
                  popperClassName="time-popper"
                />
              </div>
            </div>
            <div className="time-presets">
              <button type="button" className="btn btn-small btn-outline" onClick={() => applyPreset(5)}>5m</button>
              <button type="button" className="btn btn-small btn-outline" onClick={() => applyPreset(60)}>1h</button>
              <button type="button" className="btn btn-small btn-outline" onClick={() => applyPreset(6 * 60)}>6h</button>
              <button type="button" className="btn btn-small btn-outline" onClick={() => applyPreset(24 * 60)}>24h</button>
              <button
                type="button"
                className="btn btn-small btn-outline"
                onClick={clearTimeRange}
                disabled={!timeStart && !timeEnd}
                title="Vaqt cheklovini olib tashlash (barcha loglar)"
              >
                Hammasi
              </button>
            </div>
          </div>
          <div className="time-tz-hint">
            Mahalliy vaqt ({browserTZ}) — server'ga UTC'da yuboriladi. Jadvaldagi <code>_time</code> ham mahalliyga aylantirilgan (asl UTC qiymat tooltip'da).
          </div>
          <div className="ds-selector">
            <label className="filter-label">Datasource</label>
            {datasources.length > 0 ? (
              <select
                value={selectedDS}
                onChange={(e) => setSelectedDS(e.target.value)}
                className="input ds-select"
              >
                {datasources.map((ds) => (
                  <option key={ds.id} value={ds.id}>{ds.name || ds.url}</option>
                ))}
              </select>
            ) : (
              <span className="ds-empty-hint">Settings dan datasource qo'shing</span>
            )}
          </div>
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
                if (!query.trim() || !selectedDS) return;
                await api.startExport(query, selectedDS, toISO(timeStart), toISO(timeEnd), sortOrder);
                navigate("/jobs");
              }}
              disabled={!query.trim() || !selectedDS}
              className="btn btn-small btn-export-action"
            >
              Export
            </button>
          </div>

          {editingQuery && (
            <div className="save-modal-overlay" onClick={closeEditQuery}>
              <div className="save-modal" onClick={(e) => e.stopPropagation()}>
                <h3>Queryni tahrirlash</h3>
                <div className="form-group">
                  <label className="label">Nomi</label>
                  <input
                    value={editQueryName}
                    onChange={(e) => setEditQueryName(e.target.value)}
                    placeholder="Query nomi"
                    className="input"
                    autoFocus
                  />
                </div>
                <div className="form-group">
                  <label className="label">Query</label>
                  <textarea
                    value={editQueryText}
                    onChange={(e) => setEditQueryText(e.target.value)}
                    rows={3}
                    className="query-input"
                  />
                </div>
                {datasources.length > 0 && (
                  <div className="form-group">
                    <label className="label">Datasource</label>
                    <select
                      value={editQueryDS}
                      onChange={(e) => setEditQueryDS(e.target.value)}
                      className="input ds-select"
                    >
                      <option value="">— tanlanmagan —</option>
                      {datasources.map((ds) => (
                        <option key={ds.id} value={ds.id}>{ds.name || ds.url}</option>
                      ))}
                    </select>
                  </div>
                )}
                {isAdmin && (
                  <label className="global-check">
                    <input
                      type="checkbox"
                      checked={editQueryGlobal}
                      onChange={(e) => setEditQueryGlobal(e.target.checked)}
                    />
                    Global (barcha userlarga ko'rinadi)
                  </label>
                )}
                <div className="save-modal-actions">
                  <button onClick={closeEditQuery} disabled={editQuerySaving} className="btn btn-small btn-outline">
                    Bekor qilish
                  </button>
                  <button
                    onClick={handleUpdateQuery}
                    disabled={editQuerySaving || !editQueryName.trim() || !editQueryText.trim()}
                    className="btn btn-small btn-primary"
                  >
                    {editQuerySaving ? "..." : "Saqlash"}
                  </button>
                </div>
                {editQueryMsg && <div className="save-modal-msg">{editQueryMsg}</div>}
              </div>
            </div>
          )}

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
                {isAdmin && (
                  <label className="global-check">
                    <input type="checkbox" checked={saveGlobal} onChange={(e) => setSaveGlobal(e.target.checked)} />
                    Global (barcha userlarga ko'rinadi)
                  </label>
                )}
                <div className="save-modal-actions">
                  <button onClick={() => setShowSaveModal(false)} disabled={savingQuery} className="btn btn-small btn-outline">
                    Bekor qilish
                  </button>
                  <button onClick={handleSaveQuery} disabled={savingQuery || !saveName.trim()} className="btn btn-small btn-primary">
                    {savingQuery ? "..." : "Saqlash"}
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
                        <th key={col}>
                          {col === "_time" ? (
                            <button
                              type="button"
                              className="th-sort-btn"
                              onClick={toggleSortOrder}
                              title={`_time bo'yicha ${sortOrder === "asc" ? "kamayish" : "o'sish"} tartibida saralash`}
                            >
                              {col} <span className="th-sort-arrow">{sortOrder === "asc" ? "▲" : "▼"}</span>
                            </button>
                          ) : col}
                        </th>
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
                                <span className="log-time" title={`UTC: ${log[col]}`}>{fmtLocalTime(log[col])}</span>
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
