import { useEffect, useMemo, useRef, useState } from "react";
import { Scenario } from "../api";
import { compactMoney, money } from "../format";

// Saldo descontado acumulado mês a mês: a curva sai do investimento inicial
// (negativo) e o payback é o ponto em que ela cruza o zero de vez.

const H = 280;
const PAD = { top: 28, right: 20, bottom: 34, left: 76 };

function niceStep(range: number, target: number): number {
  const raw = range / target;
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const norm = raw / mag;
  return (norm >= 5 ? 10 : norm >= 2 ? 5 : norm >= 1 ? 2 : 1) * mag;
}

export function PaybackChart({ scenario }: { scenario: Scenario }) {
  const wrap = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(760);
  const [hover, setHover] = useState<number | null>(null);

  useEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver((entries) => setWidth(Math.max(320, entries[0].contentRect.width)));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const points = useMemo(
    () => [scenario.initialFlow, ...scenario.months.map((m) => m.cumulativeDiscounted)],
    [scenario],
  );
  const n = points.length - 1;
  const lo = Math.min(0, ...points);
  const hi = Math.max(0, ...points);
  const span = hi - lo || 1;
  const step = niceStep(span, 4);
  const yMin = Math.floor(lo / step) * step;
  const yMax = Math.ceil(hi / step) * step;

  const innerW = width - PAD.left - PAD.right;
  const innerH = H - PAD.top - PAD.bottom;
  const x = (month: number) => PAD.left + (n === 0 ? 0 : (month / n) * innerW);
  const y = (v: number) => PAD.top + (1 - (v - yMin) / (yMax - yMin || 1)) * innerH;
  const zero = y(0);

  const line = points.map((v, i) => `${i === 0 ? "M" : "L"}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(" ");
  const area = `${line} L${x(n).toFixed(1)},${zero.toFixed(1)} L${x(0).toFixed(1)},${zero.toFixed(1)} Z`;

  const yTicks: number[] = [];
  for (let v = yMin; v <= yMax + step / 2; v += step) yTicks.push(Math.abs(v) < step / 1e6 ? 0 : v);
  const xStep = n <= 12 ? 3 : n <= 24 ? 6 : 12;
  const xTicks: number[] = [];
  for (let m = 0; m <= n; m += xStep) xTicks.push(m);
  if (xTicks[xTicks.length - 1] !== n) xTicks.push(n);

  const payback = scenario.paybackMonth;
  const limit = scenario.maxPaybackMonth;
  const late = payback !== null && payback > limit;
  // Os rótulos do payback e do limite não podem se sobrepor: quando os dois
  // marcadores estão próximos, cada um escreve para o seu lado.
  const close = payback !== null && Math.abs(x(payback) - x(limit)) < 150;
  const paybackLeft = payback !== null && (close ? payback <= limit : x(payback) > width - 170);
  const limitLeft = close ? !paybackLeft : x(limit) > width - 190;

  function onMove(e: React.MouseEvent<SVGSVGElement>) {
    const rect = e.currentTarget.getBoundingClientRect();
    const px = e.clientX - rect.left;
    const month = Math.round(((px - PAD.left) / innerW) * n);
    setHover(Math.min(n, Math.max(0, month)));
  }

  const tipLeft = hover !== null && x(hover) > width - 190;
  return (
    <div className="chart" ref={wrap}>
      <svg
        width={width}
        height={H}
        role="img"
        aria-label={
          payback === null
            ? `Saldo descontado acumulado: não recupera o investimento em ${n} meses.`
            : `Saldo descontado acumulado: recupera o investimento no mês ${payback} de ${n}.`
        }
        onMouseMove={onMove}
        onMouseLeave={() => setHover(null)}
      >
        <defs>
          <clipPath id="above-zero">
            <rect x={PAD.left} y={PAD.top} width={innerW} height={Math.max(0, zero - PAD.top)} />
          </clipPath>
          <clipPath id="below-zero">
            <rect x={PAD.left} y={zero} width={innerW} height={Math.max(0, PAD.top + innerH - zero)} />
          </clipPath>
        </defs>

        {yTicks.map((v) => (
          <g key={v}>
            <line x1={PAD.left} x2={width - PAD.right} y1={y(v)} y2={y(v)} className={v === 0 ? "chart-zero" : "chart-grid"} />
            <text x={PAD.left - 10} y={y(v)} className="chart-tick" textAnchor="end" dominantBaseline="middle">
              {compactMoney(v)}
            </text>
          </g>
        ))}
        {xTicks.map((m) => (
          <text key={m} x={x(m)} y={H - 10} className="chart-tick" textAnchor="middle">
            {m === 0 ? "início" : `mês ${m}`}
          </text>
        ))}

        <path d={area} className="chart-area-neg" clipPath="url(#below-zero)" />
        <path d={area} className="chart-area-pos" clipPath="url(#above-zero)" />

        {limit > 0 && limit <= n && (
          <g>
            <line x1={x(limit)} x2={x(limit)} y1={PAD.top - 6} y2={PAD.top + innerH} className="chart-limit" />
            <text x={x(limit) + (limitLeft ? -6 : 6)} y={PAD.top - 12} className="chart-note" textAnchor={limitLeft ? "end" : "start"}>
              Limite da política: mês {limit}
            </text>
          </g>
        )}

        <path d={line} className="chart-line" fill="none" />

        {payback !== null && payback <= n && (
          <g>
            <circle cx={x(payback)} cy={y(points[payback])} r={7} className={late ? "chart-mark chart-mark-late" : "chart-mark"} />
            <text
              x={x(payback) + (paybackLeft ? -14 : 14)}
              // A curva sobe da esquerda para a direita: à esquerda do ponto o
              // rótulo fica acima dela, à direita fica abaixo.
              y={paybackLeft ? Math.max(y(points[payback]) - 14, PAD.top + 12) : Math.min(y(points[payback]) + 22, PAD.top + innerH - 6)}
              className="chart-label"
              textAnchor={paybackLeft ? "end" : "start"}
            >
              Payback: mês {payback}
            </text>
          </g>
        )}

        {hover !== null && (
          <g pointerEvents="none">
            <line x1={x(hover)} x2={x(hover)} y1={PAD.top} y2={PAD.top + innerH} className="chart-cross" />
            <circle cx={x(hover)} cy={y(points[hover])} r={4.5} className="chart-hover-dot" />
          </g>
        )}
      </svg>
      {hover !== null && (
        <div
          className="chart-tip"
          style={{ left: x(hover) + (tipLeft ? -12 : 12), top: PAD.top, transform: tipLeft ? "translateX(-100%)" : undefined }}
        >
          <span className="chart-tip-title">{hover === 0 ? "Início do contrato" : `Mês ${hover}`}</span>
          <span>Saldo acumulado: {money(points[hover])}</span>
          {hover > 0 && <span>Fluxo do mês: {money(scenario.months[hover - 1].netCashFlow)}</span>}
        </div>
      )}
    </div>
  );
}
