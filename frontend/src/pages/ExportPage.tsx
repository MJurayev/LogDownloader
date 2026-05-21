import { useState, useEffect } from "react";
import DatePicker from "react-datepicker";
import "react-datepicker/dist/react-datepicker.css";
import { api, type Datasource } from "../api";
import "./Pages.css";

const browserTZ = (() => {
  try {
    const parts = new Intl.DateTimeFormat("en", { timeZoneName: "shortOffset" }).formatToParts(new Date());
    return parts.find((p) => p.type === "timeZoneName")?.value || "";
  } catch {
    return "";
  }
})();

export default function ExportPage() {
  const [query, setQuery] = useState("");
  const [datasources, setDatasources] = useState<Datasource[]>([]);
  const [selectedDS, setSelectedDS] = useState("");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  const [timeStart, setTimeStart] = useState<Date | null>(null);
  const [timeEnd, setTimeEnd] = useState<Date | null>(null);

  useEffect(() => {
    api.getDatasources().then((ds) => {
      setDatasources(ds || []);
      if (ds && ds.length > 0) setSelectedDS(ds[0].id);
    });
  }, []);

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

  const handleExport = async () => {
    if (!query.trim() || !selectedDS) return;
    setLoading(true);
    setMessage("");
    try {
      const res = await api.startExport(query, selectedDS, toISO(timeStart), toISO(timeEnd), "asc");
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
          Mahalliy vaqt ({browserTZ}) — server'ga UTC'da yuboriladi.
        </div>
        <textarea
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder='Masalan: level:"error"'
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
