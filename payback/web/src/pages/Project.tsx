import { FormEvent, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Analysis, api, ProjectDetail, ProjectStatus, Scenario } from "../api";
import { useUser } from "../App";
import { Conditions, projectInput } from "../components/Conditions";
import { Items } from "../components/Items";
import { PaybackChart } from "../components/PaybackChart";
import { dateTime, healthLabel, money, parseNumber, pct, pctMonth, statusLabel, toField } from "../format";
import { ConfirmButton, ErrorNote, Loading, Row, useAction, useLoad, ViabilityTag } from "../ui";

/** A decisão em uma frase, no lugar de um painel de indicadores. */
function verdict(s: Scenario, months: number): { tone: string; title: string; text: string } {
  const p = s.paybackMonth;
  if (s.viability === "VIAVEL") {
    return {
      tone: "good",
      title: "Viável",
      text: `O investimento volta no mês ${p} de ${months}, dentro do limite de ${s.maxPaybackMonth} meses da política.`,
    };
  }
  if (s.viability === "EXCECAO_COMERCIAL") {
    return {
      tone: "warn",
      title: "Exceção comercial",
      text: `O contrato cria valor, mas o investimento só volta no mês ${p} de ${months}. O limite da política é o mês ${s.maxPaybackMonth}.`,
    };
  }
  return {
    tone: "bad",
    title: "Inviável",
    text:
      p === null
        ? `O contrato não recupera o investimento em ${months} meses à taxa de oportunidade da empresa.`
        : "O contrato não rende mais que a taxa de oportunidade da empresa.",
  };
}

function Ledger({ s, analysis }: { s: Scenario; analysis: Analysis }) {
  const taxRate = s.totalRevenue > 0 ? s.taxes / s.totalRevenue : 0;
  return (
    <section className="block">
      <header className="block-head">
        <h2>Memória de cálculo</h2>
      </header>
      <dl className="ledger">
        <Row label="Receita bruta contratada" value={money(s.totalRevenue)} />
        <Row sub label={`Impostos estimados (${pct(taxRate)})`} value={`− ${money(s.taxes)}`} />
        <Row sub label="Inadimplência esperada" value={`− ${money(s.totalExpectedLoss)}`} />
        <Row sub label="Suporte" value={`− ${money(s.totalSupportCost)}`} />
        <Row sub label="Licenças por pontos" value={`− ${money(s.totalLicensingCost)}`} />
        <Row sub label="Comissões" value={`− ${money(s.commission.total)}`} />
        <Row strong label="Caixa gerado pelo contrato" value={money(s.netCashAfterRisks)} />
        <Row sub label="Investimento inicial (CAPEX)" value={`− ${money(s.capex)}`} />
        <Row strong label="Resultado líquido de caixa" value={money(s.netCashResult)} />
      </dl>
      <dl className="ledger ledger-gap">
        <Row label="Valor presente líquido (VPL)" value={money(s.npv)} />
        <Row label="VPL sem inadimplência e suporte" value={money(analysis.scenarios!.base.npv)} />
        <Row label="Retorno sobre o investimento" value={pct(s.roiOnCapex)} />
        <Row label="Payback sem desconto" value={s.simplePaybackMonth === null ? "não recupera" : `mês ${s.simplePaybackMonth}`} />
        <Row label="Caixa gerado depois do payback" value={money(s.cashAfterPayback)} />
      </dl>
      {analysis.capex && (
        <dl className="ledger ledger-gap">
          <Row label="Hardware" value={money(analysis.capex.hardware)} />
          <Row label="Software e licenças" value={money(analysis.capex.software)} />
          <Row label="DIFAL" value={money(analysis.capex.difal)} />
        </dl>
      )}
      <dl className="ledger ledger-gap">
        <Row label={`Comissão do vendedor (${pct(s.commission.rates.sellerRate)})`} value={money(s.commission.seller)} />
        <Row label={`Comissão do arquiteto (${pct(s.commission.rates.architectRate)})`} value={money(s.commission.architect)} />
        <Row label={`Comissão do gerente (${pct(s.commission.rates.managerRate)})`} value={money(s.commission.manager)} />
        <Row sub label="Base: margem do contrato após impostos" value={money(s.commission.netBase)} />
      </dl>
    </section>
  );
}

