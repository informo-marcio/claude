import { FormEvent, useEffect, useState } from "react";
import { api, Policy, Project, ProjectDetail, ProjectInput } from "../api";
import { date, money, parseNumber, pct, toField, UFS } from "../format";
import { ErrorNote, Field, Row, useAction } from "../ui";

/** Monta o corpo de edição a partir do projeto, trocando só o que mudou. */
export function projectInput(p: Project, changes: Partial<ProjectInput> = {}): ProjectInput {
  return {
    clientName: p.clientName,
    name: p.name,
    status: p.status,
    isTest: p.isTest,
    durationMonths: p.durationMonths,
    monthlyRevenue: p.monthlyRevenue,
    implementationRevenue: p.implementationRevenue,
    expectedLossRate: p.expectedLossRate,
    monthlySupportCost: p.monthlySupportCost,
    exchangeRate: p.exchangeRate,
    destState: p.destState,
    contractStartDate: p.contractStartDate,
    dealId: p.dealId,
    notes: p.notes,
    version: p.version,
    ...changes,
  };
}

function toForm(p: Project) {
  return {
    clientName: p.clientName,
    name: p.name,
    durationMonths: String(p.durationMonths),
    monthlyRevenue: toField(p.monthlyRevenue),
    implementationRevenue: toField(p.implementationRevenue),
    lossPct: toField(p.expectedLossRate * 100, 4),
    supportCost: toField(p.monthlySupportCost),
    exchangeRate: toField(p.exchangeRate, 4),
    destState: p.destState,
    contractStartDate: p.contractStartDate ?? "",
    dealId: p.dealId === null ? "" : String(p.dealId),
    isTest: p.isTest,
    notes: p.notes,
  };
}

export function Conditions({
  project,
  policy,
  canEdit,
  onSaved,
}: {
  project: Project;
  policy: Policy;
  canEdit: boolean;
  onSaved: (d: ProjectDetail) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState(() => toForm(project));
  const save = useAction();
  useEffect(() => {
    if (!editing) setForm(toForm(project));
  }, [project, editing]);
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm((f) => ({ ...f, [k]: e.target.value }));

  async function submit(e: FormEvent) {
    e.preventDefault();
    const saved = await save.run(() =>
      api.updateProject(
        project.id,
        projectInput(project, {
          clientName: form.clientName,
          name: form.name,
          durationMonths: Number(form.durationMonths),
          monthlyRevenue: parseNumber(form.monthlyRevenue) ?? 0,
          implementationRevenue: parseNumber(form.implementationRevenue) ?? 0,
          expectedLossRate: (parseNumber(form.lossPct) ?? 0) / 100,
          monthlySupportCost: parseNumber(form.supportCost) ?? 0,
          exchangeRate: parseNumber(form.exchangeRate),
          destState: form.destState,
          contractStartDate: form.contractStartDate || null,
          dealId: parseNumber(form.dealId),
          isTest: form.isTest,
          notes: form.notes,
        }),
      ),
    );
    if (saved) {
      onSaved(saved);
      setEditing(false);
    }
  }

  if (!editing) {
    return (
      <section className="block">
        <header className="block-head">
          <h2>Condições do contrato</h2>
          {canEdit && (
            <button type="button" className="btn btn-quiet" onClick={() => setEditing(true)}>
              Editar condições
            </button>
          )}
        </header>
        <dl className="facts">
          <Row label="Mensalidade" value={money(project.monthlyRevenue)} />
          <Row label="Prazo" value={`${project.durationMonths} meses`} />
          <Row label="Receita de implantação" value={money(project.implementationRevenue)} />
          <Row label="Inadimplência esperada" value={pct(project.expectedLossRate)} />
          <Row label="Suporte por mês" value={money(project.monthlySupportCost)} />
          <Row label="Câmbio USD/BRL" value={project.exchangeRate === null ? "não informado" : toField(project.exchangeRate, 4)} />
          <Row label="Início do contrato" value={project.contractStartDate ? date(project.contractStartDate) : "não informado"} />
          <Row label="UF de destino" value={project.destState} />
          {project.dealId !== null && <Row label="Deal ID do Pipedrive" value={project.dealId} />}
        </dl>
        {project.notes && <p className="notes">{project.notes}</p>}
      </section>
    );
  }

  const durations = [...new Set([...policy.settings.contractDurations, project.durationMonths])].sort((a, b) => a - b);
  return (
    <section className="block">
      <header className="block-head">
        <h2>Condições do contrato</h2>
      </header>
      <form onSubmit={submit} className="form">
        <div className="form-grid">
          <Field label="Cliente">
            <input className="input" required maxLength={200} value={form.clientName} onChange={set("clientName")} />
          </Field>
          <Field label="Nome do projeto">
            <input className="input" required maxLength={200} value={form.name} onChange={set("name")} />
          </Field>
          <Field label="Mensalidade (R$)">
            <input className="input num" inputMode="decimal" value={form.monthlyRevenue} onChange={set("monthlyRevenue")} autoFocus />
          </Field>
          <Field label="Prazo do contrato">
            <select className="input" value={form.durationMonths} onChange={set("durationMonths")}>
              {durations.map((d) => (
                <option key={d} value={d}>
                  {d} meses
                </option>
              ))}
            </select>
          </Field>
          <Field label="Receita de implantação (R$)">
            <input className="input num" inputMode="decimal" value={form.implementationRevenue} onChange={set("implementationRevenue")} />
          </Field>
          <Field label="Inadimplência esperada (%)" hint="Perda esperada sobre a mensalidade faturada.">
            <input className="input num" inputMode="decimal" value={form.lossPct} onChange={set("lossPct")} />
          </Field>
          <Field label="Suporte por mês (R$)" hint="Custo operacional recorrente.">
            <input className="input num" inputMode="decimal" value={form.supportCost} onChange={set("supportCost")} />
          </Field>
          <Field label="Câmbio USD/BRL" hint="Vale para todos os itens em dólar e licenças por pontos.">
            <input className="input num" inputMode="decimal" value={form.exchangeRate} onChange={set("exchangeRate")} />
          </Field>
          <Field label="Início do contrato">
            <input className="input" type="date" value={form.contractStartDate} onChange={set("contractStartDate")} />
          </Field>
          <Field label="UF de destino">
            <select className="input" value={form.destState} onChange={set("destState")}>
              {UFS.map((uf) => (
                <option key={uf}>{uf}</option>
              ))}
            </select>
          </Field>
          <Field label="Deal ID do Pipedrive">
            <input className="input num" inputMode="numeric" value={form.dealId} onChange={set("dealId")} />
          </Field>
          <Field label="Observações" wide>
            <textarea className="input" rows={3} maxLength={5000} value={form.notes} onChange={set("notes")} />
          </Field>
        </div>
        <label className="check">
          <input type="checkbox" checked={form.isTest} onChange={(e) => setForm((f) => ({ ...f, isTest: e.target.checked }))} />
          É um projeto de teste (fica fora dos indicadores da carteira)
        </label>
        <ErrorNote error={save.error} />
        <div className="form-actions">
          <button type="submit" className="btn btn-primary" disabled={save.busy}>
            {save.busy ? "Salvando…" : "Salvar condições"}
          </button>
          <button
            type="button"
            className="btn btn-quiet"
            onClick={() => {
              save.clear();
              setEditing(false);
            }}
          >
            Cancelar
          </button>
        </div>
      </form>
    </section>
  );
}
