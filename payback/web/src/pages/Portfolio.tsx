import { useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, ProjectRow, ProjectStatus } from "../api";
import { compactMoney, money, pctMonth } from "../format";
import { ErrorNote, Loading, useLoad, ViabilityTag } from "../ui";

const TABS: { status: ProjectStatus; label: string; noun: [string, string] }[] = [
  { status: "active", label: "Ativos", noun: ["contrato ativo", "contratos ativos"] },
  { status: "draft", label: "Rascunhos", noun: ["rascunho", "rascunhos"] },
  { status: "completed", label: "Concluídos", noun: ["contrato concluído", "contratos concluídos"] },
  { status: "cancelled", label: "Cancelados", noun: ["projeto cancelado", "projetos cancelados"] },
];

type SortKey = "client" | "npv" | "irr" | "payback" | "monthly";

function sortValue(r: ProjectRow, key: SortKey): number | string {
  switch (key) {
    case "client":
      return `${r.project.clientName} ${r.project.name}`.toLowerCase();
    case "npv":
      return r.summary.status === "ready" ? r.summary.npv : -Infinity;
    case "irr":
      return r.summary.monthlyIrr ?? -Infinity;
    case "payback":
      return r.summary.paybackMonth ?? Infinity;
    case "monthly":
      return r.project.monthlyRevenue;
  }
}

