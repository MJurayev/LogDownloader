import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import DatePicker from "react-datepicker";
import "react-datepicker/dist/react-datepicker.css";
import {
  api,
  type SavedQuery,
  type Datasource,
  type ExportFormat,
  type ExportContent,
  type HitsResult,
  type HistoryEntry,
  type Profile,
  type ValueHits,
} from "../api";
import { useT } from "../i18n";
import Histogram from "../components/Histogram";
import FieldsPanel from "../components/FieldsPanel";
import "./Pages.css";

const PAGE_SIZE = 50;
const TAIL_MAX_ROWS = 1000;
const DEFAULT_EXPORT_FORMAT: ExportFormat = "tar.gz";
const ARCHIVE_SPLIT_OPTIONS = [0, 500, 1024, 2048, 4096];

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

const PRIORITY_COLS = ["_time", "_msg", "level", "service", "host"];

// addFilterToQuery — filtr qismiga (birinchi `|` dan oldin) `AND expr` qo'shadi.
function addFilterToQuery(query: string, expr: string): string {
  const pipeIdx = query.indexOf("|");
  const filter = (pipeIdx >= 0 ? query.slice(0, pipeIdx) : query).trim();
  const rest = pipeIdx >= 0 ? " " + query.slice(pipeIdx).trim() : "";
  if (!filter || filter === "*") return expr + rest;
  if (filter.includes(expr)) return query;
  return `${filter} AND ${expr}${rest}`;
}

// Ustun sozlamalari localStorage'da (datasource bo'yicha).
interface ColPrefs {
  hidden: string[];
  order: string[];
}
const colPrefsKey = (ds: string) => `lv_cols_${ds}`;
function loadColPrefs(ds: string): ColPrefs {
  try {
    const raw = localStorage.getItem(colPrefsKey(ds));
    if (raw) return JSON.parse(raw);
  } catch {
    /* ignore */
  }
  return { hidden: [], order: [] };
}

