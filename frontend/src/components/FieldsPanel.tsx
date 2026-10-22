import { useEffect, useState } from "react";
import { api, type ValueHits } from "../api";
import { useT } from "../i18n";

interface Props {
  datasourceId: string;
  query: string;
  start?: string;
  end?: string;
  refreshKey: number; // Search bosilganda oshadi → qayta yuklash
  onAddFilter: (expr: string) => void;
}

// jsonQuote — LogsQL qiymatini qo'shtirnoq ichida xavfsiz ko'rsatish.
const jsonQuote = (v: string) => JSON.stringify(v);

// FieldsPanel — VictoriaLogs field_names / field_values / streams.
// Maydon bosilsa top qiymatlari yuklanadi; qiymat bosilsa `field:"value"`
// filtri query'ga qo'shiladi; stream bosilsa `_stream:{...}`.
export default function FieldsPanel({ datasourceId, query, start, end, refreshKey, onAddFilter }: Props) {
  const { t } = useT();
  const [fields, setFields] = useState<ValueHits[]>([]);
  const [streams, setStreams] = useState<ValueHits[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [expanded, setExpanded] = useState<string | null>(null);
  const [values, setValues] = useState<Record<string, ValueHits[]>>({});

  useEffect(() => {
    if (!datasourceId) return;
    let cancelled = false;
    setLoading(true);
    setError("");
    Promise.all([
      api.getFields(datasourceId, query || "*", start, end),
      api.getStreams(datasourceId, query || "*", start, end, 30),
    ])
      .then(([f, s]) => {
        if (cancelled) return;
        setFields(f);
        setStreams(s);
        setValues({});
        setExpanded(null);
      })
      .catch((e) => !cancelled && setError(typeof e === "string" ? e : String(e)))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [datasourceId, refreshKey]);

  const toggleField = async (name: string) => {
    if (expanded === name) {
      setExpanded(null);
      return;
    }
    setExpanded(name);
    if (!values[name]) {
      try {
        const v = await api.getFieldValues(datasourceId, query || "*", name, 15, start, end);
        setValues((prev) => ({ ...prev, [name]: v }));
      } catch {
        setValues((prev) => ({ ...prev, [name]: [] }));
      }
    }
  };

  if (!datasourceId) return <div className="sq-empty">{t("noDatasourceHint")}</div>;
  if (loading && fields.length === 0) return <div className="sq-empty">{t("loading")}</div>;
  if (error) return <div className="sq-empty error-text">{error}</div>;
  if (fields.length === 0) return <div className="sq-empty">{t("fieldsEmpty")}</div>;

  return (
    <div className="fields-panel">
      <div className="fields-hint">{t("fieldsHintPanel")}</div>
      <div className="fields-list">
        {fields.map((f) => (
          <div key={f.value} className="field-item">
            <button type="button" className={`field-name ${expanded === f.value ? "active" : ""}`} onClick={() => toggleField(f.value)}>
              <span className="field-chevron">{expanded === f.value ? "▾" : "▸"}</span>
              <span className="field-label">{f.value}</span>
              <span className="field-hits">{f.hits.toLocaleString()}</span>
            </button>
            {expanded === f.value && (
              <div className="field-values">
                {values[f.value] === undefined ? (
                  <div className="field-value muted">{t("loading")}</div>
                ) : values[f.value].length === 0 ? (
                  <div className="field-value muted">—</div>
                ) : (
                  values[f.value].map((v) => (
                    <button
                      key={v.value}
                      type="button"
                      className="field-value"
                      title={`${f.value}:${jsonQuote(v.value)}`}
                      onClick={() => onAddFilter(`${f.value}:${jsonQuote(v.value)}`)}
                    >
                      <span className="field-value-text">{v.value === "" ? "«»" : v.value}</span>
                      <span className="field-hits">{v.hits.toLocaleString()}</span>
                    </button>
                  ))
                )}
              </div>
            )}
          </div>
        ))}
      </div>
      {streams.length > 0 && (
        <>
          <div className="sq-panel-header fields-subhead">{t("streamsTitle")}</div>
          <div className="fields-list">
            {streams.map((s) => (
              <button key={s.value} type="button" className="field-value stream-item" title={s.value} onClick={() => onAddFilter(`_stream:${s.value}`)}>
                <span className="field-value-text">{s.value}</span>
                <span className="field-hits">{s.hits.toLocaleString()}</span>
              </button>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