export function Portfolio() {
  const { data, error, loading, reload } = useLoad(api.projects, []);
  // A aba fica no endereço, para o botão "voltar" e os links preservarem o filtro.
  const [params, setParams] = useSearchParams();
  const tab = TABS.find((t) => t.status === params.get("status"))?.status ?? null;
  const setTab = (status: ProjectStatus) => setParams({ status }, { replace: true });
  const [showTests, setShowTests] = useState(false);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "client", desc: false });

  const real = useMemo(() => (data ?? []).filter((r) => showTests || !r.project.isTest), [data, showTests]);
  const counts = useMemo(() => {
    const c: Record<string, number> = {};
    for (const r of real) c[r.project.status] = (c[r.project.status] ?? 0) + 1;
    return c;
  }, [real]);
  // Abre na primeira aba que tem projetos: ativos, senão rascunhos.
  const current = tab ?? TABS.find((t) => counts[t.status])?.status ?? "active";
  const tabInfo = TABS.find((t) => t.status === current)!;

  const rows = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = real.filter(
      (r) =>
        r.project.status === current &&
        (q === "" || `${r.project.clientName} ${r.project.name}`.toLowerCase().includes(q)),
    );
    return list.sort((a, b) => {
      const va = sortValue(a, sort.key);
      const vb = sortValue(b, sort.key);
      const cmp = va < vb ? -1 : va > vb ? 1 : 0;
      return sort.desc ? -cmp : cmp;
    });
  }, [real, current, query, sort]);

  const inTab = real.filter((r) => r.project.status === current);
  const ready = inTab.filter((r) => r.summary.status === "ready");
  const totals = ready.reduce(
    (t, r) => ({ capex: t.capex + r.summary.capex, npv: t.npv + r.summary.npv, revenue: t.revenue + r.summary.totalRevenue }),
    { capex: 0, npv: 0, revenue: 0 },
  );
  const mix = {
    viable: ready.filter((r) => r.summary.viability === "VIAVEL").length,
    exception: ready.filter((r) => r.summary.viability === "EXCECAO_COMERCIAL").length,
    notViable: ready.filter((r) => r.summary.viability === "INVIAVEL").length,
    pending: inTab.length - ready.length,
  };
  const belowIrr = ready.filter((r) => r.summary.irrStatus === "ABAIXO_DO_LIMITE").length;
  const hiddenTests = (data ?? []).filter((r) => r.project.isTest).length;

  const header = (key: SortKey, label: string, numeric = true) => (
    <th className={numeric ? "num" : undefined} aria-sort={sort.key === key ? (sort.desc ? "descending" : "ascending") : "none"}>
      <button
        type="button"
        className="th-sort"
        onClick={() => setSort((s) => ({ key, desc: s.key === key ? !s.desc : numeric }))}
      >
        {label}
        {sort.key === key && <span aria-hidden="true">{sort.desc ? " ↓" : " ↑"}</span>}
      </button>
    </th>
  );

  return (
    <div className="page">
      <header className="page-head">
        <h1>Carteira</h1>
        <Link to="/projetos/novo" className="btn btn-primary">
          Novo projeto
        </Link>
      </header>

      <ErrorNote error={error} onRetry={reload} />
      {loading && !data && <Loading what="a carteira" />}

      {data && data.length === 0 && (
        <div className="empty">
          <h2>Nenhum projeto ainda</h2>
          <p>Cadastre a primeira proposta para ver investimento, payback e viabilidade.</p>
          <Link to="/projetos/novo" className="btn btn-primary">
            Criar o primeiro projeto
          </Link>
        </div>
      )}

      {data && data.length > 0 && (
        <>
          <div className="tabs" role="tablist" aria-label="Status dos projetos">
            {TABS.map((t) => (
              <button
                key={t.status}
                type="button"
                role="tab"
                aria-selected={t.status === current}
                className="tab"
                onClick={() => setTab(t.status)}
              >
                {t.label} <span className="tab-count">{counts[t.status] ?? 0}</span>
              </button>
            ))}
          </div>

          {inTab.length > 0 && (
            <section className="summary" aria-label="Resumo">
              <p className="summary-line">
                {inTab.length} {inTab.length === 1 ? tabInfo.noun[0] : tabInfo.noun[1]}
                {ready.length > 0 && (
                  <>
                    {" "}
                    {inTab.length === 1 ? "soma" : "somam"} <strong>{compactMoney(totals.capex)}</strong> investidos,{" "}
                    <strong>{compactMoney(totals.revenue)}</strong> de receita contratada e{" "}
                    <strong>{compactMoney(totals.npv)}</strong> de valor presente líquido.
                  </>
                )}
                {ready.length === 0 && " ainda sem análise completa."}
              </p>
              <div className="mix" role="img" aria-label="Distribuição das decisões">
                {mix.viable > 0 && <span className="mix-seg mix-good" style={{ flexGrow: mix.viable }} />}
                {mix.exception > 0 && <span className="mix-seg mix-warn" style={{ flexGrow: mix.exception }} />}
                {mix.notViable > 0 && <span className="mix-seg mix-bad" style={{ flexGrow: mix.notViable }} />}
                {mix.pending > 0 && <span className="mix-seg mix-muted" style={{ flexGrow: mix.pending }} />}
              </div>
              <ul className="mix-legend">
                <li><span className="dot mix-good" /> {mix.viable} {mix.viable === 1 ? "viável" : "viáveis"}</li>
                <li><span className="dot mix-warn" /> {mix.exception} em exceção comercial</li>
                <li><span className="dot mix-bad" /> {mix.notViable} {mix.notViable === 1 ? "inviável" : "inviáveis"}</li>
                {mix.pending > 0 && <li><span className="dot mix-muted" /> {mix.pending} com análise pendente</li>}
                {belowIrr > 0 && <li>{belowIrr} abaixo da TIR mínima</li>}
              </ul>
            </section>
          )}

          <div className="toolbar">
            <input
              type="search"
              className="input"
              placeholder="Buscar por cliente ou projeto"
              aria-label="Buscar por cliente ou projeto"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />
            {hiddenTests > 0 && (
              <label className="check">
                <input type="checkbox" checked={showTests} onChange={(e) => setShowTests(e.target.checked)} />
                Incluir {hiddenTests} {hiddenTests === 1 ? "projeto de teste" : "projetos de teste"}
              </label>
            )}
          </div>

          {rows.length === 0 ? (
            <p className="empty-line">
              {query ? "Nenhum projeto corresponde à busca." : `Nenhum projeto em "${tabInfo.label}".`}
            </p>
          ) : (
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    {header("client", "Cliente e projeto", false)}
                    <th className="num">Prazo</th>
                    {header("monthly", "Mensalidade")}
                    <th className="num">Investimento</th>
                    {header("npv", "VPL")}
                    {header("irr", "TIR")}
                    {header("payback", "Payback")}
                    <th>Decisão</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map(({ project: p, summary: s, access }) => (
                    <tr key={p.id}>
                      <td>
                        <Link to={`/projetos/${p.id}`} className="row-link">
                          <span className="row-client">{p.clientName}</span>
                          <span className="row-name">{p.name}</span>
                        </Link>
                        {p.isTest && <span className="tag tag-muted">Teste</span>}
                        {access !== "owner" && <span className="row-owner">de {p.ownerName}</span>}
                      </td>
                      <td className="num">{p.durationMonths} meses</td>
                      <td className="num">{money(p.monthlyRevenue)}</td>
                      {s.status === "ready" ? (
                        <>
                          <td className="num">{money(s.capex)}</td>
                          <td className={s.npv < 0 ? "num neg" : "num"}>{money(s.npv)}</td>
                          <td className={s.irrStatus === "ABAIXO_DO_LIMITE" ? "num warn" : "num"}>{pctMonth(s.monthlyIrr)}</td>
                          <td className="num">{s.paybackMonth === null ? "não recupera" : `mês ${s.paybackMonth}`}</td>
                        </>
                      ) : (
                        <td colSpan={4} className="pending">
                          Faltam premissas para calcular
                        </td>
                      )}
                      <td>
                        <ViabilityTag value={s.viability} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}
