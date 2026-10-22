import { useMemo, useState } from "react";
import type { HitsResult } from "../api";
import { useT } from "../i18n";

interface Props {
  hits: HitsResult;
  onSelect?: (start: Date, end: Date) => void;
}

// stepSeconds — "600s" / "1h" / "5m" ko'rinishidagi qadamni soniyaga.
function stepSeconds(step: string): number {
  const m = /^(\d+)([smhd])$/.exec(step.trim());
  if (!m) return 3600;
  const n = Number(m[1]);
  return n * ({ s: 1, m: 60, h: 3600, d: 86400 } as Record<string, number>)[m[2]];
}

const fmtTick = (d: Date, spanSec: number): string => {
  const p = (n: number) => String(n).padStart(2, "0");
  if (spanSec <= 2 * 86400) return `${p(d.getHours())}:${p(d.getMinutes())}`;
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
};

// Histogram — inline SVG ustunli diagramma (kutubxonasiz), ikkala temada
// CSS o'zgaruvchilar orqali ranglanadi. Ustun bosilsa onSelect(bucket oralig'i).
export default function Histogram({ hits, onSelect }: Props) {
  const { t } = useT();
  const [hover, setHover] = useState<number | null>(null);
  const n = hits.values.length;
  const max = useMemo(() => Math.max(1, ...hits.values), [hits.values]);
  const step = stepSeconds(hits.step);
  if (n === 0) return null;

  const W = 800;
  const H = 90;
  const padB = 18;
  const barW = W / n;
  const first = new Date(hits.timestamps[0]);
  const last = new Date(hits.timestamps[n - 1]);
  const spanSec = (last.getTime() - first.getTime()) / 1000 + step;
  const tickEvery = Math.max(1, Math.ceil(n / 8));

  return (
    <div className="hist">
      <div className="hist-head">
        <span className="hist-title">{t("histogramTitle")}</span>
        <span className="hist-total">
          {hover !== null
            ? `${fmtTick(new Date(hits.timestamps[hover]), spanSec)} — ${hits.values[hover].toLocaleString()}`
            : `${hits.total.toLocaleString()} · ${hits.step}`}
        </span>
        {onSelect && <span className="hist-hint">{t("histogramHint")}</span>}
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="hist-svg" role="img">
        {hits.values.map((v, i) => {
          const h = v > 0 ? Math.max(2, ((H - padB) * v) / max) : 0;
          return (
            <rect
              key={i}
              x={i * barW + 0.5}
              y={H - padB - h}
              width={Math.max(1, barW - 1)}
              height={h}
              className={`hist-bar ${hover === i ? "active" : ""}`}
              onMouseEnter={() => setHover(i)}
              onMouseLeave={() => setHover(null)}
              onClick={() => {
                if (!onSelect) return;
                const s = new Date(hits.timestamps[i]);
                onSelect(s, new Date(s.getTime() + step * 1000));
              }}
            >
              <title>{`${hits.timestamps[i]} — ${v}`}</title>
            </rect>
          );
        })}
        {hits.timestamps.map((ts, i) =>
          i % tickEvery === 0 ? (
            <text key={ts} x={i * barW + 2} y={H - 4} className="hist-tick">
              {fmtTick(new Date(ts), spanSec)}
            </text>
          ) : null,
        )}
      </svg>
    </div>
  );
}
