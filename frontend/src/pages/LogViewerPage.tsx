import { Fragment, useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import DatePicker from "react-datepicker";
import "react-datepicker/dist/react-datepicker.css";
import { api, type SavedQuery, type Datasource } from "../api";
import { useT } from "../i18n";
import "./Pages.css";

const PAGE_SIZE = 50;

// Export formati qulflangan: har doim .tar.gz ("Avtomatik"). Tanlash imkoni yo'q.
const EXPORT_FORMAT = "tar.gz" as const;

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
  const { t } = useT();
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

  // Export fayl nomi. Bo'sh bo'lsa server default `export_<vaqt>.tar.gz` beradi.
  const [exportFileName, setExportFileName] = useState("");
  const [exporting, setExporting] = useState(false);

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
      setEditQueryMsg(t("saved"));
      setTimeout(() => { closeEditQuery(); setEditQuerySaving(false); }, 800);
    } catch {
      setEditQueryMsg(t("genericError"));
      setEditQuerySaving(false);
    }
  };

  const handleSaveQuery = async () => {
    if (!saveName.trim() || !query.trim()) return;
    setSavingQuery(true);
    try {
      await api.createQuery(saveName, query, selectedDS, saveGlobal);
      await loadSavedQueries();
      setSaveMsg(t("saved"));
      setTimeout(() => { setShowSaveModal(false); setSaveMsg(""); setSaveName(""); setSaveGlobal(false); setSavingQuery(false); }, 600);
    } catch {
      setSaveMsg(t("genericError"));
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

  const fetchLogs = async (pageNum: number, order: "asc" | "desc" = sortOrder) => {
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
        order,
      );
      setLogs(res.logs || []);
      setHasMore(res.has_more);
      setTotal(res.total);
      setPage(pageNum);
      setSearched(true);
      setExpandedRow(null);
    } catch (err) {
      setError(typeof err === "string" ? err : t("queryError"));
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
    if (searched) fetchLogs(0, next);
  };

  const handleExport = async () => {
    if (!query.trim() || !selectedDS || exporting) return;
    setExporting(true);
    try {
      await api.startExport(query, selectedDS, toISO(timeStart), toISO(timeEnd), sortOrder, exportFileName, EXPORT_FORMAT);
      setExportFileName("");
      navigate("/jobs");
    } catch {
      setError(t("genericError"));
    } finally {
      setExporting(false);
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
        <div className="sq-panel-header">{t("savedQueries")}</div>
        {savedQueries.length === 0 ? (
          <div className="sq-empty">{t("noQueries")}</div>
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
                    title={t("edit")}
                  >
                    ✎
                  </button>
                  <button
                    className="sq-item-delete"
                    onClick={(e) => { e.stopPropagation(); handleDeleteQuery(sq.id); }}
                    title={t("delete")}
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
        <h1>{t("lvTitle")}</h1>
        <p className="subtitle">{t("lvSubtitle")}</p>

        <div className="viewer-controls">
          <div className="time-range-row">
            <div className="time-range-inputs">
              <div className="time-range-field">
                <label className="filter-label">{t("from")}</label>
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
                  placeholderText={t("fromPlaceholder")}
                  isClearable
                  className="input time-input"
                  popperClassName="time-popper"
                />
              </div>
              <div className="time-range-field">
                <label className="filter-label">{t("to")}</label>
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
                  placeholderText={t("toPlaceholder")}
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
                title={t("presetAllTitle")}
              >
                {t("presetAll")}
              </button>
            </div>
          </div>
          <div className="time-tz-hint">
            {t("tzHint", { tz: browserTZ })}
          </div>
          <div className="ds-selector">
            <label className="filter-label">{t("datasource")}</label>
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
              <span className="ds-empty-hint">{t("noDatasourceHint")}</span>
            )}
          </div>
          <div className="viewer-query-row">
            <textarea
              value={query}
              onChange={(e) => { setQuery(e.target.value); setActiveQueryId(null); }}
              onKeyDown={handleKeyDown}
              placeholder={t("queryPlaceholder")}
              rows={2}
              className="query-input viewer-query"
            />
            <button
              onClick={handleSearch}
              disabled={loading || !query.trim()}
              className="btn btn-primary btn-search"
            >
              {loading ? "..." : t("search")}
            </button>
          </div>
          <div className="viewer-secondary-actions">
            <button
              onClick={() => { setSaveName(""); setSaveMsg(""); setShowSaveModal(true); }}
              disabled={!query.trim()}
              className="btn btn-small btn-outline"
            >
              {t("saveQuery")}
            </button>
            <input
              value={exportFileName}
              onChange={(e) => setExportFileName(e.target.value)}
              placeholder={t("fileNamePlaceholder")}
              className="input export-filename-input"
            />
            <span className="export-format-locked" title={t("exportFormatLockedTitle")}>
              {t("exportFormatLocked")}
            </span>
            <button
              onClick={handleExport}
              disabled={!query.trim() || !selectedDS || exporting}
              className="btn btn-small btn-export-action"
            >
              {exporting ? "..." : t("export")}
            </button>
          </div>

          {editingQuery && (
            <div className="save-modal-overlay" onClick={closeEditQuery}>
              <div className="save-modal" onClick={(e) => e.stopPropagation()}>
                <h3>{t("editQueryTitle")}</h3>
                <div className="form-group">
                  <label className="label">{t("name")}</label>
                  <input
                    value={editQueryName}
                    onChange={(e) => setEditQueryName(e.target.value)}
                    placeholder={t("queryNamePlaceholder")}
                    className="input"
                    autoFocus
                  />
                </div>
                <div className="form-group">
                  <label className="label">{t("query")}</label>
                  <textarea
                    value={editQueryText}
                    onChange={(e) => setEditQueryText(e.target.value)}
                    rows={3}
                    className="query-input"
                  />
                </div>
                {datasources.length > 0 && (
                  <div className="form-group">
                    <label className="label">{t("datasource")}</label>
                    <select
                      value={editQueryDS}
                      onChange={(e) => setEditQueryDS(e.target.value)}
                      className="input ds-select"
                    >
                      <option value="">{t("dsNotSelected")}</option>
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
                    {t("globalHint")}
                  </label>
                )}
                <div className="save-modal-actions">
                  <button onClick={closeEditQuery} disabled={editQuerySaving} className="btn btn-small btn-outline">
                    {t("cancel")}
                  </button>
                  <button
                    onClick={handleUpdateQuery}
                    disabled={editQuerySaving || !editQueryName.trim() || !editQueryText.trim()}
                    className="btn btn-small btn-primary"
                  >
                    {editQuerySaving ? "..." : t("save")}
                  </button>
                </div>
                {editQueryMsg && <div className="save-modal-msg">{editQueryMsg}</div>}
              </div>
            </div>
          )}

          {showSaveModal && (
            <div className="save-modal-overlay" onClick={() => setShowSaveModal(false)}>
              <div className="save-modal" onClick={(e) => e.stopPropagation()}>
                <h3>{t("saveQueryTitle")}</h3>
                <code className="save-modal-query">{query}</code>
                <input
                  value={saveName}
                  onChange={(e) => setSaveName(e.target.value)}
                  placeholder={t("queryNamePlaceholder")}
                  className="input"
                  autoFocus
                  onKeyDown={(e) => { if (e.key === "Enter") handleSaveQuery(); }}
                />
                {isAdmin && (
                  <label className="global-check">
                    <input type="checkbox" checked={saveGlobal} onChange={(e) => setSaveGlobal(e.target.checked)} />
                    {t("globalHint")}
                  </label>
                )}
                <div className="save-modal-actions">
                  <button onClick={() => setShowSaveModal(false)} disabled={savingQuery} className="btn btn-small btn-outline">
                    {t("cancel")}
                  </button>
                  <button onClick={handleSaveQuery} disabled={savingQuery || !saveName.trim()} className="btn btn-small btn-primary">
                    {savingQuery ? "..." : t("save")}
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
              <span>{t("logsFound", { n: total })}{hasMore ? "+" : ""}</span>
              <span className="viewer-page-info">
                {t("page", { n: page + 1 })} ({page * PAGE_SIZE + 1}-{page * PAGE_SIZE + logs.length})
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
                              title={sortOrder === "asc" ? t("sortDescTitle") : t("sortAscTitle")}
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
                      <Fragment key={i}>
                        <tr
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
                          <tr className="log-detail-row">
                            <td colSpan={columns.length}>
                              <pre className="log-detail">{JSON.stringify(log, null, 2)}</pre>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="empty">{t("noLogs")}</p>
            )}

            <div className="pagination">
              <button
                onClick={() => fetchLogs(page - 1)}
                disabled={page === 0 || loading}
                className="btn btn-small btn-outline"
              >
                {t("prev")}
              </button>
              <span className="page-num">{t("page", { n: page + 1 })}</span>
              <button
                onClick={() => fetchLogs(page + 1)}
                disabled={!hasMore || loading}
                className="btn btn-small btn-outline"
              >
                {t("next")}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