export default function LogViewerPage() {
  const { t } = useT();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [query, setQuery] = useState(searchParams.get("q") || "*");
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [selectedDS, setSelectedDS] = useState(searchParams.get("ds") || "");
  const [profile, setProfile] = useState<Profile | null>(null);

  const [logs, setLogs] = useState<Record<string, string>[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [total, setTotal] = useState(0);
  const [searched, setSearched] = useState(false);
  const [expandedRow, setExpandedRow] = useState<number | null>(null);
  const [hits, setHits] = useState<HitsResult | null>(null);
  const [copiedRow, setCopiedRow] = useState<number | null>(null);

  // Save / edit query modals
  const [showSaveModal, setShowSaveModal] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [saveMsg, setSaveMsg] = useState("");
  const [saveGlobal, setSaveGlobal] = useState(false);
  const [savingQuery, setSavingQuery] = useState(false);
  const isAdmin = JSON.parse(localStorage.getItem("user") || "{}").role === "admin";
  const [savedQueries, setSavedQueries] = useState<SavedQuery[]>([]);
  const [activeQueryId, setActiveQueryId] = useState<string | null>(null);
  const [editingQuery, setEditingQuery] = useState<SavedQuery | null>(null);
  const [editQueryName, setEditQueryName] = useState("");
  const [editQueryText, setEditQueryText] = useState("");
  const [editQueryDS, setEditQueryDS] = useState("");
  const [editQueryGlobal, setEditQueryGlobal] = useState(false);
  const [editQuerySaving, setEditQuerySaving] = useState(false);
  const [editQueryMsg, setEditQueryMsg] = useState("");

  // Side panel: saved | fields | history
  const [sidePanel, setSidePanel] = useState<"saved" | "fields" | "history">("saved");
  const [fieldsRefresh, setFieldsRefresh] = useState(0);
  const [history, setHistory] = useState<HistoryEntry[]>([]);

  // Time range. null = cheklov yo'q (barcha loglar).
  const parseParam = (k: string): Date | null => {
    const v = searchParams.get(k);
    if (!v) return null;
    const d = new Date(v);
    return isNaN(d.getTime()) ? null : d;
  };
  const [timeStart, setTimeStart] = useState<Date | null>(parseParam("from"));
  const [timeEnd, setTimeEnd] = useState<Date | null>(parseParam("to"));
  const [sortOrder, setSortOrder] = useState<"asc" | "desc">(searchParams.get("sort") === "desc" ? "desc" : "asc");

  // Live tail
  const [tailing, setTailing] = useState(false);
  const [tailCount, setTailCount] = useState(0);
  const tailAbort = useRef<AbortController | null>(null);

  // Autocomplete
  const [fieldNames, setFieldNames] = useState<string[]>([]);
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [suggestIdx, setSuggestIdx] = useState(0);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  // Columns
  const [colPrefs, setColPrefs] = useState<ColPrefs>({ hidden: [], order: [] });
  const [showCols, setShowCols] = useState(false);

  // Export options
  const [exportFileName, setExportFileName] = useState("");
  const [exportFormat, setExportFormat] = useState<ExportFormat>(DEFAULT_EXPORT_FORMAT);
  const [exportContent, setExportContent] = useState<ExportContent>("ndjson");
  const [exportFields, setExportFields] = useState("");
  const [archiveSplit, setArchiveSplit] = useState(0);
  const [notifyOnDone, setNotifyOnDone] = useState(false);
  const [autoShare, setAutoShare] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [exportMsg, setExportMsg] = useState("");

  const loadSavedQueries = async () => {
    const data = await api.getQueries();
    setSavedQueries(data || []);
  };

  useEffect(() => {
    loadSavedQueries();
    api.me().then(setProfile).catch(() => setProfile(null));
    api.getDatasources().then((ds) => {
      setDatasources(ds || []);
      if (ds && ds.length > 0) {
        setSelectedDS((cur) => (cur && ds.some((d) => d.id === cur) ? cur : ds[0].id));
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (selectedDS) setColPrefs(loadColPrefs(selectedDS));
  }, [selectedDS]);

  useEffect(() => {
    if (sidePanel === "history") api.getHistory().then(setHistory).catch(() => setHistory([]));
  }, [sidePanel]);

  // Maydon nomlari (autocomplete) — datasource tanlanganda fon rejimida.
  useEffect(() => {
    if (!selectedDS) return;
    api
      .getFields(selectedDS, "*")
      .then((f: ValueHits[]) => setFieldNames(f.map((x) => x.value)))
      .catch(() => setFieldNames([]));
  }, [selectedDS, fieldsRefresh]);

  // URL'dan kelgan query bo'lsa avtomatik qidirish (datasource yuklangach).
  const autoRan = useRef(false);
  useEffect(() => {
    if (autoRan.current || !selectedDS || !searchParams.get("q")) return;
    autoRan.current = true;
    fetchLogs(0);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedDS]);

  useEffect(() => () => tailAbort.current?.abort(), []);

  const toISO = (d: Date | null): string | undefined => d?.toISOString();

  const syncURL = (q: string, ds: string, s: Date | null, e: Date | null, order: string) => {
    const p: Record<string, string> = { q, ds, sort: order };
    if (s) p.from = s.toISOString();
    if (e) p.to = e.toISOString();
    setSearchParams(p, { replace: true });
  };

  const fetchLogs = useCallback(
    async (pageNum: number, order: "asc" | "desc" = sortOrder, s: Date | null = timeStart, e: Date | null = timeEnd, q: string = query) => {
      if (tailing) stopTail();
      setLoading(true);
      setError("");
      try {
        const hitsPromise = pageNum === 0 ? api.getHits(selectedDS, q, toISO(s), toISO(e)).catch(() => null) : Promise.resolve(hits);
        const res = await api.queryLogs(q, selectedDS, PAGE_SIZE, pageNum * PAGE_SIZE, toISO(s), toISO(e), order);
        setLogs(res.logs || []);
        setHasMore(res.has_more);
        setTotal(res.total);
        setPage(pageNum);
        setSearched(true);
        setExpandedRow(null);
        setHits(await hitsPromise);
        if (pageNum === 0) {
          setFieldsRefresh((n) => n + 1);
          syncURL(q, selectedDS, s, e, order);
        }
      } catch (err) {
        setError(typeof err === "string" ? err : t("queryError"));
        setLogs([]);
        setHits(null);
      } finally {
        setLoading(false);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [selectedDS, query, timeStart, timeEnd, sortOrder, tailing, hits],
  );

  const handleSearch = () => fetchLogs(0);

  const toggleSortOrder = () => {
    const next: "asc" | "desc" = sortOrder === "asc" ? "desc" : "asc";
    setSortOrder(next);
    if (searched) fetchLogs(0, next);
  };

  const applyPreset = (minutes: number) => {
    const now = new Date();
    setTimeStart(new Date(now.getTime() - minutes * 60 * 1000));
    setTimeEnd(now);
  };

  const clearTimeRange = () => {
    setTimeStart(null);
    setTimeEnd(null);
  };

  const zoomTo = (s: Date, e: Date) => {
    setTimeStart(s);
    setTimeEnd(e);
    fetchLogs(0, sortOrder, s, e);
  };

  // Live tail
  const stopTail = () => {
    tailAbort.current?.abort();
    tailAbort.current = null;
    setTailing(false);
  };

  const startTail = async () => {
    if (!selectedDS || !query.trim()) return;
    stopTail();
    const ctrl = new AbortController();
    tailAbort.current = ctrl;
    setTailing(true);
    setTailCount(0);
    setLogs([]);
    setHits(null);
    setSearched(true);
    setError("");
    setExpandedRow(null);
    try {
      await api.tail(
        selectedDS,
        query,
        (row) => {
          setLogs((prev) => [row, ...prev].slice(0, TAIL_MAX_ROWS));
          setTailCount((n) => n + 1);
        },
        ctrl.signal,
      );
    } catch (err) {
      if (!ctrl.signal.aborted) setError(t("liveTailError", { err: err instanceof Error ? err.message : String(err) }));
    } finally {
      if (tailAbort.current === ctrl) {
        tailAbort.current = null;
        setTailing(false);
      }
    }
  };

  // Fields panel → query
  const addFilter = (expr: string) => {
    const next = addFilterToQuery(query, expr);
    setQuery(next);
    setActiveQueryId(null);
    fetchLogs(0, sortOrder, timeStart, timeEnd, next);
  };

  // Autocomplete: kursor oldidagi so'z bo'yicha maydon nomlari.
  const updateSuggestions = (value: string, cursor: number) => {
    const before = value.slice(0, cursor);
    const m = /([A-Za-z_][A-Za-z0-9_.-]*)$/.exec(before);
    if (!m || fieldNames.length === 0) {
      setSuggestions([]);
      return;
    }
    const tok = m[1].toLowerCase();
    const list = fieldNames.filter((f) => f.toLowerCase().startsWith(tok) && f.toLowerCase() !== tok).slice(0, 8);
    setSuggestions(list);
    setSuggestIdx(0);
  };

  const applySuggestion = (name: string) => {
    const ta = textareaRef.current;
    const cursor = ta ? ta.selectionStart : query.length;
    const before = query.slice(0, cursor).replace(/([A-Za-z_][A-Za-z0-9_.-]*)$/, "");
    const after = query.slice(cursor);
    const next = `${before}${name}:${after}`;
    setQuery(next);
    setSuggestions([]);
    requestAnimationFrame(() => {
      if (ta) {
        const pos = before.length + name.length + 1;
        ta.focus();
        ta.setSelectionRange(pos, pos);
      }
    });
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (suggestions.length > 0) {
      if (e.key === "Tab" || (e.key === "Enter" && !e.shiftKey && suggestions.length === 1)) {
        e.preventDefault();
        applySuggestion(suggestions[suggestIdx] || suggestions[0]);
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setSuggestIdx((i) => (i + 1) % suggestions.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setSuggestIdx((i) => (i - 1 + suggestions.length) % suggestions.length);
        return;
      }
      if (e.key === "Escape") {
        setSuggestions([]);
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSearch();
    }
  };

  // Columns
  const allColumns = useMemo((): string[] => {
    if (logs.length === 0) return [];
    const allKeys = new Set<string>();
    logs.forEach((log) => Object.keys(log).forEach((k) => allKeys.add(k)));
    const sorted = PRIORITY_COLS.filter((k) => allKeys.has(k));
    allKeys.forEach((k) => {
      if (!PRIORITY_COLS.includes(k)) sorted.push(k);
    });
    // foydalanuvchi tartibi (mavjud bo'lganlar oldin, qolgani default tartibda)
    const ordered = colPrefs.order.filter((k) => sorted.includes(k));
    sorted.forEach((k) => {
      if (!ordered.includes(k)) ordered.push(k);
    });
    return ordered;
  }, [logs, colPrefs.order]);
  const columns = allColumns.filter((c) => !colPrefs.hidden.includes(c));

  const saveColPrefs = (p: ColPrefs) => {
    setColPrefs(p);
    try {
      localStorage.setItem(colPrefsKey(selectedDS), JSON.stringify(p));
    } catch {
      /* ignore */
    }
  };
  const toggleCol = (c: string) =>
    saveColPrefs({ ...colPrefs, hidden: colPrefs.hidden.includes(c) ? colPrefs.hidden.filter((x) => x !== c) : [...colPrefs.hidden, c] });
  const moveCol = (c: string, dir: -1 | 1) => {
    const order = [...allColumns];
    const i = order.indexOf(c);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= order.length) return;
    [order[i], order[j]] = [order[j], order[i]];
    saveColPrefs({ ...colPrefs, order });
  };

  const copyRow = async (i: number, log: Record<string, string>) => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(log, null, 2));
      setCopiedRow(i);
      setTimeout(() => setCopiedRow(null), 1200);
    } catch {
      /* ignore */
    }
  };

  // Saved queries
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
      await api.updateQuery(editingQuery.id, editQueryName, editQueryText, editQueryDS || undefined, editQueryGlobal);
      await loadSavedQueries();
      setEditQueryMsg(t("saved"));
      setTimeout(() => {
        closeEditQuery();
        setEditQuerySaving(false);
      }, 800);
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
      setTimeout(() => {
        setShowSaveModal(false);
        setSaveMsg("");
        setSaveName("");
        setSaveGlobal(false);
        setSavingQuery(false);
      }, 600);
    } catch {
      setSaveMsg(t("genericError"));
      setSavingQuery(false);
    }
  };

  // Export
  const canNotify = !!(profile && (profile.email || profile.telegram_chat_id));
  const handleExport = async () => {
    if (!query.trim() || !selectedDS || exporting) return;
    setExporting(true);
    setExportMsg("");
    try {
      await api.startExport({
        query,
        datasourceId: selectedDS,
        start: toISO(timeStart),
        end: toISO(timeEnd),
        sortOrder,
        name: exportFileName,
        format: exportFormat,
        content: exportContent,
        fields: exportFields.split(",").map((s) => s.trim()).filter(Boolean),
        archiveSplitMB: exportFormat === "tar.gz" ? archiveSplit : 0,
        notify: notifyOnDone && canNotify,
        autoShare,
      });
      setExportFileName("");
      navigate("/jobs");
    } catch (err) {
      setExportMsg(typeof err === "string" ? err : t("genericError"));
    } finally {
      setExporting(false);
    }
  };

  const dsSelect = (value: string, onChange: (v: string) => void, allowEmpty = false) => (
    <select value={value} onChange={(e) => onChange(e.target.value)} className="input ds-select">
      {allowEmpty && <option value="">{t("dsNotSelected")}</option>}
      {datasources.map((ds) => (
        <option key={ds.id} value={ds.id}>
          {ds.name || ds.url}
        </option>
      ))}
    </select>
  );

  return (
    <div className="log-viewer-layout">
      {/* Side panel */}
      <div className="sq-panel">
        <div className="lv-tabs">
          {(["saved", "fields", "history"] as const).map((p) => (
            <button key={p} type="button" className={`lv-tab ${sidePanel === p ? "active" : ""}`} onClick={() => setSidePanel(p)}>
              {p === "saved" ? t("panelSaved") : p === "fields" ? t("panelFields") : t("panelHistory")}
            </button>
          ))}
        </div>

        {sidePanel === "saved" &&
          (savedQueries.length === 0 ? (
            <div className="sq-empty">{t("noQueries")}</div>
          ) : (
            <div className="sq-list">
              {savedQueries.map((sq) => (
                <div key={sq.id} className={`sq-item ${activeQueryId === sq.id ? "active" : ""}`} onClick={() => handleSelectQuery(sq)}>
                  <div className="sq-item-name">
                    {sq.name}
                    {sq.global && <span className="global-badge">G</span>}
                  </div>
                  <div className="sq-item-query">{sq.query}</div>
                  <div className="sq-item-actions">
                    <button className="sq-item-edit" onClick={(e) => { e.stopPropagation(); openEditQuery(sq); }} title={t("edit")}>
                      ✎
                    </button>
                    <button className="sq-item-delete" onClick={(e) => { e.stopPropagation(); handleDeleteQuery(sq.id); }} title={t("delete")}>
                      &times;
                    </button>
                  </div>
                </div>
              ))}
            </div>
          ))}

        {sidePanel === "fields" && (
          <FieldsPanel datasourceId={selectedDS} query={query} start={toISO(timeStart)} end={toISO(timeEnd)} refreshKey={fieldsRefresh} onAddFilter={addFilter} />
        )}

        {sidePanel === "history" &&
          (history.length === 0 ? (
            <div className="sq-empty">{t("historyEmpty")}</div>
          ) : (
            <div className="sq-list">
              {history.map((h) => (
                <div
                  key={h.at + h.query}
                  className="sq-item"
                  onClick={() => {
                    setQuery(h.query);
                    setActiveQueryId(null);
                    if (h.datasource_id && datasources.some((d) => d.id === h.datasource_id)) setSelectedDS(h.datasource_id);
                  }}
                >
                  <div className="sq-item-query">{h.query}</div>
                  <div className="sq-item-time">{new Date(h.at).toLocaleString()}</div>
                </div>
              ))}
              <button
                type="button"
                className="btn btn-small btn-outline sq-clear"
                onClick={async () => {
                  await api.clearHistory();
                  setHistory([]);
                }}
              >
                {t("clearHistory")}
              </button>
            </div>
          ))}
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
              <button type="button" className="btn btn-small btn-outline" onClick={clearTimeRange} disabled={!timeStart && !timeEnd} title={t("presetAllTitle")}>
                {t("presetAll")}
              </button>
            </div>
          </div>
          <div className="time-tz-hint">{t("tzHint", { tz: browserTZ })}</div>
          <div className="ds-selector">
            <label className="filter-label">{t("datasource")}</label>
            {datasources.length > 0 ? dsSelect(selectedDS, setSelectedDS) : <span className="ds-empty-hint">{t("noDatasourceHint")}</span>}
          </div>
          <div className="viewer-query-row">
            <div className="query-wrap">
              <textarea
                ref={textareaRef}
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setActiveQueryId(null);
                  updateSuggestions(e.target.value, e.target.selectionStart);
                }}
                onKeyDown={handleKeyDown}
                onBlur={() => setTimeout(() => setSuggestions([]), 150)}
                placeholder={t("queryPlaceholder")}
                rows={2}
                className="query-input viewer-query"
              />
              {suggestions.length > 0 && (
                <div className="suggest-list">
                  {suggestions.map((s, i) => (
                    <button key={s} type="button" className={`suggest-item ${i === suggestIdx ? "active" : ""}`} onMouseDown={(e) => { e.preventDefault(); applySuggestion(s); }}>
                      {s}
                    </button>
                  ))}
                  <span className="suggest-hint">{t("autocompleteHint")}</span>
                </div>
              )}
            </div>
            <button onClick={handleSearch} disabled={loading || !query.trim() || !selectedDS} className="btn btn-primary btn-search">
              {loading ? "..." : t("search")}
            </button>
            <button
              type="button"
              onClick={tailing ? stopTail : startTail}
              disabled={!query.trim() || !selectedDS}
              className={`btn btn-search ${tailing ? "btn-danger" : "btn-outline"}`}
              title={t("liveTail")}
            >
              {tailing ? t("liveTailStop") : t("liveTail")}
            </button>
          </div>

          <div className="viewer-secondary-actions">
            <button onClick={() => { setSaveName(""); setSaveMsg(""); setShowSaveModal(true); }} disabled={!query.trim()} className="btn btn-small btn-outline">
              {t("saveQuery")}
            </button>
            <input value={exportFileName} onChange={(e) => setExportFileName(e.target.value)} placeholder={t("fileNamePlaceholder")} className="input export-filename-input" />
            <select value={exportFormat} onChange={(e) => setExportFormat(e.target.value as ExportFormat)} className="input export-format-select" title={t("formatSelectTitle")}>
              <option value="tar.gz">.tar.gz</option>
              <option value="log.gz">.log.gz</option>
              <option value="log">{t("formatRaw")}</option>
            </select>
            <span className={`export-locked-chip ${exportFormat === "tar.gz" ? "" : "muted"}`} title={exportFormat === "tar.gz" ? t("chunkAutoTitle") : t("chunkOnlyTarTitle")}>
              {t("chunkAuto")}
            </span>
            <button onClick={handleExport} disabled={!query.trim() || !selectedDS || exporting} className="btn btn-small btn-export-action">
              {exporting ? "..." : t("export")}
            </button>
          </div>

          <details className="export-options">
            <summary>{t("exportOptions")}</summary>
            <div className="export-options-grid">
              <div className="form-group">
                <label className="label">{t("contentLabel")}</label>
                <select value={exportContent} onChange={(e) => setExportContent(e.target.value as ExportContent)} className="input ds-select">
                  <option value="ndjson">{t("contentNdjson")}</option>
                  <option value="msg">{t("contentMsg")}</option>
                  <option value="csv">{t("contentCsv")}</option>
                </select>
              </div>
              <div className="form-group">
                <label className="label">{t("fieldsLabel")}</label>
                <input value={exportFields} onChange={(e) => setExportFields(e.target.value)} placeholder={t("fieldsPlaceholder")} className="input" disabled={exportContent === "msg"} list="lv-field-names" />
                <datalist id="lv-field-names">
                  {fieldNames.map((f) => (
                    <option key={f} value={f} />
                  ))}
                </datalist>
                <div className="field-hint">{t("fieldsHint")}</div>
              </div>
              <div className="form-group">
                <label className="label">{t("archiveSplitLabel")}</label>
                <select value={archiveSplit} onChange={(e) => setArchiveSplit(Number(e.target.value))} className="input ds-select" disabled={exportFormat !== "tar.gz"}>
                  {ARCHIVE_SPLIT_OPTIONS.map((mb) => (
                    <option key={mb} value={mb}>
                      {mb === 0 ? t("archiveSplitNone") : mb >= 1024 ? `${mb / 1024} GB` : `${mb} MB`}
                    </option>
                  ))}
                </select>
              </div>
              <div className="form-group export-checks">
                <label className="global-check" title={canNotify ? "" : t("notifyNoTarget")}>
                  <input type="checkbox" checked={notifyOnDone && canNotify} disabled={!canNotify} onChange={(e) => setNotifyOnDone(e.target.checked)} />
                  {t("notifyLabel")}
                  {!canNotify && <span className="field-hint"> ({t("notifyNoTarget")})</span>}
                </label>
                <label className="global-check">
                  <input type="checkbox" checked={autoShare} onChange={(e) => setAutoShare(e.target.checked)} />
                  {t("autoShareLabel")}
                </label>
              </div>
            </div>
            {exportMsg && <div className="error-text">{exportMsg}</div>}
          </details>

          {editingQuery && (
            <div className="save-modal-overlay" onClick={closeEditQuery}>
              <div className="save-modal" onClick={(e) => e.stopPropagation()}>
                <h3>{t("editQueryTitle")}</h3>
                <div className="form-group">
                  <label className="label">{t("name")}</label>
                  <input value={editQueryName} onChange={(e) => setEditQueryName(e.target.value)} placeholder={t("queryNamePlaceholder")} className="input" autoFocus />
                </div>
                <div className="form-group">
                  <label className="label">{t("query")}</label>
                  <textarea value={editQueryText} onChange={(e) => setEditQueryText(e.target.value)} rows={3} className="query-input" />
                </div>
                {datasources.length > 0 && (
                  <div className="form-group">
                    <label className="label">{t("datasource")}</label>
                    {dsSelect(editQueryDS, setEditQueryDS, true)}
                  </div>
                )}
                {isAdmin && (
                  <label className="global-check">
                    <input type="checkbox" checked={editQueryGlobal} onChange={(e) => setEditQueryGlobal(e.target.checked)} />
                    {t("globalHint")}
                  </label>
                )}
                <div className="save-modal-actions">
                  <button onClick={closeEditQuery} disabled={editQuerySaving} className="btn btn-small btn-outline">{t("cancel")}</button>
                  <button onClick={handleUpdateQuery} disabled={editQuerySaving || !editQueryName.trim() || !editQueryText.trim()} className="btn btn-small btn-primary">
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
                <input value={saveName} onChange={(e) => setSaveName(e.target.value)} placeholder={t("queryNamePlaceholder")} className="input" autoFocus onKeyDown={(e) => { if (e.key === "Enter") handleSaveQuery(); }} />
                {isAdmin && (
                  <label className="global-check">
                    <input type="checkbox" checked={saveGlobal} onChange={(e) => setSaveGlobal(e.target.checked)} />
                    {t("globalHint")}
                  </label>
                )}
                <div className="save-modal-actions">
                  <button onClick={() => setShowSaveModal(false)} disabled={savingQuery} className="btn btn-small btn-outline">{t("cancel")}</button>
                  <button onClick={handleSaveQuery} disabled={savingQuery || !saveName.trim()} className="btn btn-small btn-primary">{savingQuery ? "..." : t("save")}</button>
                </div>
                {saveMsg && <div className="save-modal-msg">{saveMsg}</div>}
              </div>
            </div>
          )}
        </div>

        {error && <div className="error-banner">{error}</div>}

        {searched && !error && (
          <>
            {hits && !tailing && hits.values.length > 0 && <Histogram hits={hits} onSelect={zoomTo} />}

            <div className="viewer-info">
              <span>
                {tailing ? t("liveTailOn", { n: tailCount }) : `${t("logsFound", { n: total })}${hasMore ? "+" : ""}`}
              </span>
              <span className="viewer-info-right">
                {!tailing && (
                  <span className="viewer-page-info">
                    {t("page", { n: page + 1 })} ({page * PAGE_SIZE + 1}-{page * PAGE_SIZE + logs.length})
                  </span>
                )}
                <span className="cols-wrap">
                  <button type="button" className="btn btn-small btn-outline" onClick={() => setShowCols((v) => !v)} disabled={allColumns.length === 0}>
                    {t("columnsBtn")}
                  </button>
                  {showCols && (
                    <div className="cols-popover" onMouseLeave={() => setShowCols(false)}>
                      <div className="cols-title">{t("columnsTitle")}</div>
                      {allColumns.map((c) => (
                        <div key={c} className="cols-row">
                          <label className="global-check">
                            <input type="checkbox" checked={!colPrefs.hidden.includes(c)} onChange={() => toggleCol(c)} />
                            {c}
                          </label>
                          <span className="cols-move">
                            <button type="button" onClick={() => moveCol(c, -1)} title="←">◀</button>
                            <button type="button" onClick={() => moveCol(c, 1)} title="→">▶</button>
                          </span>
                        </div>
                      ))}
                      <button type="button" className="btn btn-small btn-outline" onClick={() => saveColPrefs({ hidden: [], order: [] })}>
                        {t("columnsReset")}
                      </button>
                    </div>
                  )}
                </span>
              </span>
            </div>

            {logs.length > 0 ? (
              <div className="log-table-wrap">
                <table className="log-table">
                  <thead>
                    <tr>
                      {columns.map((col) => (
                        <th key={col}>
                          {col === "_time" && !tailing ? (
                            <button type="button" className="th-sort-btn" onClick={toggleSortOrder} title={sortOrder === "asc" ? t("sortDescTitle") : t("sortAscTitle")}>
                              {col} <span className="th-sort-arrow">{sortOrder === "asc" ? "▲" : "▼"}</span>
                            </button>
                          ) : (
                            col
                          )}
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
                              <div className="log-detail-actions">
                                <button type="button" className="btn btn-small btn-outline" onClick={(e) => { e.stopPropagation(); copyRow(i, log); }}>
                                  {copiedRow === i ? t("copied") : t("copyRow")}
                                </button>
                              </div>
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
              <p className="empty">{tailing ? t("liveTailOn", { n: 0 }) : t("noLogs")}</p>
            )}

            {!tailing && (
              <div className="pagination">
                <button onClick={() => fetchLogs(page - 1)} disabled={page === 0 || loading} className="btn btn-small btn-outline">{t("prev")}</button>
                <span className="page-num">{t("page", { n: page + 1 })}</span>
                <button onClick={() => fetchLogs(page + 1)} disabled={!hasMore || loading} className="btn btn-small btn-outline">{t("next")}</button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
