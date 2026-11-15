import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import { parseNumber, pct, UFS } from "../format";
import { ErrorNote, Field, Loading, useAction, useLoad } from "../ui";

export function ProjectNew() {
  const navigate = useNavigate();
  const settings = useLoad(api.settings, []);
  const save = useAction();
  const [form, setForm] = useState({
    clientName: "",
    name: "",
    durationMonths: "36",
    monthlyRevenue: "",
    implementationRevenue: "",
    exchangeRate: "",
    destState: "SP",
    contractStartDate: "",
    dealId: "",
    isTest: false,
  });
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm((f) => ({ ...f, [k]: e.target.value }));

  if (settings.error) return <div className="page"><ErrorNote error={settings.error} onRetry={settings.reload} /></div>;
  if (!settings.data) return <div className="page"><Loading what="a política financeira" /></div>;
  const policy = settings.data.policy.settings;

  async function submit(e: FormEvent) {
    e.preventDefault();
    const created = await save.run(() =>
      api.createProject({
        clientName: form.clientName,
        name: form.name,
        isTest: form.isTest,
        durationMonths: Number(form.durationMonths),
        monthlyRevenue: parseNumber(form.monthlyRevenue) ?? 0,
        implementationRevenue: parseNumber(form.implementationRevenue) ?? 0,
        exchangeRate: parseNumber(form.exchangeRate),
        destState: form.destState,
        contractStartDate: form.contractStartDate || null,
        dealId: parseNumber(form.dealId),
        notes: "",
      }),
    );
    if (created) navigate(`/projetos/${created.project.id}`);
  }

  return (
    <div className="page page-narrow">
      <header className="page-head">
        <h1>Novo projeto</h1>
      </header>
      <p className="lead">
        Informe as condições comerciais. Os equipamentos e licenças entram na tela seguinte, e a análise aparece
        assim que houver itens.
      </p>
      <form onSubmit={submit} className="form">
        <div className="form-grid">
          <Field label="Cliente">
            <input className="input" required maxLength={200} value={form.clientName} onChange={set("clientName")} autoFocus />
          </Field>
          <Field label="Nome do projeto">
            <input className="input" required maxLength={200} value={form.name} onChange={set("name")} />
          </Field>
          <Field label="Prazo do contrato">
            <select className="input" value={form.durationMonths} onChange={set("durationMonths")}>
              {[...policy.contractDurations].sort((a, b) => a - b).map((d) => (
                <option key={d} value={d}>
                  {d} meses
                </option>
              ))}
            </select>
          </Field>
          <Field label="Mensalidade (R$)" hint="Pode ficar em branco e ser definida depois.">
            <input className="input num" inputMode="decimal" placeholder="0,00" value={form.monthlyRevenue} onChange={set("monthlyRevenue")} />
          </Field>
          <Field label="Receita de implantação (R$)" hint="Recebida no início do contrato.">
            <input className="input num" inputMode="decimal" placeholder="0,00" value={form.implementationRevenue} onChange={set("implementationRevenue")} />
          </Field>
          <Field label="Câmbio USD/BRL" hint="Obrigatório se houver itens em dólar ou licenças por pontos.">
            <input className="input num" inputMode="decimal" placeholder="Ex.: 5,45" value={form.exchangeRate} onChange={set("exchangeRate")} />
          </Field>
          <Field label="Início do contrato" hint="Obrigatório para licenças por pontos.">
            <input className="input" type="date" value={form.contractStartDate} onChange={set("contractStartDate")} />
          </Field>
          <Field label="UF de destino" hint="Onde a Informo recebe o equipamento; define o DIFAL.">
            <select className="input" value={form.destState} onChange={set("destState")}>
              {UFS.map((uf) => (
                <option key={uf}>{uf}</option>
              ))}
            </select>
          </Field>
          <Field label="Deal ID do Pipedrive" hint="Opcional.">
            <input className="input num" inputMode="numeric" value={form.dealId} onChange={set("dealId")} />
          </Field>
        </div>
        <label className="check">
          <input type="checkbox" checked={form.isTest} onChange={(e) => setForm((f) => ({ ...f, isTest: e.target.checked }))} />
          É um projeto de teste (fica fora dos indicadores da carteira)
        </label>
        <p className="form-note">
          O projeto começa com inadimplência esperada de {pct(policy.defaultExpectedLossRate)} e suporte de R${" "}
          {policy.defaultMonthlySupportCost.toFixed(2).replace(".", ",")} por mês, conforme a política. Dá para ajustar
          depois.
        </p>
        <ErrorNote error={save.error} />
        <div className="form-actions">
          <button type="submit" className="btn btn-primary" disabled={save.busy}>
            {save.busy ? "Criando…" : "Criar projeto"}
          </button>
          <button type="button" className="btn btn-quiet" onClick={() => navigate("/")}>
            Cancelar
          </button>
        </div>
      </form>
    </div>
  );
}