function Pricing({ detail, canEdit, onSaved }: { detail: ProjectDetail; canEdit: boolean; onSaved: (d: ProjectDetail) => void }) {
  const { project, analysis } = detail;
  const s = analysis.scenarios!.pessimistic;
  const [text, setText] = useState("");
  const [sim, setSim] = useState<{ monthly: number; result: Analysis } | null>(null);
  const run = useAction();
  const apply = useAction();

  async function simulate(e: FormEvent) {
    e.preventDefault();
    const monthly = parseNumber(text);
    if (monthly === null) return;
    const result = await run.run(() => api.simulate(project.id, monthly));
    if (result) setSim({ monthly, result });
  }
  async function applyMonthly(monthly: number) {
    const saved = await apply.run(() => api.updateProject(project.id, projectInput(project, { monthlyRevenue: monthly })));
    if (saved) {
      onSaved(saved);
      setSim(null);
      setText("");
    }
  }
  const floor = (v: number | null) => (v === null ? "nenhuma mensalidade resolve" : money(v));
  const gap = s.minimumPolicyMonthly === null ? null : project.monthlyRevenue - s.minimumPolicyMonthly;
  const simS = sim?.result.scenarios?.pessimistic;

  return (
    <section className="block">
      <header className="block-head">
        <h2>Mensalidade</h2>
      </header>
      <dl className="ledger">
        <Row label="Mensalidade proposta" value={money(project.monthlyRevenue)} />
        <Row label={`Mínima para o payback da política (mês ${s.maxPaybackMonth})`} value={floor(s.minimumPolicyMonthly)} />
        <Row label="Mínima para não perder valor (VPL zero)" value={floor(s.minimumEconomicMonthly)} />
        {gap !== null && (
          <Row
            strong
            label={gap >= 0 ? "Folga sobre o mínimo da política" : "Falta para o mínimo da política"}
            value={`${money(Math.abs(gap))} por mês`}
          />
        )}
      </dl>

      <form onSubmit={simulate} className="simulator">
        <label className="field">
          <span className="field-label">Simular outra mensalidade (R$)</span>
          <input className="input num" inputMode="decimal" placeholder={toField(project.monthlyRevenue)} value={text} onChange={(e) => setText(e.target.value)} />
        </label>
        <button type="submit" className="btn" disabled={run.busy || parseNumber(text) === null}>
          {run.busy ? "Calculando…" : "Simular"}
        </button>
      </form>
      <ErrorNote error={run.error ?? apply.error} />
      {sim && simS && (
        <div className="sim-result" role="status">
          <p>
            Com {money(sim.monthly)} por mês: <ViabilityTag value={simS.viability} />{" "}
            {simS.paybackMonth === null ? "sem payback" : `payback no mês ${simS.paybackMonth}`}, VPL de {money(simS.npv)}, TIR de{" "}
            {pctMonth(simS.monthlyIrr)}
          </p>
          {canEdit && (
            <button type="button" className="btn btn-primary" disabled={apply.busy} onClick={() => applyMonthly(sim.monthly)}>
              Usar {money(sim.monthly)} no projeto
            </button>
          )}
        </div>
      )}

      {analysis.sensitivity && (
        <div className="table-wrap">
          <table className="table table-compact">
            <caption>Sensibilidade à mensalidade</caption>
            <thead>
              <tr>
                <th>Variação</th>
                <th className="num">Mensalidade</th>
                <th className="num">VPL</th>
                <th className="num">Payback</th>
                <th>Decisão</th>
              </tr>
            </thead>
            <tbody>
              {analysis.sensitivity.map((r) => (
                <tr key={r.delta} className={r.delta === 0 ? "row-current" : undefined}>
                  <td>{r.delta === 0 ? "Proposta" : `${r.delta > 0 ? "+" : "−"}${Math.abs(r.delta * 100)}%`}</td>
                  <td className="num">{money(r.monthlyRevenue)}</td>
                  <td className={r.npv < 0 ? "num neg" : "num"}>{money(r.npv)}</td>
                  <td className="num">{r.paybackMonth === null ? "não recupera" : `mês ${r.paybackMonth}`}</td>
                  <td>
                    <ViabilityTag value={r.viability} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Sharing({ projectId }: { projectId: number }) {
  const shares = useLoad(() => api.shares(projectId), [projectId]);
  const [email, setEmail] = useState("");
  const action = useAction();
  async function add(e: FormEvent) {
    e.preventDefault();
    const list = await action.run(() => api.addShare(projectId, email));
    if (list) {
      shares.setData(list);
      setEmail("");
    }
  }
  async function remove(userId: number) {
    await action.run(() => api.removeShare(projectId, userId));
    shares.reload();
  }
  return (
    <section className="block">
      <header className="block-head">
        <h2>Compartilhamento</h2>
      </header>
      <p className="block-text">Quem recebe o projeto pode ver tudo, mas não alterar.</p>
      <ErrorNote error={shares.error} onRetry={shares.reload} />
      {shares.data && shares.data.length > 0 && (
        <ul className="plain-list">
          {shares.data.map((s) => (
            <li key={s.userId}>
              <span>
                {s.name || s.email} <span className="muted">{s.name ? s.email : ""}</span>
              </span>
              <button type="button" className="btn-link" onClick={() => remove(s.userId)}>
                Remover
              </button>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={add} className="inline-form">
        <input
          className="input"
          type="email"
          required
          placeholder="E-mail completo da pessoa"
          aria-label="E-mail da pessoa"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <button type="submit" className="btn" disabled={action.busy}>
          Compartilhar
        </button>
      </form>
      <ErrorNote error={action.error} />
    </section>
  );
}

function History({ projectId, version }: { projectId: number; version: number }) {
  // Recarrega a cada alteração do projeto (a versão muda).
  const events = useLoad(() => api.events(projectId), [projectId, version]);
  return (
    <section className="block">
      <header className="block-head">
        <h2>Histórico</h2>
      </header>
      <ErrorNote error={events.error} onRetry={events.reload} />
      {events.data && (
        <ol className="history">
          {events.data.map((e) => (
            <li key={e.id}>
              <span className="history-what">{e.description}</span>
              <span className="history-who">
                {e.userName || e.userEmail}
                {e.asAdmin ? " (como administrador)" : ""}, {dateTime(e.createdAt)}
              </span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

function MonthlyTable({ s }: { s: Scenario }) {
  return (
    <details className="block details">
      <summary>Fluxo de caixa mês a mês</summary>
      <div className="table-wrap">
        <table className="table table-compact">
          <thead>
            <tr>
              <th>Mês</th>
              <th className="num">Receita</th>
              <th className="num">Impostos</th>
              <th className="num">Perda e suporte</th>
              <th className="num">Licenças</th>
              <th className="num">Fluxo líquido</th>
              <th className="num">Saldo descontado</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>Início</td>
              <td className="num" colSpan={4}>
                Investimento, comissão e implantação
              </td>
              <td className="num neg">{money(s.initialFlow)}</td>
              <td className={s.initialFlow < 0 ? "num neg" : "num"}>{money(s.initialFlow)}</td>
            </tr>
            {s.months.map((m) => (
              <tr key={m.month} className={m.month === s.paybackMonth ? "row-current" : undefined}>
                <td>{m.month}</td>
                <td className="num">{money(m.grossRevenue)}</td>
                <td className="num">{money(m.taxes)}</td>
                <td className="num">{money(m.expectedLoss + m.supportCost)}</td>
                <td className="num">{money(m.licensingCost)}</td>
                <td className="num">{money(m.netCashFlow)}</td>
                <td className={m.cumulativeDiscounted < 0 ? "num neg" : "num"}>{money(m.cumulativeDiscounted)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </details>
  );
}

const STATUSES: ProjectStatus[] = ["draft", "active", "completed", "cancelled"];

export function ProjectPage() {
  const id = Number(useParams().id);
  const navigate = useNavigate();
  const user = useUser();
  const { data, setData, error, loading, reload } = useLoad(() => api.project(id), [id]);
  const settings = useLoad(api.settings, []);
  const action = useAction();
  const [sharing, setSharing] = useState(false);

  if (error) {
    return (
      <div className="page">
        <ErrorNote error={error} onRetry={error.status === 404 ? undefined : reload} />
        <Link to="/">Voltar para a carteira</Link>
      </div>
    );
  }
  if (!data || !settings.data) return <div className="page">{(loading || settings.loading) && <Loading what="o projeto" />}</div>;

  const { project, items, analysis, access } = data;
  const canEdit = access === "owner" || access === "admin";
  const s = analysis.scenarios?.pessimistic;
  const empty = items.length === 0;
  const v = s && !empty ? verdict(s, project.durationMonths) : null;

  async function changeStatus(status: ProjectStatus) {
    const saved = await action.run(() => api.updateProject(project.id, projectInput(project, { status })));
    if (saved) setData(saved);
  }
  async function remove() {
    const ok = await action.run(async () => {
      await api.deleteProject(project.id);
      return true;
    });
    if (ok) navigate("/");
  }

  return (
    <div className="page">
      <Link to="/" className="back">
        Carteira
      </Link>
      <header className="page-head project-head">
        <div>
          <h1>
            {project.clientName}
            <span className="h1-sub">{project.name}</span>
          </h1>
          <p className="meta">
            {project.isTest && <span className="tag tag-muted">Teste</span>}
            {access === "shared" && <span>Compartilhado por {project.ownerName}, somente leitura. </span>}
            {access === "admin" && <span>Projeto de {project.ownerName}. Suas alterações ficam registradas como administrador. </span>}
            {project.settingsVersionId !== null && <span>Usa a política financeira congelada na ativação.</span>}
          </p>
        </div>
        <div className="head-actions">
          {canEdit ? (
            <label className="status-select">
              <span>Status</span>
              <select className="input" value={project.status} disabled={action.busy} onChange={(e) => changeStatus(e.target.value as ProjectStatus)}>
                {STATUSES.map((st) => (
                  <option key={st} value={st}>
                    {statusLabel[st]}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            <span className="tag tag-muted">{statusLabel[project.status]}</span>
          )}
          {canEdit && (
            <button type="button" className="btn btn-quiet" aria-expanded={sharing} onClick={() => setSharing((v) => !v)}>
              Compartilhar
            </button>
          )}
          {canEdit && <ConfirmButton label="Excluir" confirmLabel="Excluir projeto" onConfirm={remove} disabled={action.busy} />}
        </div>
      </header>
      <ErrorNote error={action.error} />
      {sharing && canEdit && <Sharing projectId={project.id} />}

      {analysis.status === "blocked" && (
        <section className="verdict verdict-muted">
          <h2>Faltam premissas para calcular</h2>
          <ul>
            {(analysis.blockers ?? []).map((b) => (
              <li key={b}>{b}</li>
            ))}
          </ul>
        </section>
      )}
      {analysis.status === "ready" && empty && (
        <section className="verdict verdict-muted">
          <h2>Adicione os itens da proposta</h2>
          <p>A análise aparece assim que houver equipamentos ou licenças no projeto.</p>
        </section>
      )}
      {v && s && (
        <section className={`verdict verdict-${v.tone}`}>
          <h2>{v.title}</h2>
          <p className="verdict-text">{v.text}</p>
          <dl className="verdict-facts">
            <div>
              <dt>Valor presente líquido</dt>
              <dd>{money(s.npv)}</dd>
            </div>
            <div>
              <dt>Retorno do projeto (TIR)</dt>
              <dd>
                {pctMonth(s.monthlyIrr)}
                {analysis.irr?.status === "ABAIXO_DO_LIMITE" && (
                  <span className="fact-note warn"> abaixo do mínimo de {pct(analysis.irr.threshold)}</span>
                )}
              </dd>
            </div>
            <div>
              <dt>Folga depois do payback</dt>
              <dd>
                {analysis.health?.hasPayback ? `${analysis.health.bufferMonths} meses` : "nenhuma"}
                <span className="fact-note"> saúde {healthLabel[analysis.health?.status ?? "CRITICA"].toLowerCase()}</span>
              </dd>
            </div>
            <div>
              <dt>Investimento inicial</dt>
              <dd>{money(s.capex)}</dd>
            </div>
          </dl>
          <PaybackChart scenario={s} />
        </section>
      )}

      <div className="columns">
        <div className="col-main">
          <Conditions project={project} policy={settings.data.policy} canEdit={canEdit} onSaved={setData} />
          <Items project={project} items={items} analysis={analysis} canEdit={canEdit} onChanged={setData} />
          {s && !empty && <Pricing detail={data} canEdit={canEdit} onSaved={setData} />}
          {s && !empty && <MonthlyTable s={s} />}
        </div>
        <div className="col-side">
          {s && !empty && <Ledger s={s} analysis={analysis} />}
          <History projectId={project.id} version={project.version} />
        </div>
      </div>
      {user.role === "admin" && access !== "owner" && <p className="muted small">Você está vendo este projeto como administrador.</p>}
    </div>
  );
}
